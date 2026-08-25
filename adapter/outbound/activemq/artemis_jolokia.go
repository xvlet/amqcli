package activemq

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xvlet/amqcli/config"
	"github.com/xvlet/amqcli/domain"
)

// ArtemisJolokiaClient implements domain.QueueRepository for Apache ActiveMQ Artemis (2.x / 3.x)
type ArtemisJolokiaClient struct {
	url        string
	username   string
	password   string
	brokerName string
	client     *http.Client
}

func NewArtemisJolokiaClient(cfg config.ActiveMQConfig) *ArtemisJolokiaClient {
	url := cfg.JolokiaURL
	if url == "" {
		host := cfg.Host
		if host == "" {
			host = "127.0.0.1"
		}
		webPort := cfg.WebPort
		if webPort == "" {
			webPort = "8161"
		}
		// Default Artemis Jolokia path in Hawtio
		url = fmt.Sprintf("http://%s:%s/console/jolokia", host, webPort)
	}

	if !strings.Contains(url, "?") {
		url += "?maxDepth=10&maxCollectionSize=10000&maxObjects=10000"
	}

	return &ArtemisJolokiaClient{
		url:        url,
		username:   cfg.Username,
		password:   cfg.Password,
		brokerName: "localhost",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetURL returns the active Jolokia URL
func (a *ArtemisJolokiaClient) GetURL() string {
	return a.url
}

// SetURL sets or updates the Jolokia URL
func (a *ArtemisJolokiaClient) SetURL(url string) {
	if !strings.Contains(url, "?") {
		url += "?maxDepth=10&maxCollectionSize=10000&maxObjects=10000"
	}
	a.url = url
}

func (a *ArtemisJolokiaClient) doRequest(reqData JolokiaRequest) ([]byte, error) {
	b, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, a.url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Set Origin header to satisfy Artemis Jolokia CORS policies (allow localhost / 127.0.0.1)
	origin := "http://localhost:8161"
	if parts := strings.Split(a.url, "/"); len(parts) >= 3 {
		hostPort := parts[2]
		scheme := parts[0]
		if strings.HasPrefix(scheme, "http") {
			if strings.HasPrefix(hostPort, "127.0.0.1") {
				port := ""
				if hp := strings.Split(hostPort, ":"); len(hp) > 1 {
					port = ":" + hp[1]
				}
				origin = fmt.Sprintf("%s//localhost%s", scheme, port)
			} else {
				origin = fmt.Sprintf("%s//%s", scheme, hostPort)
			}
		}
	}
	req.Header.Set("Origin", origin)

	if a.username != "" && a.password != "" {
		req.SetBasicAuth(a.username, a.password)
	}

	// #nosec G704 -- internal http client using configured URL
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var baseResp struct {
		Status int    `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &baseResp); err == nil {
		if baseResp.Status != 200 && baseResp.Status != 0 {
			return nil, fmt.Errorf("artemis jolokia error (status %d): %s", baseResp.Status, baseResp.Error)
		}
	} else {
		trimmed := strings.TrimSpace(string(respBytes))
		if strings.HasPrefix(trimmed, "<") {
			if resp.StatusCode == http.StatusUnauthorized {
				return nil, fmt.Errorf("jolokia authentication failed (401): check username/password")
			}
			if resp.StatusCode == http.StatusForbidden {
				return nil, fmt.Errorf("jolokia access forbidden (403): check origins/permissions")
			}
			return nil, fmt.Errorf("jolokia returned HTML instead of JSON (status %d): check URL and credentials", resp.StatusCode)
		}
	}

	return respBytes, nil
}

func (a *ArtemisJolokiaClient) GetBrokerStats() (domain.BrokerStats, error) {
	var stats domain.BrokerStats

	// 1. Get Artemis ServerControl Stats
	reqData := JolokiaRequest{
		Type:  "read",
		Mbean: "org.apache.activemq.artemis:broker=*",
	}

	respBytes, err := a.doRequest(reqData)
	if err == nil {
		var result struct {
			Value map[string]map[string]interface{} `json:"value"`
		}
		if err := json.Unmarshal(respBytes, &result); err == nil {
			for mbeanKey, v := range result.Value {
				// Cache brokerName from MBean key
				a.extractBrokerName(mbeanKey)

				if val, ok := v["TotalMessagesAdded"].(float64); ok {
					stats.TotalEnqueueCount = int64(val)
				}
				if val, ok := v["TotalMessagesAcknowledged"].(float64); ok {
					stats.TotalDequeueCount = int64(val)
				}
				if val, ok := v["TotalConsumerCount"].(float64); ok {
					stats.TotalConsumerCount = int64(val)
				}
				if val, ok := v["TotalConnectionCount"].(float64); ok {
					stats.TotalProducerCount = int64(val) // Use Connection count as proxy if producer count is not global
				} else if val, ok := v["ConnectionCount"].(float64); ok {
					stats.TotalProducerCount = int64(val)
				}
				if val, ok := v["AddressMemoryUsagePercentage"].(float64); ok {
					stats.MemoryPercentUsage = int(val)
				}
				if val, ok := v["DiskStoreUsage"].(float64); ok {
					stats.StorePercentUsage = int(val)
				}
				if uptime, ok := v["Uptime"].(string); ok && uptime != "" {
					stats.Uptime = uptime
				} else if uptimeMillis, ok := v["UptimeMillis"].(float64); ok && uptimeMillis > 0 {
					stats.Uptime = formatDuration(time.Duration(uptimeMillis) * time.Millisecond)
				}
				break
			}
		}
	}

	// 2. Get Operating System CPU Stats
	osReq := JolokiaRequest{
		Type:      "read",
		Mbean:     "java.lang:type=OperatingSystem",
		Attribute: "ProcessCpuLoad,SystemCpuLoad",
	}
	if osBytes, err := a.doRequest(osReq); err == nil {
		var result struct {
			Value struct {
				ProcessCpuLoad float64 `json:"ProcessCpuLoad"`
				SystemCpuLoad  float64 `json:"SystemCpuLoad"`
			} `json:"value"`
		}
		if err := json.Unmarshal(osBytes, &result); err == nil {
			cpu := result.Value.ProcessCpuLoad
			if cpu < 0 {
				cpu = result.Value.SystemCpuLoad
			}
			if cpu > 0 {
				stats.CPUUsage = cpu * 100.0
			}
		}
	}

	return stats, nil
}

func (a *ArtemisJolokiaClient) GetBrokerInfo() (string, error) {
	artemisReq := JolokiaRequest{
		Type:      "read",
		Mbean:     "org.apache.activemq.artemis:broker=*",
		Attribute: "Version",
	}

	respBytes, err := a.doRequest(artemisReq)
	if err != nil {
		return "", err
	}

	var result struct {
		Value map[string]struct {
			Version string `json:"Version"`
		} `json:"value"`
	}
	if err := json.Unmarshal(respBytes, &result); err == nil && len(result.Value) > 0 {
		for k, v := range result.Value {
			a.extractBrokerName(k)
			if v.Version != "" {
				return fmt.Sprintf("Apache ActiveMQ Artemis %s", v.Version), nil
			}
		}
	}

	var resultStr struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(respBytes, &resultStr); err == nil && resultStr.Value != "" {
		return fmt.Sprintf("Apache ActiveMQ Artemis %s", resultStr.Value), nil
	}

	return "Apache ActiveMQ Artemis", nil
}

func (a *ArtemisJolokiaClient) GetJVMStats() (domain.JVMStats, error) {
	var stats domain.JVMStats

	batchReq := []JolokiaRequest{
		{
			Type:  "read",
			Mbean: "java.lang:type=Memory",
		},
		{
			Type:  "read",
			Mbean: "java.lang:type=Threading",
		},
	}

	b, _ := json.Marshal(batchReq)
	req, _ := http.NewRequest(http.MethodPost, a.url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if a.username != "" && a.password != "" {
		req.SetBasicAuth(a.username, a.password)
	}

	// #nosec G704 -- internal http client
	resp, err := a.client.Do(req)
	if err != nil {
		return stats, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return stats, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return stats, err
	}

	var results []struct {
		Request struct {
			Mbean string `json:"mbean"`
		} `json:"request"`
		Value map[string]interface{} `json:"value"`
	}

	if err := json.Unmarshal(body, &results); err != nil {
		return stats, err
	}

	for _, r := range results {
		switch r.Request.Mbean {
		case "java.lang:type=Memory":
			if heap, ok := r.Value["HeapMemoryUsage"].(map[string]interface{}); ok {
				if used, ok := heap["used"].(float64); ok {
					stats.HeapMemoryUsed = int64(used)
				}
				if max, ok := heap["max"].(float64); ok {
					stats.HeapMemoryMax = int64(max)
				}
			}
			if nonHeap, ok := r.Value["NonHeapMemoryUsage"].(map[string]interface{}); ok {
				if used, ok := nonHeap["used"].(float64); ok {
					stats.NonHeapMemoryUsed = int64(used)
				}
			}
		case "java.lang:type=Threading":
			if count, ok := r.Value["ThreadCount"].(float64); ok {
				stats.ThreadCount = int(count)
			}
			if peak, ok := r.Value["PeakThreadCount"].(float64); ok {
				stats.PeakThreadCount = int(peak)
			}
		}
	}

	return stats, nil
}

func (a *ArtemisJolokiaClient) GetQueues() ([]domain.Queue, error) {
	// Query all queues across all addresses
	reqData := JolokiaRequest{
		Type:  "read",
		Mbean: "org.apache.activemq.artemis:broker=*,component=addresses,address=*,subcomponent=queues,routing-type=*,queue=*",
	}

	respBytes, err := a.doRequest(reqData)
	if err != nil {
		if strings.Contains(err.Error(), "status 404") || strings.Contains(err.Error(), "No MBean") {
			return []domain.Queue{}, nil
		}
		return nil, err
	}

	var result struct {
		Value map[string]map[string]interface{} `json:"value"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}

	var queues []domain.Queue
	for mbeanKey, props := range result.Value {
		name := a.extractProperty(mbeanKey, "queue")
		if name == "" {
			if v, ok := props["Name"].(string); ok {
				name = v
			} else if v, ok := props["QueueName"].(string); ok {
				name = v
			}
		}

		// Filter out internal multicast or temporary sub-queues if needed, but display all named queues
		if name != "" {
			q := domain.Queue{
				Name: name,
			}

			if v, ok := props["MessageCount"].(float64); ok {
				q.Pending = int64(v)
			}
			if v, ok := props["ConsumerCount"].(float64); ok {
				q.Consumers = int64(v)
			}
			if v, ok := props["MessagesAdded"].(float64); ok {
				q.Enqueued = int64(v)
			}
			if v, ok := props["MessagesAcknowledged"].(float64); ok {
				q.Dequeued = int64(v)
			}
			if v, ok := props["DeliveringCount"].(float64); ok {
				q.InFlightCount = int64(v)
			}
			if v, ok := props["MessagesExpired"].(float64); ok {
				q.ExpiredCount = int64(v)
			}
			if v, ok := props["MessagesKilled"].(float64); ok && q.ExpiredCount == 0 {
				q.ExpiredCount = int64(v)
			}
			if v, ok := props["PersistentSize"].(float64); ok {
				q.StoreMessageSize = int64(v)
			}

			queues = append(queues, q)
		}
	}

	sort.Slice(queues, func(i, j int) bool {
		// DLQ to the top if present
		if strings.EqualFold(queues[i].Name, "DLQ") || strings.EqualFold(queues[i].Name, "ActiveMQ.DLQ") {
			return true
		}
		if strings.EqualFold(queues[j].Name, "DLQ") || strings.EqualFold(queues[j].Name, "ActiveMQ.DLQ") {
			return false
		}
		return strings.ToLower(queues[i].Name) < strings.ToLower(queues[j].Name)
	})

	return queues, nil
}

func (a *ArtemisJolokiaClient) GetTopics() ([]domain.Topic, error) {
	// In Artemis, topics are multicast queues or addresses with multicast routing
	reqData := JolokiaRequest{
		Type:  "read",
		Mbean: "org.apache.activemq.artemis:broker=*,component=addresses,address=*,subcomponent=queues,routing-type=\"multicast\",queue=*",
	}

	respBytes, err := a.doRequest(reqData)
	if err != nil {
		if strings.Contains(err.Error(), "status 404") || strings.Contains(err.Error(), "No MBean") {
			return []domain.Topic{}, nil
		}
		return nil, err
	}

	var result struct {
		Value map[string]map[string]interface{} `json:"value"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}

	var topics []domain.Topic
	for mbeanKey, props := range result.Value {
		name := a.extractProperty(mbeanKey, "address")
		if name == "" {
			name = a.extractProperty(mbeanKey, "queue")
		}

		if name != "" {
			t := domain.Topic{
				Name: name,
			}
			if v, ok := props["MessagesAdded"].(float64); ok {
				t.EnqueueCount = int64(v)
			}
			if v, ok := props["MessagesAcknowledged"].(float64); ok {
				t.DequeueCount = int64(v)
			}
			if v, ok := props["ConsumerCount"].(float64); ok {
				t.ConsumerCount = int64(v)
			}
			topics = append(topics, t)
		}
	}

	sort.Slice(topics, func(i, j int) bool {
		return strings.ToLower(topics[i].Name) < strings.ToLower(topics[j].Name)
	})

	return topics, nil
}

func (a *ArtemisJolokiaClient) GetConnections() ([]domain.Connection, error) {
	// 1. Try listConnectionsAsJSON() operation on ActiveMQServerControl
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     a.getServerControlMBean(),
		Operation: "listConnectionsAsJSON()",
	}

	respBytes, err := a.doRequest(reqData)
	if err == nil {
		var rawJSON string
		var resultStr struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(respBytes, &resultStr); err == nil && resultStr.Value != "" {
			rawJSON = resultStr.Value
		} else {
			var resultMap struct {
				Value map[string]string `json:"value"`
			}
			if err := json.Unmarshal(respBytes, &resultMap); err == nil && len(resultMap.Value) > 0 {
				for k, v := range resultMap.Value {
					a.extractBrokerName(k)
					rawJSON = v
					break
				}
			}
		}

		if rawJSON != "" {
			var connList []struct {
				ConnectionID  string `json:"connectionID"`
				ClientAddress string `json:"clientAddress"`
				RemoteAddress string `json:"remoteAddress"`
				CreationTime  int64  `json:"creationTime"`
			}
			if err := json.Unmarshal([]byte(rawJSON), &connList); err == nil && len(connList) > 0 {
				var connections []domain.Connection
				for _, c := range connList {
					addr := c.ClientAddress
					if addr == "" {
						addr = c.RemoteAddress
					}
					addr = strings.TrimPrefix(addr, "/")
					addr = strings.TrimPrefix(addr, "tcp://")

					connections = append(connections, domain.Connection{
						Name:          c.ConnectionID,
						RemoteAddress: addr,
						Active:        true,
						Slow:          false,
					})
				}
				sort.Slice(connections, func(i, j int) bool {
					return strings.ToLower(connections[i].Name) < strings.ToLower(connections[j].Name)
				})
				return connections, nil
			}
		}
	}

	// 2. Fallback: Query connection MBeans directly
	connReq := JolokiaRequest{
		Type:  "read",
		Mbean: "org.apache.activemq.artemis:broker=*,component=connections,connection=*",
	}
	if connBytes, err := a.doRequest(connReq); err == nil {
		var connRes struct {
			Value map[string]map[string]interface{} `json:"value"`
		}
		if err := json.Unmarshal(connBytes, &connRes); err == nil {
			var connections []domain.Connection
			for k, v := range connRes.Value {
				name := a.extractProperty(k, "connection")
				addr := ""
				if aVal, ok := v["RemoteAddress"].(string); ok {
					addr = aVal
				} else if aVal, ok := v["ClientAddress"].(string); ok {
					addr = aVal
				}
				addr = strings.TrimPrefix(addr, "/")
				addr = strings.TrimPrefix(addr, "tcp://")

				if name != "" {
					connections = append(connections, domain.Connection{
						Name:          name,
						RemoteAddress: addr,
						Active:        true,
					})
				}
			}
			sort.Slice(connections, func(i, j int) bool {
				return strings.ToLower(connections[i].Name) < strings.ToLower(connections[j].Name)
			})
			return connections, nil
		}
	}

	return []domain.Connection{}, nil
}

func (a *ArtemisJolokiaClient) GetAllConsumers() ([]domain.Consumer, error) {
	// 1. Try listConsumersAsJSON(java.lang.String) operation on ActiveMQServerControl
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     a.getServerControlMBean(),
		Operation: "listConsumersAsJSON(java.lang.String)",
		Arguments: []interface{}{"{}"},
	}

	respBytes, err := a.doRequest(reqData)
	if err == nil {
		var rawJSON string
		var resultStr struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(respBytes, &resultStr); err == nil && resultStr.Value != "" {
			rawJSON = resultStr.Value
		} else {
			var resultMap struct {
				Value map[string]string `json:"value"`
			}
			if err := json.Unmarshal(respBytes, &resultMap); err == nil && len(resultMap.Value) > 0 {
				for k, v := range resultMap.Value {
					a.extractBrokerName(k)
					rawJSON = v
					break
				}
			}
		}

		if rawJSON != "" {
			var rawConsumers []struct {
				ConsumerID           interface{} `json:"consumerID"`
				ConnectionID         string      `json:"connectionID"`
				SessionID            string      `json:"sessionID"`
				QueueName            string      `json:"queueName"`
				Address              string      `json:"address"`
				RemoteAddress        string      `json:"remoteAddress"`
				ClientAddress        string      `json:"clientAddress"`
				DeliveringCount      int64       `json:"deliveringCount"`
				MessagesAcknowledged int64       `json:"messagesAcknowledged"`
				CreationTime         int64       `json:"creationTime"`
			}
			if err := json.Unmarshal([]byte(rawJSON), &rawConsumers); err == nil {
				var consumers []domain.Consumer
				for _, rc := range rawConsumers {
					addr := rc.RemoteAddress
					if addr == "" {
						addr = rc.ClientAddress
					}
					addr = strings.TrimPrefix(addr, "/")
					addr = strings.TrimPrefix(addr, "tcp://")

					dest := rc.QueueName
					if dest == "" {
						dest = rc.Address
					}

					cID := fmt.Sprintf("%v", rc.ConsumerID)
					c := domain.Consumer{
						ConsumerID:       cID,
						ConnectionID:     rc.ConnectionID,
						ClientID:         rc.SessionID,
						DestinationName:  dest,
						RemoteAddress:    addr,
						Dequeues:         rc.MessagesAcknowledged,
						PendingQueueSize: rc.DeliveringCount,
					}

					pid, uptime := a.parseConsumerInfo(c.ClientID, c.ConnectionID)
					if rc.CreationTime > 0 {
						dur := time.Since(time.UnixMilli(rc.CreationTime))
						if dur > 0 {
							uptime = formatDuration(dur)
						}
					}
					c.PID = pid
					c.Uptime = uptime

					consumers = append(consumers, c)
				}

				sort.Slice(consumers, func(i, j int) bool {
					return strings.ToLower(consumers[i].DestinationName) < strings.ToLower(consumers[j].DestinationName)
				})
				return consumers, nil
			}
		}
	}

	// 2. Fallback: Query consumer MBeans directly
	mbeanPattern := "org.apache.activemq.artemis:broker=*,component=addresses,address=*,subcomponent=queues,routing-type=*,queue=*,subcomponent=consumers,consumer-id=*"
	consReq := JolokiaRequest{
		Type:  "read",
		Mbean: mbeanPattern,
	}

	consBytes, err := a.doRequest(consReq)
	if err != nil {
		return []domain.Consumer{}, nil
	}

	var mbeanResult struct {
		Value map[string]map[string]interface{} `json:"value"`
	}
	if err := json.Unmarshal(consBytes, &mbeanResult); err != nil {
		return []domain.Consumer{}, nil
	}

	var consumers []domain.Consumer
	for k, props := range mbeanResult.Value {
		cID := a.extractProperty(k, "consumer-id")
		qName := a.extractProperty(k, "queue")

		c := domain.Consumer{
			ConsumerID:      cID,
			DestinationName: qName,
		}

		if v, ok := props["ConnectionID"].(string); ok {
			c.ConnectionID = v
		}
		if v, ok := props["SessionID"].(string); ok {
			c.ClientID = v
		}
		if v, ok := props["RemoteAddress"].(string); ok {
			c.RemoteAddress = strings.TrimPrefix(strings.TrimPrefix(v, "/"), "tcp://")
		}
		if v, ok := props["MessagesAcknowledged"].(float64); ok {
			c.Dequeues = int64(v)
		}
		if v, ok := props["DeliveringCount"].(float64); ok {
			c.PendingQueueSize = int64(v)
		}

		pid, uptime := a.parseConsumerInfo(c.ClientID, c.ConnectionID)
		c.PID = pid
		c.Uptime = uptime

		consumers = append(consumers, c)
	}

	sort.Slice(consumers, func(i, j int) bool {
		return strings.ToLower(consumers[i].DestinationName) < strings.ToLower(consumers[j].DestinationName)
	})

	return consumers, nil
}

func (a *ArtemisJolokiaClient) GetQueueDetail(name string) (*domain.QueueDetail, error) {
	// Query specific queue MBean using wildcard address to support any address binding
	mbeanPattern := fmt.Sprintf("org.apache.activemq.artemis:broker=*,component=addresses,address=*,subcomponent=queues,routing-type=*,queue=%s", name)
	if !strings.Contains(name, "\"") {
		mbeanPattern = fmt.Sprintf("org.apache.activemq.artemis:broker=*,component=addresses,address=*,subcomponent=queues,routing-type=*,queue=\"%s\"", name)
	}

	reqData := JolokiaRequest{
		Type:  "read",
		Mbean: mbeanPattern,
	}

	respBytes, err := a.doRequest(reqData)
	if err != nil {
		// Fallback without quotes
		mbeanPattern = fmt.Sprintf("org.apache.activemq.artemis:broker=*,component=addresses,address=*,subcomponent=queues,routing-type=*,queue=%s", name)
		reqData.Mbean = mbeanPattern
		respBytes, err = a.doRequest(reqData)
		if err != nil {
			return nil, err
		}
	}

	var result struct {
		Value map[string]map[string]interface{} `json:"value"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}

	qd := &domain.QueueDetail{
		Name: name,
	}

	var specificMBean string
	for mbeanKey, props := range result.Value {
		specificMBean = mbeanKey
		if v, ok := props["MessageCount"].(float64); ok {
			qd.QueueSize = int64(v)
		}
		if v, ok := props["ConsumerCount"].(float64); ok {
			qd.ConsumerCount = int64(v)
		}
		if v, ok := props["MessagesAdded"].(float64); ok {
			qd.EnqueueCount = int64(v)
		}
		if v, ok := props["MessagesAcknowledged"].(float64); ok {
			qd.DequeueCount = int64(v)
		}
		if v, ok := props["DeliveringCount"].(float64); ok {
			qd.InFlightCount = int64(v)
		}
		if v, ok := props["MessagesExpired"].(float64); ok {
			qd.ExpiredCount = int64(v)
		}
		if v, ok := props["MessagesKilled"].(float64); ok && qd.ExpiredCount == 0 {
			qd.ExpiredCount = int64(v)
		}
		if v, ok := props["PersistentSize"].(float64); ok {
			qd.StoreMessageSize = int64(v)
		}
		break
	}

	// Resolve connection RemoteAddress map
	connMap := make(map[string]string)
	conns, _ := a.GetConnections()
	for _, conn := range conns {
		connMap[conn.Name] = conn.RemoteAddress
	}

	// Directly query consumers on this specific queue MBean
	if specificMBean != "" {
		consReq := JolokiaRequest{
			Type:      "exec",
			Mbean:     specificMBean,
			Operation: "listConsumersAsJSON()",
		}
		if cBytes, err := a.doRequest(consReq); err == nil {
			var cResult struct {
				Value string `json:"value"`
			}
			if err := json.Unmarshal(cBytes, &cResult); err == nil && cResult.Value != "" {
				var rawCons []struct {
					ConsumerID           interface{} `json:"consumerID"`
					SequentialID         interface{} `json:"sequentialId"`
					ConnectionID         string      `json:"connectionID"`
					SessionID            string      `json:"sessionID"`
					MessagesAcknowledged int64       `json:"messagesAcknowledged"`
					CreationTime         int64       `json:"creationTime"`
				}
				if err := json.Unmarshal([]byte(cResult.Value), &rawCons); err == nil {
					for _, rc := range rawCons {
						cID := fmt.Sprintf("%v", rc.ConsumerID)
						remoteAddr := connMap[rc.ConnectionID]
						if remoteAddr == "" {
							remoteAddr = "127.0.0.1"
						}

						uptime := "-"
						if rc.CreationTime > 0 {
							dur := time.Since(time.UnixMilli(rc.CreationTime))
							if dur > 0 {
								uptime = formatDuration(dur)
							}
						}

						pid, _ := a.parseConsumerInfo(rc.SessionID, rc.ConnectionID)
						qd.Consumers = append(qd.Consumers, domain.Consumer{
							ConsumerID:      cID,
							ConnectionID:    rc.ConnectionID,
							ClientID:        rc.SessionID,
							DestinationName: name,
							RemoteAddress:   remoteAddr,
							Dequeues:        rc.MessagesAcknowledged,
							PID:             pid,
							Uptime:          uptime,
						})
					}
				}
			}
		}
	}

	// Fallback to GetAllConsumers if specific MBean query returned nothing but ConsumerCount > 0
	if len(qd.Consumers) == 0 && qd.ConsumerCount > 0 {
		allConsumers, err := a.GetAllConsumers()
		if err == nil {
			for _, c := range allConsumers {
				if strings.EqualFold(c.DestinationName, name) {
					qd.Consumers = append(qd.Consumers, c)
				}
			}
		}
	}

	return qd, nil
}

func (a *ArtemisJolokiaClient) CreateQueue(name string) error {
	// In Artemis, createQueue on ActiveMQServerControl takes (address, queueName, routingType)
	// e.g. createQueue(java.lang.String,java.lang.String,java.lang.String)
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     a.getServerControlMBean(),
		Operation: "createQueue(java.lang.String,java.lang.String,java.lang.String)",
		Arguments: []interface{}{name, name, "ANYCAST"},
	}

	_, err := a.doRequest(reqData)
	if err != nil {
		// Fallback for older Artemis versions: createAddress or 2-arg createQueue
		reqData.Operation = "createQueue(java.lang.String,java.lang.String)"
		reqData.Arguments = []interface{}{name, name}
		_, err2 := a.doRequest(reqData)
		if err2 == nil {
			return nil
		}
		return err
	}
	return nil
}

func (a *ArtemisJolokiaClient) DeleteQueue(name string) error {
	// In Artemis: destroyQueue(java.lang.String,boolean) or destroyQueue(java.lang.String)
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     a.getServerControlMBean(),
		Operation: "destroyQueue(java.lang.String,boolean)",
		Arguments: []interface{}{name, true},
	}

	_, err := a.doRequest(reqData)
	if err != nil {
		// Fallback for single arg destroyQueue
		reqData.Operation = "destroyQueue(java.lang.String)"
		reqData.Arguments = []interface{}{name}
		_, err2 := a.doRequest(reqData)
		if err2 == nil {
			return nil
		}
		return err
	}
	return nil
}

