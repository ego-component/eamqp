// reconnect demonstrates connection close notifications and the explicit
// reconnect boundary.
//
// eamqp exposes Reconnect() as a primitive, but it does not run a background
// supervisor that automatically rebuilds channels, exchanges, queues, bindings,
// and consumers. Production services should put that lifecycle in their own
// consumer/producer supervisor.
//
// Usage:
//
//	# Terminal 1 - start the reconnect consumer:
//	go run ./examples/reconnect --config=examples/config/local.toml consumer
//
//	# Terminal 2 - start the reconnect producer:
//	go run ./examples/reconnect --config=examples/config/local.toml producer
//
//	# Terminal 3 - while producer is running, kill the RabbitMQ container:
//	docker stop rabbitmq-dev
//	docker start rabbitmq-dev
//
//	Watch both terminals: close notifications are emitted and the current
//	channel/consumer must be rebuilt by application code.
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

const (
	exchange   = "reconnect.ex"
	queueName  = "reconnect.queue"
	routingKey = "reconnect.test"
)

func main() {
	args := exampleconfig.Args()
	if len(args) < 1 {
		fmt.Println("Usage: go run ./examples/reconnect --config=examples/config/local.toml <producer|consumer>")
		os.Exit(1)
	}
	role := args[0]

	client, err := exampleconfig.LoadClient("amqp.reconnect")
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ch, err := client.NewChannel()
	if err != nil {
		log.Fatalf("Failed to create channel: %v", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}
	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}
	if err := ch.QueueBind(queueName, routingKey, exchange, false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	// Monitor connection close events.
	closeChan := client.NotifyClose()
	go func() {
		for err := range closeChan {
			if err == nil {
				fmt.Println("Connection closed gracefully.")
			} else {
				fmt.Printf("Connection lost: %v. Reconnect() is explicit; rebuild channel/topology before resuming.\n", err)
			}
		}
	}()

	// Monitor channel close events.
	channelCloseChan := ch.NotifyClose()
	go func() {
		for err := range channelCloseChan {
			if err == nil {
				fmt.Println("Channel closed gracefully.")
			} else {
				fmt.Printf("Channel closed: %v\n", err)
			}
		}
	}()

	switch role {
	case "producer":
		runProducer(ch)
	case "consumer":
		runConsumer(ch)
	default:
		fmt.Printf("Unknown role: %s\n", role)
		os.Exit(1)
	}
}

func runProducer(ch *eamqp.Channel) {
	fmt.Println("Producer ready. Publishing messages every 2s.")
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
			fmt.Println("Producer shutdown.")
			return
		case <-ticker.C:
			i++
			body := fmt.Sprintf(`{"seq": %d, "ts": %d}`, i, time.Now().Unix())
			err := ch.Publish(exchange, routingKey, false, false, amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Body:         []byte(body),
			})
			if err != nil {
				log.Printf("Publish %d failed: %v", i, err)
				fmt.Println("Current channel may be broken; application supervisor should reconnect and rebuild topology.")
			} else {
				fmt.Printf("[%s] Published: %s\n", time.Now().Format("15:04:05"), body)
			}
		}
	}
}

func runConsumer(ch *eamqp.Channel) {
	deliveries, err := ch.Consume(queueName, "reconnect-consumer", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to start consuming: %v", err)
	}

	fmt.Println("Consumer ready. Waiting for messages...")
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
			fmt.Println("Consumer shutdown.")
			return
		case delivery, ok := <-deliveries:
			if !ok {
				fmt.Println("Delivery channel closed. Consumer will exit.")
				return
			}
			fmt.Printf("[%s] Received: %s\n", time.Now().Format("15:04:05"), string(delivery.Body))
			if err := delivery.Ack(false); err != nil {
				log.Printf("Ack failed: %v", err)
			}
		}
	}
}
