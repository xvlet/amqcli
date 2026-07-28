package main

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/go-stomp/stomp/v3"
)

func main() {
	fmt.Println("Starting high-throughput STOMP message generation... (Press Ctrl+C to stop)")

	var totalSent uint64
	numWorkers := 50 // Run 50 concurrent senders for high TPS

	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			// Each worker gets its own connection for optimal concurrency without locking bottlenecks
			conn, err := stomp.Dial("tcp", "127.0.0.1:61613", stomp.ConnOpt.Login("admin", "admin"))
			if err != nil {
				log.Fatalf("Worker %d failed to connect to STOMP: %v", workerID, err)
			}
			defer func() { _ = conn.Disconnect() }()

			seq := 0
			for {
				seq++
				now := time.Now().Format(time.RFC3339Nano)

				// Create dynamic JSON payload
				payload := []byte(fmt.Sprintf(`{"id": "msg-stomp-w%d-%06d", "type": "test", "status": "active", "timestamp": "%s"}`, workerID, seq, now))

				// Send to STOMP.IN.Q (ActiveMQ STOMP requires /queue/ prefix)
				err = conn.Send("/queue/STOMP.IN.Q", "application/json", payload)
				if err != nil {
					log.Printf("Worker %d failed to send: %v", workerID, err)
					time.Sleep(1 * time.Second)
					continue
				}

				atomic.AddUint64(&totalSent, 1)

				// Sleep 10ms to prevent CPU overload
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Print stats every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastCount uint64
	for {
		<-ticker.C
		currentTotal := atomic.LoadUint64(&totalSent)
		tps := currentTotal - lastCount
		lastCount = currentTotal
		fmt.Printf("[STOMP.IN.Q] TPS: %d msgs/sec | Total Sent: %d\n", tps, currentTotal)
	}
}