func (a *ArtemisJolokiaClient) PurgeQueue(name string) error {
	queueMBean, err := a.findQueueMBean(name)
	if err != nil {
		return err
	}

	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     queueMBean,
		Operation: "removeAllMessages()",
	}

	_, err = a.doRequest(reqData)
	return err
}

func (a *ArtemisJolokiaClient) RemoveMessage(queueName string, messageID string) error {
	queueMBean, err := a.findQueueMBean(queueName)
	if err != nil {
		return err
	}

	// Artemis removeMessage can take long messageID or string filter
	if numID, err := strconv.ParseInt(messageID, 10, 64); err == nil {
		reqData := JolokiaRequest{
			Type:      "exec",
			Mbean:     queueMBean,
			Operation: "removeMessage(long)",
			Arguments: []interface{}{numID},
		}
		if _, err := a.doRequest(reqData); err == nil {
			return nil
		}
	}

	// Fallback using filter
	filter := fmt.Sprintf("JMSMessageID = '%s' OR AMQMessageID = '%s'", messageID, messageID)
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     queueMBean,
		Operation: "removeMessages(java.lang.String)",
		Arguments: []interface{}{filter},
	}
	_, err = a.doRequest(reqData)
	return err
}

func (a *ArtemisJolokiaClient) MoveMessage(queueName string, messageID string, destQueue string) error {
	queueMBean, err := a.findQueueMBean(queueName)
	if err != nil {
		return err
	}

	filter := fmt.Sprintf("JMSMessageID = '%s' OR AMQMessageID = '%s'", messageID, messageID)
	if _, err := strconv.ParseInt(messageID, 10, 64); err == nil {
		filter = fmt.Sprintf("AMQMessageID = %s OR JMSMessageID = '%s'", messageID, messageID)
	}

	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     queueMBean,
		Operation: "moveMessages(java.lang.String,java.lang.String)",
		Arguments: []interface{}{filter, destQueue},
	}

	_, err = a.doRequest(reqData)
	return err
}

