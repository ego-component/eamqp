// sender publishes test messages to the retry queue.
//
// Usage:
//
//	go run ./examples/retry-consumer-sender --config=examples/config/local.toml
package main

import (
	"fmt"
	"log"
	"time"

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

	const exchange = "retry.ex"
	if err := ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	// Messages containing "fail" will be retried up to 3 times by the listener,
	// then rejected to the DLQ. Messages without "fail" succeed on first attempt.
	messages := []string{
		`{"order_id": 1, "type": "success"}`,
		`{"order_id": 2, "type": "fail"}`,
		`{"order_id": 3, "type": "success"}`,
		`{"order_id": 4, "type": "fail"}`,
		`{"order_id": 5, "type": "success"}`,
	}

	for i, msg := range messages {
		err := ch.Publish(exchange, "retry.test", false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         []byte(msg),
		})
		if err != nil {
			log.Printf("Failed to publish message %d: %v", i, err)
		} else {
			fmt.Printf("Sent: %s\n", msg)
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println("\nAll messages sent. Check listener output.")
}
