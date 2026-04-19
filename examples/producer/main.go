// producer example demonstrates publishing with confirms.
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

	// Create channel.
	ch, err := client.NewChannel()
	if err != nil {
		log.Fatalf("Failed to create channel: %v", err)
	}
	defer ch.Close()

	// Enable publisher confirms.
	if err := ch.Confirm(false); err != nil {
		log.Fatalf("Failed to enable confirms: %v", err)
	}

	// Get confirm channel.
	confirms := ch.NotifyPublish()

	// Declare exchange.
	exchange := "producer.example"
	if err := ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	// Declare queue.
	queue := "producer.example.queue"
	_, err = ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Bind queue.
	if err := ch.QueueBind(queue, "test", exchange, false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	// Publish messages with confirms.
	routingKey := "test"
	for i := 0; i < 10; i++ {
		body := fmt.Sprintf(`{"id": %d, "message": "hello %d"}`, i, i)

		err := ch.Publish(exchange, routingKey, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         []byte(body),
			Timestamp:    time.Now(),
		})
		if err != nil {
			log.Printf("Failed to publish: %v", err)
			continue
		}

		// Wait for confirmation.
		select {
		case confirm := <-confirms:
			if confirm.Ack {
				fmt.Printf("Message %d confirmed (tag=%d)\n", i, confirm.DeliveryTag)
			} else {
				fmt.Printf("Message %d rejected (tag=%d)\n", i, confirm.DeliveryTag)
			}
		case <-time.After(5 * time.Second):
			fmt.Printf("Message %d: confirm timeout\n", i)
		}
	}

	fmt.Println("Done")
}
