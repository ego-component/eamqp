// dead-letter demonstrates Dead Letter Exchange (DLX) and Dead Letter Queue (DLQ)
// patterns for handling failed message processing.
//
// How it works:
//  1. Message is consumed from the main queue
//  2. If processing fails and you Nack(requeue=false), broker routes it to the DLX
//  3. DLX routes it to the DLQ for later inspection/reprocessing
//  4. Messages can also expire (TTL) or be rejected by queue bounds
//
// Real-world use cases:
//   - Payment failures -> DLQ -> manual review by ops team
//   - Invalid message format -> DLQ -> data engineering fixes schema
//   - Order fulfillment failures -> DLQ -> human intervention
//
// Usage:
//
//	# Terminal 1 - start consumer:
//	go run ./examples/dead-letter --config=examples/config/local.toml consumer
//
//	# Terminal 2 - send messages:
//	go run ./examples/dead-letter --config=examples/config/local.toml publisher
//
//	# Terminal 3 - inspect DLQ:
//	go run ./examples/dead-letter --config=examples/config/local.toml inspect-dlq
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ego-component/eamqp"
	"github.com/ego-component/eamqp/examples/internal/exampleconfig"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	args := exampleconfig.Args()
	if len(args) < 1 {
		fmt.Println("Usage: go run ./examples/dead-letter --config=examples/config/local.toml <consumer|publisher|inspect-dlq>")
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

	if err := setupTopology(ch); err != nil {
		log.Fatalf("Setup failed: %v", err)
	}

	switch role {
	case "consumer":
		runConsumer(ch)
	case "publisher":
		runPublisher(ch)
	case "inspect-dlq":
		inspectDLQ(ch)
	default:
		fmt.Printf("Unknown role: %s\n", role)
		os.Exit(1)
	}
}

func setupTopology(ch *eamqp.Channel) error {
	const (
		exchange    = "dl.ex"
		dlxExchange = "dlx.ex"
		mainQueue   = "dl.main"
		dlqQueue    = "dl.dead"
		routingKey  = "dl.test"
		dlRouting   = "dl.dead"
	)

	if err := ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(dlxExchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(dlqQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind(dlqQueue, dlRouting, dlxExchange, false, nil); err != nil {
		return err
	}

	// Main queue with DLX settings: rejected/expired messages go to DLX.
	args := amqp.Table{
		"x-dead-letter-exchange":    dlxExchange,
		"x-dead-letter-routing-key": dlRouting,
	}
	if _, err := ch.QueueDeclare(mainQueue, true, false, false, false, args); err != nil {
		return err
	}
	return ch.QueueBind(mainQueue, routingKey, exchange, false, nil)
}

func runConsumer(ch *eamqp.Channel) {
	const mainQueue = "dl.main"
	deliveries, err := ch.Consume(mainQueue, "dl-consumer", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to start consuming: %v", err)
	}

	fmt.Println("Consumer ready. Main queue: dl.main")
	fmt.Println("  'die'  -> Nack(false,false) -> DLX -> DLQ")
	fmt.Println("  other -> Ack -> done")

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
				fmt.Println("Delivery channel closed.")
				return
			}
			body := string(delivery.Body)
			fmt.Printf("Processing: %s\n", body)

			if strings.Contains(body, "die") {
				// Reject without requeue -> broker sends to DLX -> DLQ.
				fmt.Printf("  -> REJECTED (to DLQ): %s\n", body)
				delivery.Nack(false, false)
			} else {
				fmt.Printf("  -> ACKED: %s\n", body)
				delivery.Ack(false)
			}
		}
	}
}

func runPublisher(ch *eamqp.Channel) {
	const exchange = "dl.ex"
	messages := []string{
		`{"order_id": 1, "status": "success"}`,
		`{"order_id": 2, "status": "die-now"}`,
		`{"order_id": 3, "status": "success"}`,
		`{"order_id": 4, "status": "die-now"}`,
		`{"order_id": 5, "status": "success"}`,
	}

	fmt.Println("Sending messages...")
	for i, msg := range messages {
		err := ch.Publish(exchange, "dl.test", false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         []byte(msg),
		})
		if err != nil {
			log.Printf("Publish %d failed: %v", i, err)
		} else {
			fmt.Printf("Sent: %s\n", msg)
		}
		time.Sleep(200 * time.Millisecond)
	}

	time.Sleep(2 * time.Second) // Let consumer process.

	fmt.Println("\n=== Queue Status ===")
	countMain := countQueue(ch, "dl.main")
	countDLQ := countQueue(ch, "dl.dead")
	fmt.Printf("Main queue (dl.main): %d messages\n", countMain)
	fmt.Printf("DLQ       (dl.dead): %d messages\n", countDLQ)
	fmt.Println("Expected: 3 success (acked), 2 die (DLQ)")
}

func inspectDLQ(ch *eamqp.Channel) {
	const dlqQueue = "dl.dead"
	fmt.Printf("Inspecting DLQ: %s\n", dlqQueue)

	deliveries, err := ch.Consume(dlqQueue, "dlq-inspect", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to consume DLQ: %v", err)
	}

	fmt.Println("DLQ messages:")
	i := 0
	for {
		select {
		case delivery, ok := <-deliveries:
			if !ok {
				fmt.Printf("Total DLQ messages inspected: %d\n", i)
				return
			}
			i++
			fmt.Printf("[%d] %s\n", i, string(delivery.Body))
			// Ack to remove from queue after inspection.
			delivery.Ack(false)
		case <-time.After(5 * time.Second):
			fmt.Printf("Timeout. Total DLQ messages inspected: %d\n", i)
			return
		}
	}
}

func countQueue(ch *eamqp.Channel, name string) int {
	// QueueDeclarePassive returns queue info without modifying it.
	q, err := ch.QueueDeclarePassive(name, true, false, false, false, nil)
	if err != nil {
		return 0
	}
	return q.Messages
}
