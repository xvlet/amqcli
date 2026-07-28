package main

import (
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/go-stomp/stomp/v3"
)

func main() {
	fmt.Println("Starting high-throughput STOMP subscriber and Echo server forwarder...")

	var totalProcessed uint64
	numWorkers := 50 // 50 concurrent receivers to match input rate

	// Custom HTTP Transport for high concurrency (prevent port exhaustion and handshake overhead)
	// tr := &http.Transport{
	// 	MaxIdleConns:        100,
	// 	MaxIdleConnsPerHost: 100,
	// 	IdleConnTimeout:     90 * time.Second,
	// }
	// client := &http.Client{
	// 	Timeout:   5 * time.Second,
	// 	Transport: tr,
	// }

	for i := 0; i < numWorkers; i++ {
		// Each worker gets its own connection
		go func(workerID int) {
			clientID := fmt.Sprintf("amqcli-stomp-worker-%d-%d-%d", os.Getpid(), time.Now().UnixMilli(), workerID)
			conn, err := stomp.Dial("tcp", "127.0.0.1:61613",
				stomp.ConnOpt.Login("admin", "admin"),
				stomp.ConnOpt.Header("client-id", clientID),
			)
			if err != nil {
				log.Fatalf("Worker %d failed to connect to STOMP: %v", workerID, err)
			}
			defer func() { _ = conn.Disconnect() }()

			// Subscribe to STOMP.IN.Q with client-individual ack
			sub, err := conn.Subscribe(
				"/queue/STOMP.IN.Q",
				stomp.AckClientIndividual,
				stomp.SubscribeOpt.Header("activemq.prefetchSize", "10"),
			)
			if err != nil {
				log.Fatalf("Worker %d failed to subscribe: %v", workerID, err)
			}
			defer func() { _ = sub.Unsubscribe() }()

			for {
				msg := <-sub.C
				if msg != nil && msg.Err != nil {
					log.Printf("Worker %d receive error: %v", workerID, msg.Err)
					time.Sleep(1 * time.Second)
					continue
				}

				if msg == nil {
					continue
				}

				// Spawn a goroutine to process the message instantly.
				// This prevents sub.C from filling up and blocking the go-stomp reader goroutine,
				// which would otherwise cause a TCP deadlock with ActiveMQ.
				go func(m *stomp.Message) {
					// Forward to Echo Server
					// resp, err := client.Post("http://127.0.0.1:58080/api/v1/test/stomp", "application/json", bytes.NewReader(m.Body))
					// if err != nil {
					// 	log.Printf("Worker %d HTTP request failed: %v", workerID, err)
					// 	_ = conn.Nack(m)
					// 	return
					// }

					// body, _ := io.ReadAll(resp.Body)
					// _ = resp.Body.Close()
					body := m.Body

					// Push responses to STOMP.OUT.Q
					err = conn.Send("/queue/STOMP.OUT.Q", "application/json", body)
					if err != nil {
						log.Printf("Worker %d failed to send to OUT.Q: %v", workerID, err)
						_ = conn.Nack(m)
						return
					}

					// Ack the original message
					err = conn.Ack(m)
					if err != nil {
						log.Printf("Worker %d failed to ACK: %v", workerID, err)
						return
					}

					atomic.AddUint64(&totalProcessed, 1)
				}(msg)
			}
		}(i)
	}

	// Drain STOMP.OUT.Q so messages don't pile up (concurrent consumers)
	for i := 0; i < numWorkers; i++ {
		go func(drainerID int) {
			clientID := fmt.Sprintf("amqcli-stomp-drainer-%d-%d-%d", os.Getpid(), time.Now().UnixMilli(), drainerID)
			drainConn, err := stomp.Dial("tcp", "127.0.0.1:61613",
				stomp.ConnOpt.Login("admin", "admin"),
				stomp.ConnOpt.Header("client-id", clientID),
			)
			if err != nil {
				log.Fatalf("Drainer %d failed to connect: %v", drainerID, err)
			}
			defer func() { _ = drainConn.Disconnect() }()

			sub, err := drainConn.Subscribe("/queue/STOMP.OUT.Q", stomp.AckAuto)
			if err != nil {
				log.Fatalf("Drainer %d failed to subscribe: %v", drainerID, err)
			}
			defer func() { _ = sub.Unsubscribe() }()

			for {
				msg := <-sub.C
				if msg != nil && msg.Err == nil {
					// Message consumed and discarded automatically due to AckAuto
					_ = msg
				}
			}
		}(i)
	}

	// Print stats every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastCount uint64
	for {
		<-ticker.C
		currentTotal := atomic.LoadUint64(&totalProcessed)
		tps := currentTotal - lastCount
		lastCount = currentTotal
		fmt.Printf("[STOMP.OUT.Q] TPS: %d msgs/sec | Total Processed: %d\n", tps, currentTotal)
	}
}
