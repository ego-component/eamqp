// connection-pool demonstrates high-availability connection pooling with round-robin
// load distribution across multiple connections.
//
// This is useful when a single connection becomes a bottleneck under high concurrency.
// Note: RabbitMQ recommends keeping connections small and using many channels per
// connection. This example shows how the pool works when PoolSize > 1.
//
// Usage:
//
//	go run ./examples/connection-pool --config=examples/config/local.toml
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ego-component/eamqp/examples/internal/exampleconfig"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	client, err := exampleconfig.LoadClient("amqp.pool")
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ch, err := client.NewChannel()
	if err != nil {
		log.Fatalf("Failed to create channel: %v", err)
	}
	if err := ch.ExchangeDeclare("pool.ex", "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}
	if _, err := ch.QueueDeclare("pool.queue", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}
	if err := ch.QueueBind("pool.queue", "pool.test", "pool.ex", false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}
	ch.Close()

	// Acquire channels concurrently to demonstrate pool behavior.
	fmt.Println("=== Acquiring 20 channels concurrently ===")
	const numChannels = 20
	var acquired int64

	var wg sync.WaitGroup
	for i := 0; i < numChannels; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			poolCh, err := client.NewChannel()
			if err != nil {
				log.Printf("Failed to acquire channel %d: %v", idx, err)
				return
			}
			atomic.AddInt64(&acquired, 1)
			defer poolCh.Close()

			err = poolCh.Publish("pool.ex", "pool.test", false, false, amqp.Publishing{
				ContentType:  "text/plain",
				DeliveryMode: amqp.Persistent,
				Body:         []byte(fmt.Sprintf(`{"channel": %d}`, idx)),
			})
			if err != nil {
				log.Printf("Publish on channel %d failed: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
	fmt.Printf("Acquired %d/%d channels\n", acquired, numChannels)

	// Show pool stats.
	fmt.Println("\n=== Pool Stats ===")
	stats := client.Stats()
	fmt.Printf("Connections active: %d\n", stats.ConnectionsActive)
	fmt.Printf("Connections total:  %d\n", stats.ConnectionsTotal)
	fmt.Printf("Reconnects:        %d\n", stats.Reconnects)

	// Demonstrate explicit AcquireChannel API.
	fmt.Println("\n=== AcquireChannel (explicit pool API) ===")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	poolCh, release, err := client.AcquireChannel(ctx)
	if err != nil {
		log.Printf("AcquireChannel failed (pool may not be enabled): %v", err)
	} else {
		fmt.Println("Acquired channel from pool.")
		err = poolCh.Publish("pool.ex", "pool.test", false, false, amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte("hello from acquire"),
		})
		if err != nil {
			log.Printf("Publish failed: %v", err)
		}
		release()
		fmt.Println("Released channel back to pool.")
	}

	fmt.Println("\nDone.")
}
