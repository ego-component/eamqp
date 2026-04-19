// qos demonstrates Quality of Service (QoS) settings for consumer-side message prefetching.
//
// Key concepts:
//   - prefetch count: max unacked messages delivered to consumer at once
//   - prefetch size: max total bytes (0 = unlimited)
//   - global=false: per-consumer limit; global=true: per-channel limit
//
// Usage:
//
//	# Terminal 1 - high-throughput consumer (prefetch=50):
//	go run ./examples/qos --config=examples/config/local.toml high
//
//	# Terminal 2 - fair consumer (prefetch=1):
//	go run ./examples/qos --config=examples/config/local.toml fair
//
//	# Terminal 3 - publish:
//	go run ./examples/qos --config=examples/config/local.toml publish
//
// Compare throughput and message distribution between the two consumers.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ego-component/eamqp"
	"github.com/ego-component/eamqp/examples/internal/exampleconfig"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	args := exampleconfig.Args()
	if len(args) < 1 {
		fmt.Println("Usage: go run ./examples/qos --config=examples/config/local.toml <high|fair|publish|inspect>")
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

	if err := ch.ExchangeDeclare("qos.ex", "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}
	if _, err := ch.QueueDeclare("qos.queue", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}
	if err := ch.QueueBind("qos.queue", "qos.test", "qos.ex", false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	switch role {
	case "high":
		runConsumer(ch, "high-throughput", 50, 0, false)
	case "fair":
		runConsumer(ch, "fair", 1, 0, false)
	case "publish":
		runPublisher(ch)
	case "inspect":
		inspectQueue(ch)
	default:
		fmt.Printf("Unknown role: %s\n", role)
		os.Exit(1)
	}
}

func runConsumer(ch *eamqp.Channel, name string, prefetchCount, prefetchSize int, global bool) {
	if err := ch.Qos(prefetchCount, prefetchSize, global); err != nil {
		log.Fatalf("Failed to set QoS: %v", err)
	}
	fmt.Printf("Consumer '%s' started. prefetchCount=%d, prefetchSize=%d, global=%v\n",
		name, prefetchCount, prefetchSize, global)

	deliveries, err := ch.Consume("qos.queue", name, false, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to start consuming: %v", err)
	}

	var processed int64
	startTime := time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		cancel()
	}()

	fmt.Println("Waiting for messages...")
	for {
		select {
		case <-ctx.Done():
			elapsed := time.Since(startTime)
			fmt.Printf("\nConsumer '%s' shutdown. Processed %d messages in %v (%.2f msg/s)\n",
				name, processed, elapsed, float64(processed)/elapsed.Seconds())
			return
		case delivery, ok := <-deliveries:
			if !ok {
				fmt.Println("Delivery channel closed.")
				return
			}
			time.Sleep(10 * time.Millisecond) // simulate work
			delivery.Ack(false)
			count := atomic.AddInt64(&processed, 1)
			if count%20 == 0 {
				fmt.Printf("  [%s] Processed: %d\n", time.Now().Format("15:04:05"), count)
			}
		}
	}
}

func runPublisher(ch *eamqp.Channel) {
	if n, _ := ch.QueuePurge("qos.queue", false); n > 0 {
		fmt.Printf("Purged %d old messages.\n", n)
	}

	const count = 100
	fmt.Printf("Publishing %d messages...\n", count)
	for i := 0; i < count; i++ {
		body := fmt.Sprintf(`{"seq": %d, "ts": %d}`, i, time.Now().UnixNano())
		if err := ch.Publish("qos.ex", "qos.test", false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         []byte(body),
		}); err != nil {
			log.Printf("Publish %d failed: %v", i, err)
		}
	}
	fmt.Printf("Published %d messages.\n", count)

	time.Sleep(500 * time.Millisecond)
	c := countQueue(ch, "qos.queue")
	fmt.Printf("Queue has %d messages remaining.\n", c)
}

func inspectQueue(ch *eamqp.Channel) {
	c := countQueue(ch, "qos.queue")
	fmt.Printf("Queue qos.queue: %d messages\n", c)
}

func countQueue(ch *eamqp.Channel, name string) int {
	q, err := ch.QueueDeclarePassive(name, true, false, false, false, nil)
	if err != nil {
		return 0
	}
	return q.Messages
}
