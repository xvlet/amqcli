package activemq

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"amqcli/config"
	"amqcli/domain"
)

var (
	pidRegex       = regexp.MustCompile(`(?:^|[^0-9])([0-9]{4,8})(?:$|[^0-9])`)
	timestampRegex = regexp.MustCompile(`(?:^|[^0-9])([0-9]{10,14})(?:$|[^0-9])`)
)

type JolokiaClient struct {
	url        string
	username   string
	password   string
	brokerName string
	client     *http.Client
}

func NewJolokiaClient(cfg config.ActiveMQConfig) *JolokiaClient {
	// Ensure Jolokia URL has parameters to prevent truncation
	url := cfg.JolokiaURL
	if !strings.Contains(url, "?") {
		url += "?maxDepth=10&maxCollectionSize=10000&maxObjects=10000"
	}

	return &JolokiaClient{
		url:        url,
		username:   cfg.Username,
		password:   cfg.Password,
		brokerName: "localhost", // default fallback
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// JolokiaRequest represents a JSON request to Jolokia
type JolokiaRequest struct {
	Type      string        `json:"type"`
	Mbean     string        `json:"mbean"`
	Attribute string        `json:"attribute,omitempty"`
	Operation string        `json:"operation,omitempty"`
	Value     interface{}   `json:"value,omitempty"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

func (j *JolokiaClient) doRequest(reqData JolokiaRequest) ([]byte, error) {
	b, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, j.url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Add Origin header to bypass Jolokia CORS strict checking
	req.Header.Set("Origin", "http://localhost")
	// Or parse from url, but ActiveMQ usually just needs it to not be absent/null for strict setups unless specifically configured.
	// Actually, just passing the Jolokia URL's domain/scheme works.
	if parts := strings.Split(j.url, "/api/jolokia"); len(parts) > 0 {
		req.Header.Set("Origin", parts[0])
	}

	if j.username != "" && j.password != "" {
		req.SetBasicAuth(j.username, j.password)
	}

	resp, err := j.client.Do(req) // #nosec G704 -- URL sourced from config file, not user input
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check inner Jolokia JSON status
	var baseResp struct {
		Status int    `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &baseResp); err == nil {
		// Valid JSON parsed
		if baseResp.Status != 200 && baseResp.Status != 0 {
			return nil, fmt.Errorf("jolokia error (status %d): %s", baseResp.Status, baseResp.Error)
		}
	} else {
		// Not a valid JSON or parsing failed.
		// If it looks like HTML, it might be an unauthorized page or error page from the server.
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

func (j *JolokiaClient) GetBrokerStats() (domain.BrokerStats, error) {
	var stats domain.BrokerStats

	// 1. Get Broker Stats
	reqData := JolokiaRequest{
		Type:      "read",
		Mbean:     "org.apache.activemq:type=Broker,brokerName=*",
		Attribute: "MemoryPercentUsage,StorePercentUsage,StoreLimit,TempPercentUsage",
	}

	respBytes, err := j.doRequest(reqData)
	if err == nil {
		var result struct {
			Value map[string]struct {
				MemoryPercentUsage int   `json:"MemoryPercentUsage"`
				StorePercentUsage  int   `json:"StorePercentUsage"`
				TempPercentUsage   int   `json:"TempPercentUsage"`
				StoreLimit         int64 `json:"StoreLimit"`
			} `json:"value"`
		}
		if err := json.Unmarshal(respBytes, &result); err == nil {
			for _, v := range result.Value {
				stats.MemoryPercentUsage = v.MemoryPercentUsage
				stats.StorePercentUsage = v.StorePercentUsage
				stats.TempPercentUsage = v.TempPercentUsage
				stats.StoreLimit = v.StoreLimit
				break
			}
		}
	}

	// 2. Get CPU Load
	osReq := JolokiaRequest{
		Type:      "read",
		Mbean:     "java.lang:type=OperatingSystem",
		Attribute: "ProcessCpuLoad,SystemCpuLoad",
	}
	osBytes, err := j.doRequest(osReq)
	if err == nil {
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
				stats.CPUUsage = cpu * 100.0 // Convert to percentage
			}
		}
	}

	return stats, nil
}

func (j *JolokiaClient) GetBrokerInfo() (string, error) {
	// Try Classic first
	reqData := JolokiaRequest{
		Type:      "read",
		Mbean:     "org.apache.activemq:type=Broker,brokerName=*",
		Attribute: "BrokerVersion",
	}

	respBytes, err := j.doRequest(reqData)
	if err == nil {
		var result struct {
			Value map[string]struct {
				BrokerVersion string `json:"BrokerVersion"`
			} `json:"value"`
		}
		if err := json.Unmarshal(respBytes, &result); err == nil && len(result.Value) > 0 {
			for _, v := range result.Value {
				if v.BrokerVersion != "" {
					return fmt.Sprintf("Apache ActiveMQ %s", v.BrokerVersion), nil
				}
			}
		}
		var resultStr struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(respBytes, &resultStr); err == nil && resultStr.Value != "" {
			return fmt.Sprintf("Apache ActiveMQ %s", resultStr.Value), nil
		}
	}

	// Try Artemis
	artemisReq := JolokiaRequest{
		Type:      "read",
		Mbean:     "org.apache.activemq.artemis:broker=*",
		Attribute: "Version",
	}

	respBytes, err = j.doRequest(artemisReq)
	if err == nil {
		var result struct {
			Value map[string]struct {
				Version string `json:"Version"`
			} `json:"value"`
		}
		if err := json.Unmarshal(respBytes, &result); err == nil && len(result.Value) > 0 {
			for _, v := range result.Value {
				if v.Version != "" {
					return fmt.Sprintf("Apache Artemis %s", v.Version), nil
				}
			}
		}
		var resultStr struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(respBytes, &resultStr); err == nil && resultStr.Value != "" {
			return fmt.Sprintf("Apache Artemis %s", resultStr.Value), nil
		}
	}

	return "", fmt.Errorf("could not determine broker info")
}

func (j *JolokiaClient) GetConnections() ([]domain.Connection, error) {
	reqData := JolokiaRequest{
		Type:      "read",
		Mbean:     "org.apache.activemq:type=Broker,connectionName=*,*",
		Attribute: "RemoteAddress,Active,Slow",
	}

	respBytes, err := j.doRequest(reqData)
	if err != nil {
		if strings.Contains(err.Error(), "status 404") && strings.Contains(err.Error(), "No MBean") {
			return []domain.Connection{}, nil
		}
		return nil, err
	}

	var result struct {
		Value map[string]struct {
			RemoteAddress string `json:"RemoteAddress"`
			Active        bool   `json:"Active"`
			Slow          bool   `json:"Slow"`
		} `json:"value"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}

	var connections []domain.Connection
	for k, v := range result.Value {
		// Extract connection name from mbean string
		// org.apache.activemq:brokerName=localhost,connectionName=ID_S822-38295-...,connector=clientConnectors,connectorName=openwire,type=Broker
		name := ""
		parts := strings.Split(k, ",")
		for _, p := range parts {
			if strings.HasPrefix(p, "connectionName=") {
				name = strings.TrimPrefix(p, "connectionName=")
				break
			}
		}

		if name != "" {
			connections = append(connections, domain.Connection{
				Name:          name,
				RemoteAddress: strings.TrimPrefix(v.RemoteAddress, "tcp://"),
				Active:        v.Active,
				Slow:          v.Slow,
			})
		}
	}

	sort.Slice(connections, func(i, j int) bool {
		return connections[i].Name < connections[j].Name
	})

	return connections, nil
}

func (j *JolokiaClient) GetQueues() ([]domain.Queue, error) {
	reqData := JolokiaRequest{
		Type:  "read",
		Mbean: "org.apache.activemq:type=Broker,brokerName=*,destinationType=Queue,destinationName=*",
	}

	respBytes, err := j.doRequest(reqData)
	if err != nil {
		if strings.Contains(err.Error(), "status 404") && strings.Contains(err.Error(), "No MBean") {
			return []domain.Queue{}, nil
		}
		return nil, err
	}

	// This is a simplified JSON parse. In a real ActiveMQ Jolokia response,
	// it will return a complex map under the "value" property.
	// Structure: { "value": { "org.apache.activemq:brokerName=localhost,destinationName=TEST,destinationType=Queue,type=Broker": { "QueueSize": 0, "ConsumerCount": 0, "EnqueueCount": 0, "DequeueCount": 0 } } }
	var result struct {
		Value map[string]struct {
			QueueSize            int64  `json:"QueueSize"`
			ConsumerCount        int64  `json:"ConsumerCount"`
			EnqueueCount         int64  `json:"EnqueueCount"`
			DequeueCount         int64  `json:"DequeueCount"`
			MemoryPercentUsage   int    `json:"MemoryPercentUsage"`
			MemoryUsageByteCount int64  `json:"MemoryUsageByteCount"`
			MemoryLimit          int64  `json:"MemoryLimit"`
			StoreMessageSize     int64  `json:"StoreMessageSize"`
			Name                 string `json:"Name"`
		} `json:"value"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}

	var queues []domain.Queue
	for k, v := range result.Value {
		// Extract queue name from mbean string or from "Name" attribute if present
		name := v.Name
		if name == "" {
			parts := strings.Split(k, ",")
			for _, p := range parts {
				if strings.HasPrefix(p, "destinationName=") {
					name = strings.TrimPrefix(p, "destinationName=")
					break
				}
			}
		}

		if name != "" {
			// Extract brokerName if not already set or "localhost"
			if j.brokerName == "localhost" || j.brokerName == "" {
				parts := strings.Split(k, ",")
				for _, p := range parts {
					if strings.HasPrefix(p, "brokerName=") {
						j.brokerName = strings.TrimPrefix(p, "brokerName=")
						break
					}
				}
			}

			queues = append(queues, domain.Queue{
				Name:               name,
				Pending:            v.QueueSize,
				Consumers:          v.ConsumerCount,
				Enqueued:           v.EnqueueCount,
				Dequeued:           v.DequeueCount,
				MemoryPercentUsage: v.MemoryPercentUsage,
				MemoryUsageBytes:   v.MemoryUsageByteCount,
				MemoryLimit:        v.MemoryLimit,
				StoreMessageSize:   v.StoreMessageSize,
			})
		}
	}

	sort.Slice(queues, func(i, j int) bool {
		if queues[i].Name == "ActiveMQ.DLQ" {
			return true
		}
		if queues[j].Name == "ActiveMQ.DLQ" {
			return false
		}
		return queues[i].Name < queues[j].Name
	})

	return queues, nil
}

func (j *JolokiaClient) GetQueueDetail(name string) (*domain.QueueDetail, error) {
	mbean := fmt.Sprintf("org.apache.activemq:type=Broker,brokerName=%s,destinationType=Queue,destinationName=%s", j.brokerName, name)

	// 1. Fetch Queue Attributes
	reqData := JolokiaRequest{
		Type:  "read",
		Mbean: mbean,
	}

	respBytes, err := j.doRequest(reqData)
	if err != nil {
		return nil, err
	}

	var result struct {
		Value map[string]interface{} `json:"value"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}

	qd := &domain.QueueDetail{
		Name: name,
	}

	if v, ok := result.Value["QueueSize"].(float64); ok {
		qd.QueueSize = int64(v)
	}
	if v, ok := result.Value["ConsumerCount"].(float64); ok {
		qd.ConsumerCount = int64(v)
	}
	if v, ok := result.Value["EnqueueCount"].(float64); ok {
		qd.EnqueueCount = int64(v)
	}
	if v, ok := result.Value["DequeueCount"].(float64); ok {
		qd.DequeueCount = int64(v)
	}
	if v, ok := result.Value["InFlightCount"].(float64); ok {
		qd.InFlightCount = int64(v)
	}
	if v, ok := result.Value["ExpiredCount"].(float64); ok {
		qd.ExpiredCount = int64(v)
	}
	if v, ok := result.Value["DispatchCount"].(float64); ok {
		qd.DispatchCount = int64(v)
	}
	if v, ok := result.Value["ProducerCount"].(float64); ok {
		qd.ProducerCount = int64(v)
	}
	if v, ok := result.Value["MemoryUsageByteCount"].(float64); ok {
		qd.MemoryUsageBytes = int64(v)
	}
	if v, ok := result.Value["MemoryPercentUsage"].(float64); ok {
		qd.MemoryPercentUsage = int(v)
	}
	if v, ok := result.Value["StoreMessageSize"].(float64); ok {
		qd.StoreMessageSize = int64(v)
	}
	if v, ok := result.Value["AverageBlockedTime"].(float64); ok {
		qd.AverageBlockedTime = v
	}

	// 2. Fetch Consumers for this Queue
	// org.apache.activemq:type=Broker,brokerName=localhost,endpoint=Consumer,destinationType=Queue,destinationName=TEST,consumerId=*
	// Added ,* at the end to match MBeans that have additional properties like clientId
	consumerPattern := fmt.Sprintf("org.apache.activemq:type=Broker,brokerName=%s,endpoint=Consumer,destinationType=Queue,destinationName=%s,consumerId=*,*", j.brokerName, name)
	consReq := JolokiaRequest{
		Type:  "read",
		Mbean: consumerPattern,
	}

	consRespBytes, err := j.doRequest(consReq)
	if err != nil {
		// If no consumers found, Jolokia might return error or empty, but we can treat it as empty consumers
		return qd, nil
	}

	var consResult struct {
		Value map[string]map[string]interface{} `json:"value"`
	}
	if err := json.Unmarshal(consRespBytes, &consResult); err == nil {
		// Prepare batch requests for RemoteAddresses.
		// Track the mbean->consumerIndex mapping so results can be correctly matched back.
		type batchEntry struct {
			mbean         string
			consumerIndex int
		}
		var batchEntries []batchEntry
		var batchReqs []JolokiaRequest

		for id, props := range consResult.Value {
			c := domain.Consumer{
				ConsumerID: id,
			}
			if v, ok := props["ClientId"].(string); ok {
				c.ClientID = v
			}
			if v, ok := props["ConnectionId"].(string); ok {
				c.ConnectionID = v
			}

			if conn, ok := props["Connection"].(map[string]interface{}); ok {
				if objName, ok := conn["objectName"].(string); ok {
					batchEntries = append(batchEntries, batchEntry{
						mbean:         objName,
						consumerIndex: len(qd.Consumers),
					})
					batchReqs = append(batchReqs, JolokiaRequest{
						Type:      "read",
						Mbean:     objName,
						Attribute: "RemoteAddress",
					})
				}
			}

			if v, ok := props["EnqueueCounter"].(float64); ok {
				c.Enqueues = int64(v)
			}
			if v, ok := props["DequeueCounter"].(float64); ok {
				c.Dequeues = int64(v)
			}
			if v, ok := props["PrefetchSize"].(float64); ok {
				c.PrefetchSize = int(v)
			}
			if v, ok := props["Exclusive"].(bool); ok {
				c.Exclusive = v
			}
			if v, ok := props["Retroactive"].(bool); ok {
				c.Retroactive = v
			}

			qd.Consumers = append(qd.Consumers, c)
		}

		// Execute Batch Request for RemoteAddresses
		if len(batchReqs) > 0 {
			b, _ := json.Marshal(batchReqs)
			req, _ := http.NewRequest(http.MethodPost, j.url, bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			if parts := strings.Split(j.url, "/api/jolokia"); len(parts) > 0 {
				req.Header.Set("Origin", parts[0])
			}
			if j.username != "" && j.password != "" {
				req.SetBasicAuth(j.username, j.password)
			}
			resp, err := j.client.Do(req)
			if err == nil {
				defer func() { _ = resp.Body.Close() }()
				var batchResp []struct {
					Value string `json:"value"`
				}
				if body, err := io.ReadAll(resp.Body); err == nil {
					if err := json.Unmarshal(body, &batchResp); err == nil && len(batchResp) == len(batchEntries) {
						// Use the tracked mbean->consumerIndex mapping for correct assignment
						for i, r := range batchResp {
							idx := batchEntries[i].consumerIndex
							if idx < len(qd.Consumers) {
								qd.Consumers[idx].RemoteAddress = strings.TrimPrefix(r.Value, "tcp://")
							}
						}
					}
				}
			}
		}

		// PID & Uptime Extraction (Done after all fields are populated)
		for i := range qd.Consumers {
			pid, uptime := j.parseConsumerInfo(qd.Consumers[i].ClientID, qd.Consumers[i].ConnectionID)
			qd.Consumers[i].PID = pid
			qd.Consumers[i].Uptime = uptime
		}
	}

	return qd, nil
}

func (j *JolokiaClient) parseConsumerInfo(clientID, connectionID string) (string, string) {
	// If ClientID or ConnectionID starts with "ID:", it's likely a default ActiveMQ ID.
	// In this case, we should not extract PID from it as it might be a port number.
	isDefaultCID := strings.HasPrefix(clientID, "ID:")
	isDefaultConnID := strings.HasPrefix(connectionID, "ID:")

	// ActiveMQ's ClientID often contains ':' or '_' as separator
	cleanCID := strings.ReplaceAll(clientID, ":", "-")
	cleanCID = strings.ReplaceAll(cleanCID, "_", "-")
	cleanConnID := strings.ReplaceAll(connectionID, ":", "-")
	cleanConnID = strings.ReplaceAll(cleanConnID, "_", "-")

	pid := "-"
	uptime := "-"

	// Try to find PID (4-8 digits)
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

	// Try to find Timestamp (10-14 digits)
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
		case 10: // Unix Seconds
			connectedAt = time.Unix(ts, 0)
		case 13: // Unix Milliseconds
			connectedAt = time.Unix(ts/1000, (ts%1000)*1e6)
		case 14: // YYYYMMDDHHMMSS
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

func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func (j *JolokiaClient) CreateQueue(name string) error {
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     fmt.Sprintf("org.apache.activemq:type=Broker,brokerName=%s", j.brokerName),
		Operation: "addQueue",
		Arguments: []interface{}{name},
	}
	_, err := j.doRequest(reqData)
	return err
}

func (j *JolokiaClient) DeleteQueue(name string) error {
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     fmt.Sprintf("org.apache.activemq:type=Broker,brokerName=%s", j.brokerName),
		Operation: "removeQueue",
		Arguments: []interface{}{name},
	}
	_, err := j.doRequest(reqData)
	return err
}

func (j *JolokiaClient) PurgeQueue(name string) error {
	mbean := fmt.Sprintf("org.apache.activemq:type=Broker,brokerName=%s,destinationType=Queue,destinationName=%s", j.brokerName, name)
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     mbean,
		Operation: "purge",
	}
	_, err := j.doRequest(reqData)
	return err
}

func (j *JolokiaClient) SetAttribute(mbean string, attribute string, value interface{}) error {
	reqData := JolokiaRequest{
		Type:      "write",
		Mbean:     mbean,
		Attribute: attribute,
		Value:     value,
	}
	_, err := j.doRequest(reqData)
	return err
}

func (j *JolokiaClient) BrowseQueue(name string) ([]domain.Message, error) {
	return j.BrowseQueueWithPagination(name, 1000, "")
}

func (j *JolokiaClient) BrowseQueueWithPagination(name string, limit int, selector string) ([]domain.Message, error) {
	mbean := fmt.Sprintf("org.apache.activemq:type=Broker,brokerName=%s,destinationType=Queue,destinationName=%s", j.brokerName, name)

	// Set the page size limit for the broker
	if limit > 0 {
		_ = j.SetAttribute(mbean, "MaxBrowsePageSize", limit)
	}

	reqData := JolokiaRequest{
		Type:  "exec",
		Mbean: mbean,
	}

	if selector != "" {
		reqData.Operation = "browse(java.lang.String)"
		reqData.Arguments = []interface{}{selector}
	} else {
		reqData.Operation = "browse()"
	}

	respBytes, err := j.doRequest(reqData)
	if err != nil {
		return nil, err
	}

	var result struct {
		Value []map[string]interface{} `json:"value"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}

	var messages []domain.Message
	for _, v := range result.Value {
		msg := domain.Message{
			Properties: make(map[string]string),
			Headers:    make(map[string]string),
		}

		if id, ok := v["JMSMessageID"].(string); ok {
			msg.MessageID = id
		}
		if cid, ok := v["JMSCorrelationID"].(string); ok {
			msg.CorrelationID = cid
		}
		if mode, ok := v["JMSDeliveryMode"].(string); ok {
			msg.Persistence = mode
		}
		if p, ok := v["JMSPriority"].(float64); ok {
			msg.Priority = int(p)
		}
		if rd, ok := v["JMSRedelivered"].(bool); ok {
			msg.Redelivered = rd
		}
		if rt, ok := v["JMSReplyTo"].(string); ok {
			msg.ReplyTo = rt
		}
		if typ, ok := v["JMSType"].(string); ok {
			msg.Type = typ
		}
		if tsStr, ok := v["JMSTimestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
				msg.Timestamp = t
			}
		}

		// Body extraction
		if text, ok := v["Text"].(string); ok {
			msg.Body = text
		} else if preview, ok := v["BodyPreview"].([]interface{}); ok {
			bodyBytes := make([]byte, len(preview))
			for i, b := range preview {
				if f, isFloat := b.(float64); isFloat {
					bodyBytes[i] = byte(f)
				}
			}
			msg.Body = string(bodyBytes)
		}

		// Properties extraction
		if strProps, ok := v["StringProperties"].(map[string]interface{}); ok {
			for pk, pv := range strProps {
				if s, isStr := pv.(string); isStr {
					msg.Properties[pk] = s
				}
			}
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

func (j *JolokiaClient) RemoveMessage(queueName string, messageID string) error {
	mbean := fmt.Sprintf("org.apache.activemq:type=Broker,brokerName=%s,destinationType=Queue,destinationName=%s", j.brokerName, queueName)
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     mbean,
		Operation: "removeMessage(java.lang.String)",
		Arguments: []interface{}{messageID},
	}
	_, err := j.doRequest(reqData)
	return err
}

func (j *JolokiaClient) MoveMessage(queueName string, messageID string, destQueue string) error {
	mbean := fmt.Sprintf("org.apache.activemq:type=Broker,brokerName=%s,destinationType=Queue,destinationName=%s", j.brokerName, queueName)
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     mbean,
		Operation: "moveMessageTo(java.lang.String,java.lang.String)",
		Arguments: []interface{}{messageID, destQueue},
	}
	_, err := j.doRequest(reqData)
	return err
}

func (j *JolokiaClient) CopyMessage(queueName string, messageID string, destQueue string) error {
	mbean := fmt.Sprintf("org.apache.activemq:type=Broker,brokerName=%s,destinationType=Queue,destinationName=%s", j.brokerName, queueName)
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     mbean,
		Operation: "copyMessageTo(java.lang.String,java.lang.String)",
		Arguments: []interface{}{messageID, destQueue},
	}
	_, err := j.doRequest(reqData)
	return err
}

func (j *JolokiaClient) RetryMessage(queueName string, messageID string) error {
	mbean := fmt.Sprintf("org.apache.activemq:type=Broker,brokerName=%s,destinationType=Queue,destinationName=%s", j.brokerName, queueName)
	reqData := JolokiaRequest{
		Type:      "exec",
		Mbean:     mbean,
		Operation: "retryMessage(java.lang.String)",
		Arguments: []interface{}{messageID},
	}
	_, err := j.doRequest(reqData)
	return err
}
