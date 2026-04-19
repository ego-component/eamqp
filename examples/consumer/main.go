// consumer example demonstrates consuming with manual acks.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ego-component/eamqp"
)

func main() {
	// Create client.
	client, err := eamqp.New(eamqp.Config{
		Addr: getAddr(),
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create channel.
	ch, err := client.NewChannel()
	if err != nil {
		log.Fatalf("Failed to create channel: %v", err)
	}
	defer ch.Close()

	// Set QoS.
	if err := ch.Qos(10, 0, false); err != nil {
		log.Fatalf("Failed to set QoS: %v", err)
	}

	// Declare queue.
	queue := "consumer.example.queue"
	_, err = ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Start consuming (manual ack).
	deliveries, err := ch.Consume(queue, "consumer-1", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to start consuming: %v", err)
	}

	// Handle graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		cancel()
	}()

	// Consume messages.
	fmt.Println("Waiting for messages...")
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Shutting down...")
			return
		case delivery, ok := <-deliveries:
			if !ok {
				fmt.Println("Channel closed")
				return
			}

			fmt.Printf("Received: %s\n", string(delivery.Body))

			// Simulate processing.
			// Acknowledge.
			if err := delivery.Ack(false); err != nil {
				log.Printf("Failed to ack: %v", err)
			}
		}
	}
}

func getAddr() string {
	if addr := os.Getenv("AMQP_ADDR"); addr != "" {
		return addr
	}
	return "amqp://guest:guest@localhost:5672/"
}
