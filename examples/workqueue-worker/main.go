// worker is one of the workqueue example. Each worker processes tasks one at a time
// with fair dispatch (prefetch=1). Multiple workers can run concurrently.
//
// Usage:
//
//	  # Terminal 1 - start worker 1:
//
//		go run ./examples/workqueue-worker --config=examples/config/local.toml
//
//	  # Terminal 2 - start worker 2:
//
//		go run ./examples/workqueue-worker --config=examples/config/local.toml
//
//	  # Terminal 3 - publish tasks:
//
//		go run ./examples/workqueue-publisher --config=examples/config/local.toml
//
// If a worker dies mid-task, the message is redelivered to another worker.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
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

	// Fair dispatch: only prefetch 1 message at a time.
	if err := ch.Qos(1, 0, false); err != nil {
		log.Fatalf("Failed to set QoS: %v", err)
	}

	deliveries, err := ch.Consume(queueName, "worker-"+randomTag(6), false, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to start consuming: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		fmt.Println("\nReceived shutdown signal, finishing current task...")
		cancel()
	}()

	fmt.Println("Worker ready. Waiting for tasks...")
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Worker shutdown complete.")
			return
		case delivery, ok := <-deliveries:
			if !ok {
				fmt.Println("Delivery channel closed.")
				return
			}
			processTask(delivery)
		}
	}
}

func processTask(delivery amqp.Delivery) {
	taskDuration := time.Duration(1+rand.Intn(3)) * time.Second
	fmt.Printf("[%s] Received: %s (processing ~%v)\n",
		time.Now().Format("15:04:05"), string(delivery.Body), taskDuration)

	select {
	case <-time.After(taskDuration):
		if err := delivery.Ack(false); err != nil {
			log.Printf("Failed to ack: %v", err)
		}
		fmt.Printf("[%s] Done: %s\n", time.Now().Format("15:04:05"), string(delivery.Body))
	}
}

func randomTag(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