func (a *ArtemisJolokiaClient) CopyMessage(queueName string, messageID string, destQueue string) error {
	queueMBean, err := a.findQueueMBean(queueName)
	if err != nil {
		return err
	}

	filter := fmt.Sprintf("JMSMessageID = '%s' OR AMQMessageID = '%s'", messageID, messageID)
	if _, err := strconv.ParseInt(messageID, 10, 64); err == nil {
		filter = fmt.Sprintf("AMQMessageID = %s OR JMSMessageID = '%s'", messageID, messageID)
	}

	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     queueMBean,
		Operation: "copyMessages(java.lang.String,java.lang.String)",
		Arguments: []interface{}{filter, destQueue},
	}

	_, err = a.doRequest(reqData)
	return err
}

func (a *ArtemisJolokiaClient) RetryMessage(queueName string, messageID string) error {
	queueMBean, err := a.findQueueMBean(queueName)
	if err != nil {
		return err
	}

	if numID, err := strconv.ParseInt(messageID, 10, 64); err == nil {
		reqData := JolokiaRequest{
			Type:      "exec",
			Mbean:     queueMBean,
			Operation: "retryMessage(long)",
			Arguments: []interface{}{numID},
		}
		if _, err := a.doRequest(reqData); err == nil {
			return nil
		}
	}

	filter := fmt.Sprintf("JMSMessageID = '%s' OR AMQMessageID = '%s'", messageID, messageID)
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     queueMBean,
		Operation: "retryMessages(java.lang.String)",
		Arguments: []interface{}{filter},
	}

	_, err = a.doRequest(reqData)
	return err
}

