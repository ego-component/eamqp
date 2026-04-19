// pubsub example demonstrates a simple publish/subscribe pattern with RabbitMQ.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ego-component/eamqp"
	amqp "github.com/rabbitmq/amqp091-go"
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

	// Declare exchange and queue.
	exchange := "events"
	queue := "my-queue"
	routingKey := "order.created"

	// Declare topic exchange.
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	// Declare queue.
	q, err := ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Bind queue to exchange.
	if err := ch.QueueBind(queue, routingKey, exchange, false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	// Start consuming in a goroutine.
	deliveries, err := ch.Consume(q.Name, "consumer-1", false, false, false, false, nil)
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
	go func() {
		for delivery := range deliveries {
			fmt.Printf("Received: %s\n", string(delivery.Body))
			fmt.Printf("  Exchange: %s, RoutingKey: %s\n", delivery.Exchange, delivery.RoutingKey)

			// Acknowledge.
			if err := delivery.Ack(false); err != nil {
				log.Printf("Failed to ack: %v", err)
			}
		}
	}()

	// Publish messages in a loop.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Shutting down...")
			return
		case <-ticker.C:
			msg := fmt.Sprintf(`{"order_id": %d, "amount": %.2f}`, time.Now().Unix(), 99.99)
			err := ch.Publish(exchange, routingKey, false, false, amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Body:         []byte(msg),
			})
			if err != nil {
				log.Printf("Failed to publish: %v", err)
				continue
			}
			fmt.Printf("Published: %s\n", msg)
		}
	}
}

func getAddr() string {
	if addr := os.Getenv("AMQP_ADDR"); addr != "" {
		return addr
	}
	return "amqp://guest:guest@localhost:5672/"
}
