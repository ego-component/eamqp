// pubsub-fanout demonstrates the fanout exchange pattern where each subscriber
// gets its own copy of every message. This is a true publish/subscribe model.
//
// Key difference from direct/topic exchanges:
//   - Fanout: all bound queues receive every message (broadcast)
//   - Each subscriber declares an exclusive, auto-delete queue bound to the fanout exchange
//
// Usage:
//
//	# Terminal 1 - start subscriber 1:
//	go run ./examples/pubsub-fanout --config=examples/config/local.toml subscriber1
//
//	# Terminal 2 - start subscriber 2:
//	go run ./examples/pubsub-fanout --config=examples/config/local.toml subscriber2
//
//	# Terminal 3 - publish messages:
//	go run ./examples/pubsub-fanout --config=examples/config/local.toml publish
//
// Run multiple subscriber terminals to see each receive every message.
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
	"github.com/ego-component/eamqp/examples/internal/exampleconfig"
	amqp "github.com/rabbitmq/amqp091-go"
)

const exchange = "pubsub-fanout.ex"

func main() {
	args := exampleconfig.Args()
	if len(args) < 1 {
		fmt.Println("Usage: go run ./examples/pubsub-fanout --config=examples/config/local.toml <subscriber-id|publish>")
		fmt.Println("  subscriber-id: any unique name (e.g., sub1, sub2)")
		fmt.Println("  publish:       run as publisher")
		os.Exit(1)
	}

	role := args[0]

	client, err := exampleconfig.LoadClient(exampleconfig.DefaultComponentKey)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ch, err := client.NewChannel()
	if err != nil {
		log.Fatalf("Failed to create channel: %v", err)
	}
	defer ch.Close()

	// Declare the fanout exchange.
	if err := ch.ExchangeDeclare(exchange, "fanout", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare fanout exchange: %v", err)
	}

	if role == "publish" {
		runPublisher(ch)
	} else {
		runSubscriber(ch, role)
	}
}

func runPublisher(ch *eamqp.Channel) {
	fmt.Println("Publisher ready. Sending events every 2s. Press Ctrl+C to stop.")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		cancel()
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Publisher shutdown.")
			return
		case <-ticker.C:
			i++
			body := fmt.Sprintf(`{"event": "notification", "id": %d, "ts": %d}`, i, time.Now().Unix())
			err := ch.Publish(exchange, "", false, false, amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Body:         []byte(body),
			})
			if err != nil {
				log.Printf("Publish failed: %v", err)
				continue
			}
			fmt.Printf("[%s] Published event: %s\n", time.Now().Format("15:04:05"), body)
		}
	}
}

func runSubscriber(ch *eamqp.Channel, id string) {
	// Exclusive queue: auto-deleted when this consumer disconnects.
	// Each subscriber gets a unique queue so all receive every message.
	// The empty queue name lets RabbitMQ generate a unique name.
	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare exclusive queue: %v", err)
	}

	// Bind the exclusive queue to the fanout exchange (routing key is ignored).
	if err := ch.QueueBind(q.Name, "", exchange, false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	deliveries, err := ch.Consume(q.Name, id, false, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to start consuming: %v", err)
	}

	fmt.Printf("Subscriber '%s' ready. Queue: %s\n", id, q.Name)
	fmt.Println("Waiting for events...")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Subscriber '%s' shutdown.\n", id)
			return
		case delivery, ok := <-deliveries:
			if !ok {
				fmt.Println("Channel closed.")
				return
			}
			fmt.Printf("[%s] [%s] Received: %s\n", time.Now().Format("15:04:05"), id, string(delivery.Body))
			if err := delivery.Ack(false); err != nil {
				log.Printf("Ack failed: %v", err)
			}
		}
	}
}
