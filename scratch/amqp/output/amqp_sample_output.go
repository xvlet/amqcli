package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/Azure/go-amqp"
)

func main() {
	// Use timeout for connection and initialization
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create AMQP connection with explicit SASL authentication
	clientID := fmt.Sprintf("amqcli-amqp-worker-%d-%d", os.Getpid(), time.Now().UnixMilli())
	opts := &amqp.ConnOptions{
		SASLType:    amqp.SASLTypePlain("admin", "admin"),
		ContainerID: clientID,
	}
	conn, err := amqp.Dial(ctx, "amqp://127.0.0.1:5672/", opts)
	if err != nil {
		log.Fatalf("Failed to dial AMQP: %v", err)
	}
	defer func() { _ = conn.Close() }()

	fmt.Println("Starting high-throughput AMQP subscriber... (Press Ctrl+C to stop)")

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
		// Each worker gets its own session for optimal compatibility
		session, err := conn.NewSession(ctx, nil)
		if err != nil {
			log.Fatalf("Worker %d failed to create AMQP session: %v", i, err)
		}

		// Create a receiver to fetch data from AMQP.IN.Q (with prefetch credit for high TPS)
		receiver, err := session.NewReceiver(ctx, "AMQP.IN.Q", &amqp.ReceiverOptions{
			Credit: 100,
		})
		if err != nil {
			log.Fatalf("Worker %d failed to create AMQP receiver: %v", i, err)
		}

		// Create a sender to push responses to AMQP.OUT.Q
		sender, err := session.NewSender(ctx, "AMQP.OUT.Q", nil)
		if err != nil {
			log.Fatalf("Worker %d failed to create AMQP sender: %v", i, err)
		}

		// Create a receiver to drain AMQP.OUT.Q
		outReceiver, err := session.NewReceiver(ctx, "AMQP.OUT.Q", &amqp.ReceiverOptions{
			Credit: 100,
		})
		if err != nil {
			log.Fatalf("Worker %d failed to create AMQP outReceiver: %v", i, err)
		}

		// Goroutine to drain and discard messages from AMQP.OUT.Q
		go func(r *amqp.Receiver) {
			defer func() { _ = r.Close(context.Background()) }()
			for {
				msg, err := r.Receive(context.Background(), nil)
				if err != nil {
					time.Sleep(1 * time.Second)
					continue
				}
				_ = r.AcceptMessage(context.Background(), msg)
			}
		}(outReceiver)

		go func(workerID int, sess *amqp.Session, r *amqp.Receiver, s *amqp.Sender) {
			defer func() { _ = r.Close(context.Background()) }()
			defer func() { _ = s.Close(context.Background()) }()
			defer func() { _ = sess.Close(context.Background()) }()

			for {
				// Wait and receive message
				msg, err := r.Receive(context.Background(), nil)
				if err != nil {
					log.Printf("Worker %d failed to receive message: %v", workerID, err)
					time.Sleep(1 * time.Second)
					continue
				}

				payload := msg.GetData()
				_ = r.AcceptMessage(context.Background(), msg) // Ack immediately

				// Send data to Echo server (HTTP POST)
				// respPayload, err := sendToEchoServer(client, payload)
				// if err != nil {
				// 	// Ignore echo errors under high load to keep processing
				// 	continue
				// }
				respPayload := payload

				// Forward the received response to AMQP.OUT.Q
				// Use timeout to prevent block
				sendCtx, sendCancel := context.WithTimeout(context.Background(), 3*time.Second)
				err = s.Send(sendCtx, amqp.NewMessage(respPayload), nil)
				sendCancel()
				if err != nil {
					continue
				}

				atomic.AddUint64(&totalProcessed, 1)
			}
		}(i, session, receiver, sender)
	}

	// Print stats every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastCount uint64
	for {
		<-ticker.C
		current := atomic.LoadUint64(&totalProcessed)
		tps := current - lastCount
		lastCount = current
		fmt.Printf("[AMQP.OUT.Q] TPS: %d msgs/sec | Total Processed: %d\n", tps, current)
	}
}

// sendToEchoServer sends the received data to the Echo server and returns the response payload.
// func sendToEchoServer(client *http.Client, data []byte) ([]byte, error) {
// 	resp, err := client.Post("http://127.0.0.1:58080/api/v1/test/amqp", "application/json", bytes.NewReader(data))
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer func() { _ = resp.Body.Close() }()
//
// 	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
// 		return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
// 	}
//
// 	return io.ReadAll(resp.Body)
// }
