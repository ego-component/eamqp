// publisher sends tasks to the workqueue. Publishes a task every 2 seconds.
//
// Usage:
//
//	go run ./examples/workqueue-publisher --config=examples/config/local.toml
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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

	const queueName = "workqueue.tasks"
	_, err = ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		cancel()
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	fmt.Println("Publisher ready. Sending tasks every 2s. Press Ctrl+C to stop.")
	i := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nPublisher shutdown.")
			return
		case <-ticker.C:
			i++
			body := fmt.Sprintf(`{"task_id": %d, "description": "process item #%d", "timestamp": %d}`,
				i, i, time.Now().Unix())
			err := ch.Publish("", queueName, false, false, amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				ContentType:  "application/json",
				Body:         []byte(body),
			})
			if err != nil {
				log.Printf("Failed to publish task %d: %v", i, err)
				continue
			}
			fmt.Printf("[%s] Published task %d\n", time.Now().Format("15:04:05"), i)
		}
	}
}
