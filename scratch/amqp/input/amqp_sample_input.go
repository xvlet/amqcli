package main

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/Azure/go-amqp"
)

func main() {
	// Use timeout for connection and initialization
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create AMQP connection with explicit SASL authentication
	opts := &amqp.ConnOptions{
		SASLType: amqp.SASLTypePlain("admin", "admin"),
	}
	conn, err := amqp.Dial(ctx, "amqp://127.0.0.1:5672/", opts)
	if err != nil {
		log.Fatalf("Failed to dial AMQP: %v", err)
	}
	defer func() { _ = conn.Close() }()

	fmt.Println("Starting high-throughput AMQP message generation... (Press Ctrl+C to stop)")

	var totalSent uint64
	numWorkers := 50 // Run 50 concurrent senders for high TPS

	// Initialize workers sequentially to avoid AMQP handshake deadlocks
	for i := 0; i < numWorkers; i++ {
		// Each worker gets its own session and sender for optimal compatibility
		session, err := conn.NewSession(ctx, nil)
		if err != nil {
			log.Fatalf("Worker %d failed to create AMQP session: %v", i, err)
		}

		sender, err := session.NewSender(ctx, "AMQP.IN.Q", nil)
		if err != nil {
			log.Fatalf("Worker %d failed to create AMQP sender: %v", i, err)
		}

		go func(workerID int, sess *amqp.Session, s *amqp.Sender) {
			defer func() { _ = s.Close(context.Background()) }()
			defer func() { _ = sess.Close(context.Background()) }()

			seq := 0
			for {
				seq++
				now := time.Now().Format(time.RFC3339Nano)

				// Create dynamic JSON payload
				payload := []byte(fmt.Sprintf(`{"id": "msg-amqp-w%d-%06d", "type": "test", "status": "active", "timestamp": "%s"}`, workerID, seq, now))

				// Use timeout to prevent blocking forever if queue is full (Producer Flow Control)
				sendCtx, sendCancel := context.WithTimeout(context.Background(), 3*time.Second)
				err := s.Send(sendCtx, amqp.NewMessage(payload), nil)
				sendCancel()

				if err != nil {
					log.Printf("Worker %d failed to send AMQP message: %v", workerID, err)
					time.Sleep(1 * time.Second)
					continue
				}

				atomic.AddUint64(&totalSent, 1)

				// 10ms sleep to control CPU usage while maintaining high TPS across 50 workers
				time.Sleep(10 * time.Millisecond)
			}
		}(i, session, sender)
	}

	// Print stats every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastCount uint64
	for {
		<-ticker.C
		current := atomic.LoadUint64(&totalSent)
		tps := current - lastCount
		lastCount = current
		fmt.Printf("[AMQP.IN.Q] TPS: %d msgs/sec | Total Sent: %d\n", tps, current)
	}
}