// Helper methods

func (a *ArtemisJolokiaClient) getServerControlMBean() string {
	if a.brokerName != "" && a.brokerName != "localhost" {
		return fmt.Sprintf("org.apache.activemq.artemis:broker=\"%s\"", a.brokerName)
	}
	return "org.apache.activemq.artemis:broker=*"
}

func (a *ArtemisJolokiaClient) findQueueMBean(queueName string) (string, error) {
	// Query for queue MBean name
	pattern := fmt.Sprintf("org.apache.activemq.artemis:broker=*,component=addresses,address=*,subcomponent=queues,routing-type=*,queue=\"%s\"", queueName)
	reqData := JolokiaRequest{
		Type:  "read",
		Mbean: pattern,
	}

	respBytes, err := a.doRequest(reqData)
	if err != nil || len(respBytes) == 0 {
		// Fallback without quotes
		pattern = fmt.Sprintf("org.apache.activemq.artemis:broker=*,component=addresses,address=*,subcomponent=queues,routing-type=*,queue=%s", queueName)
		reqData.Mbean = pattern
		respBytes, err = a.doRequest(reqData)
		if err != nil {
			return "", fmt.Errorf("queue mbean not found for '%s': %w", queueName, err)
		}
	}

	var result struct {
		Value map[string]interface{} `json:"value"`
	}
	if err := json.Unmarshal(respBytes, &result); err == nil && len(result.Value) > 0 {
		for mbean := range result.Value {
			return mbean, nil
		}
	}

	// Default template if query returned empty
	if a.brokerName != "" && a.brokerName != "localhost" {
		return fmt.Sprintf("org.apache.activemq.artemis:broker=\"%s\",component=addresses,address=\"%s\",subcomponent=queues,routing-type=\"anycast\",queue=\"%s\"", a.brokerName, queueName, queueName), nil
	}
	return fmt.Sprintf("org.apache.activemq.artemis:broker=*,component=addresses,address=\"%s\",subcomponent=queues,routing-type=\"anycast\",queue=\"%s\"", queueName, queueName), nil
}

