package activemq

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xvlet/amqcli/config"
)

func TestArtemisJolokiaClient_AllOperations(t *testing.T) {
	// Mock Jolokia Server for Artemis
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JolokiaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Check if batch request
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{"request":{"mbean":"java.lang:type=Memory"},"value":{"HeapMemoryUsage":{"used":1048576,"max":2097152},"NonHeapMemoryUsage":{"used":524288}}},
				{"request":{"mbean":"java.lang:type=Threading"},"value":{"ThreadCount":42,"PeakThreadCount":50}}
			]`))
			return
		}

		w.Header().Set("Content-Type", "application/json")

		// 1. Broker Info / Version
		if req.Type == "read" && req.Mbean == "org.apache.activemq.artemis:broker=*" && req.Attribute == "Version" {
			_, _ = w.Write([]byte(`{
				"status": 200,
				"value": {
					"org.apache.activemq.artemis:broker=\"artemis-broker\"": {
						"Version": "2.33.0"
					}
				}
			}`))
			return
		}

		// 2. Broker Stats
		if req.Type == "read" && req.Mbean == "org.apache.activemq.artemis:broker=*" {
			_, _ = w.Write([]byte(`{
				"status": 200,
				"value": {
					"org.apache.activemq.artemis:broker=\"artemis-broker\"": {
						"TotalMessagesAdded": 1500,
						"TotalMessagesAcknowledged": 1200,
						"TotalConsumerCount": 8,
						"TotalConnectionCount": 5,
						"AddressMemoryUsagePercentage": 25,
						"DiskStoreUsage": 10,
						"Uptime": "2h 30m"
					}
				}
			}`))
			return
		}

		// 3. OperatingSystem CPU
		if req.Type == "read" && req.Mbean == "java.lang:type=OperatingSystem" {
			_, _ = w.Write([]byte(`{
				"status": 200,
				"value": {
					"ProcessCpuLoad": 0.15,
					"SystemCpuLoad": 0.25
				}
			}`))
			return
		}

		// 4. Queues List
		if req.Type == "read" && strings.Contains(req.Mbean, "subcomponent=queues") && !strings.Contains(req.Mbean, "multicast") {
			_, _ = w.Write([]byte(`{
				"status": 200,
				"value": {
					"org.apache.activemq.artemis:broker=\"artemis-broker\",component=addresses,address=\"ORDER.QUEUE\",queue=\"ORDER.QUEUE\",routing-type=\"anycast\",subcomponent=queues": {
						"MessageCount": 50,
						"ConsumerCount": 2,
						"MessagesAdded": 500,
						"MessagesAcknowledged": 450,
						"DeliveringCount": 3,
						"MessagesExpired": 1,
						"PersistentSize": 20480
					},
					"org.apache.activemq.artemis:broker=\"artemis-broker\",component=addresses,address=\"DLQ\",queue=\"DLQ\",routing-type=\"anycast\",subcomponent=queues": {
						"MessageCount": 5,
						"ConsumerCount": 0,
						"MessagesAdded": 5,
						"MessagesAcknowledged": 0,
						"DeliveringCount": 0,
						"MessagesExpired": 0,
						"PersistentSize": 1024
					}
				}
			}`))
			return
		}

		// 5. Topics List (Multicast)
		if req.Type == "read" && strings.Contains(req.Mbean, "multicast") {
			_, _ = w.Write([]byte(`{
				"status": 200,
				"value": {
					"org.apache.activemq.artemis:broker=\"artemis-broker\",component=addresses,address=\"EVENTS.TOPIC\",queue=\"EVENTS.TOPIC\",routing-type=\"multicast\",subcomponent=queues": {
						"MessagesAdded": 100,
						"MessagesAcknowledged": 100,
						"ConsumerCount": 5
					}
				}
			}`))
			return
		}

		// 6. Connections List (listConnectionsAsJSON)
		if req.Type == "exec" && req.Operation == "listConnectionsAsJSON()" {
			_, _ = w.Write([]byte(`{
				"status": 200,
				"value": "[{\"connectionID\":\"conn-1\",\"clientAddress\":\"/127.0.0.1:54321\",\"creationTime\":1690000000000}]"
			}`))
			return
		}

		// 7. Consumers List (listConsumersAsJSON)
		if req.Type == "exec" && (req.Operation == "listConsumersAsJSON()" || req.Operation == "listConsumersAsJSON(java.lang.String)") {
			_, _ = w.Write([]byte(`{
				"status": 200,
				"value": "[{\"consumerID\":\"cons-1\",\"connectionID\":\"conn-1\",\"sessionID\":\"sess-12345-1690000000000\",\"queueName\":\"ORDER.QUEUE\",\"remoteAddress\":\"/127.0.0.1:54321\",\"deliveringCount\":1,\"messagesAcknowledged\":20,\"creationTime\":1690000000000}]"
			}`))
			return
		}

		// 8. Exec operations (createQueue, destroyQueue, removeAllMessages, etc.)
		if req.Type == "exec" {
			_, _ = w.Write([]byte(`{"status": 200, "value": true}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": 200, "value": {}}`))
	}))
	defer server.Close()

	cfg := config.ActiveMQConfig{
		Host:       "127.0.0.1",
		BrokerType: "artemis",
		JolokiaURL: server.URL,
	}

	client := NewArtemisJolokiaClient(cfg)

	// Test 1: GetBrokerInfo
	info, err := client.GetBrokerInfo()
	if err != nil || !strings.Contains(info, "2.33.0") {
		t.Fatalf("GetBrokerInfo failed: %v, info: %s", err, info)
	}

	// Test 2: GetBrokerStats
	stats, err := client.GetBrokerStats()
	if err != nil || stats.TotalEnqueueCount != 1500 || stats.TotalDequeueCount != 1200 {
		t.Fatalf("GetBrokerStats failed: %v, stats: %+v", err, stats)
	}

	// Test 3: GetJVMStats
	jvmStats, err := client.GetJVMStats()
	if err != nil || jvmStats.ThreadCount != 42 {
		t.Fatalf("GetJVMStats failed: %v, stats: %+v", err, jvmStats)
	}

	// Test 4: GetQueues
	queues, err := client.GetQueues()
	if err != nil || len(queues) != 2 {
		t.Fatalf("GetQueues failed: %v, queues: %+v", err, queues)
	}
	// DLQ should be sorted first
	if queues[0].Name != "DLQ" {
		t.Errorf("Expected DLQ to be sorted first, got %s", queues[0].Name)
	}
	if queues[1].Pending != 50 || queues[1].Enqueued != 500 {
		t.Errorf("Expected ORDER.QUEUE pending=50, enqueued=500, got: %+v", queues[1])
	}

	// Test 5: GetTopics
	topics, err := client.GetTopics()
	if err != nil || len(topics) != 1 || topics[0].Name != "EVENTS.TOPIC" {
		t.Fatalf("GetTopics failed: %v, topics: %+v", err, topics)
	}

	// Test 6: GetConnections
	connections, err := client.GetConnections()
	if err != nil || len(connections) != 1 || connections[0].Name != "conn-1" {
		t.Fatalf("GetConnections failed: %v, connections: %+v", err, connections)
	}

	// Test 7: GetAllConsumers
	consumers, err := client.GetAllConsumers()
	if err != nil || len(consumers) != 1 || consumers[0].ConsumerID != "cons-1" {
		t.Fatalf("GetAllConsumers failed: %v, consumers: %+v", err, consumers)
	}

	// Test 8: GetQueueDetail
	qd, err := client.GetQueueDetail("ORDER.QUEUE")
	if err != nil || qd.Name != "ORDER.QUEUE" || len(qd.Consumers) != 1 {
		t.Fatalf("GetQueueDetail failed: %v, qd: %+v", err, qd)
	}

	// Test 9: Management Operations
	if err := client.CreateQueue("NEW.QUEUE"); err != nil {
		t.Errorf("CreateQueue failed: %v", err)
	}
	if err := client.DeleteQueue("OLD.QUEUE"); err != nil {
		t.Errorf("DeleteQueue failed: %v", err)
	}
	if err := client.PurgeQueue("ORDER.QUEUE"); err != nil {
		t.Errorf("PurgeQueue failed: %v", err)
	}
	if err := client.RemoveMessage("ORDER.QUEUE", "12345"); err != nil {
		t.Errorf("RemoveMessage failed: %v", err)
	}
	if err := client.MoveMessage("ORDER.QUEUE", "12345", "OTHER.QUEUE"); err != nil {
		t.Errorf("MoveMessage failed: %v", err)
	}
	if err := client.CopyMessage("ORDER.QUEUE", "12345", "TEMP.QUEUE"); err != nil {
		t.Errorf("CopyMessage failed: %v", err)
	}
	if err := client.RetryMessage("DLQ", "12345"); err != nil {
		t.Errorf("RetryMessage failed: %v", err)
	}
}

func TestFactory_DetectQueueRepository(t *testing.T) {
	// Mock Server that responds to Artemis probe
	artemisServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JolokiaRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Mbean == "org.apache.activemq.artemis:broker=*" {
			_, _ = w.Write([]byte(`{"status": 200, "value": {"org.apache.activemq.artemis:broker=\"artemis\"": {"Version": "2.31.0"}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status": 404, "error": "No MBean"}`))
	}))
	defer artemisServer.Close()

	cfg := config.ActiveMQConfig{
		Host:       "127.0.0.1",
		BrokerType: "auto",
		JolokiaURL: artemisServer.URL,
	}

	repo, detected := DetectQueueRepository(cfg)
	if detected != "artemis" {
		t.Errorf("Expected detected broker to be artemis, got %s", detected)
	}
	if _, ok := repo.(*ArtemisJolokiaClient); !ok {
		t.Errorf("Expected *ArtemisJolokiaClient type, got %T", repo)
	}
}
