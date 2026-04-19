// producer-confirm demonstrates the high-level Producer API with publisher confirms.
// Publisher confirms ensure messages are acknowledged by the broker before we consider
// them successfully published.
//
// Usage:
//
//	go run ./examples/producer-confirm --config=examples/config/local.toml
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/ego-component/eamqp"
	"github.com/ego-component/eamqp/examples/internal/exampleconfig"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
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

	const (
		exchange   = "producer-confirm.ex"
		queueName  = "producer-confirm.queue"
		routingKey = "confirm.test"
	)

	if err := ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}
	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}
	if err := ch.QueueBind(queueName, routingKey, exchange, false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	// Create a producer with confirms enabled (blocks until broker acks).
	producer, err := eamqp.NewProducer(ch, eamqp.WithConfirm(5*time.Second))
	if err != nil {
		log.Fatalf("Failed to create producer: %v", err)
	}
	defer producer.Close()

	fmt.Println("=== Sync Publish (with confirms) ===")
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"id": %d, "msg": "sync message %d"}`, i, i)
		err := producer.Publish(exchange, routingKey, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Body:         []byte(body),
		})
		if err != nil {
			log.Printf("Publish %d failed: %v", i, err)
		} else {
			fmt.Printf("Message %d confirmed: %s\n", i, body)
		}
	}

	fmt.Println("\n=== Async Publish (fire-and-forget) ===")
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"id": %d, "msg": "async message %d"}`, i, i)
		conf, err := producer.PublishAsync(exchange, routingKey, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Body:         []byte(body),
		})
		if err != nil {
			log.Printf("PublishAsync %d failed immediately: %v", i, err)
			continue
		}
		go func(idx int, c *amqp.DeferredConfirmation) {
			select {
			case <-c.Done():
				if c.Acked() {
					fmt.Printf("Async %d confirmed\n", idx)
				} else {
					fmt.Printf("Async %d nacked\n", idx)
				}
			case <-time.After(5 * time.Second):
				fmt.Printf("Async %d timed out\n", idx)
			}
		}(i, conf)
	}

	time.Sleep(3 * time.Second)
	fmt.Println("\nDone.")
}
