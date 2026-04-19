// consumer example demonstrates consuming with manual acks.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ego-component/eamqp/examples/internal/exampleconfig"
)

func main() {
	client, err := exampleconfig.LoadClient(exampleconfig.DefaultComponentKey)
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

	// Use the same topology as examples/producer so the two examples can be
	// run together during local verification.
	exchange := "producer.example"
	if err := ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	queue := "producer.example.queue"
	_, err = ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}
	if err := ch.QueueBind(queue, "test", exchange, false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
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