func (a *ArtemisJolokiaClient) extractProperty(mbeanStr, propName string) string {
	parts := strings.Split(mbeanStr, ",")
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) == propName {
			val := strings.TrimSpace(kv[1])
			val = strings.Trim(val, "\"")
			return val
		}
	}
	return ""
}

func (a *ArtemisJolokiaClient) extractBrokerName(mbeanStr string) {
	if a.brokerName == "localhost" || a.brokerName == "" {
		bn := a.extractProperty(mbeanStr, "broker")
		if bn != "" {
			a.brokerName = bn
		}
	}
}

func (a *ArtemisJolokiaClient) parseConsumerInfo(clientID, connectionID string) (string, string) {
	isDefaultCID := strings.HasPrefix(clientID, "ID:")
	isDefaultConnID := strings.HasPrefix(connectionID, "ID:")

	cleanCID := strings.ReplaceAll(clientID, ":", "-")
	cleanCID = strings.ReplaceAll(cleanCID, "_", "-")
	cleanConnID := strings.ReplaceAll(connectionID, ":", "-")
	cleanConnID = strings.ReplaceAll(cleanConnID, "_", "-")

	pid := "-"
	uptime := "-"

	if !isDefaultCID {
		if match := pidRegex.FindStringSubmatch(cleanCID); len(match) > 1 {
			pid = match[1]
		}
	}
	if pid == "-" && !isDefaultConnID {
		if match := pidRegex.FindStringSubmatch(cleanConnID); len(match) > 1 {
			pid = match[1]
		}
	}

	var ts int64
	foundTS := false
	if match := timestampRegex.FindStringSubmatch(cleanCID); len(match) > 1 {
		if val, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			ts = val
			foundTS = true
		}
	}

	if foundTS {
		var connectedAt time.Time
		strTS := strconv.FormatInt(ts, 10)
		switch len(strTS) {
		case 10:
			connectedAt = time.Unix(ts, 0)
		case 13:
			connectedAt = time.Unix(ts/1000, (ts%1000)*1e6)
		case 14:
			if t, err := time.Parse("20060102150405", strTS); err == nil {
				connectedAt = t
			}
		}
		if !connectedAt.IsZero() {
			duration := time.Since(connectedAt)
			if duration > 0 {
				uptime = formatDuration(duration)
			}
		}
	}

	return pid, uptime
}
