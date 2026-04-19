// batch-producer demonstrates the high-level BatchProducer helper with publisher
// confirms enabled.
//
// This is a reliability comparison, not a throughput benchmark: the confirmed
// path waits for broker acknowledgements, so it can be slower than fire-and-forget
// publishing.
//
// Usage:
//
//	go run ./examples/batch-producer --config=examples/config/local.toml
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
		exchange   = "batch.ex"
		queueName  = "batch.queue"
		routingKey = "batch.test"
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

	// Purge for clean state.
	ch.QueuePurge(queueName, false)

	const count = 200

	// --- Plain publish: publish all messages without confirms (fire-and-forget).
	fmt.Printf("=== Unbatched: publishing %d messages (no confirms) ===\n", count)
	start := time.Now()
	for i := 0; i < count; i++ {
		body := fmt.Sprintf(`{"id": %d, "mode": "unbatched"}`, i)
		if err := ch.Publish(exchange, routingKey, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         []byte(body),
		}); err != nil {
			log.Printf("Publish %d failed: %v", i, err)
		}
	}
	unbatchedDuration := time.Since(start)
	fmt.Printf("Unbatched: %v (%.2f msg/s)\n", unbatchedDuration, float64(count)/unbatchedDuration.Seconds())

	// Purge before the confirmed path.
	ch.QueuePurge(queueName, false)

	// --- Confirmed batch helper: flush batches and wait for broker confirms.
	fmt.Printf("\n=== Batched: publishing %d messages in batches of %d ===\n", count, count/10)
	bp, err := eamqp.NewBatchProducer(ch, exchange, routingKey, count/10, eamqp.WithConfirm(5*time.Second))
	if err != nil {
		log.Fatalf("Failed to create batch producer: %v", err)
	}
	defer bp.Close()

	start = time.Now()
	for i := 0; i < count; i++ {
		body := fmt.Sprintf(`{"id": %d, "mode": "batched"}`, i)
		bp.Add(amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         []byte(body),
		})
		if bp.ShouldFlush() {
			if err := bp.Flush(); err != nil {
				log.Printf("Flush failed: %v", err)
			}
		}
	}
	if bp.Size() > 0 {
		if err := bp.Flush(); err != nil {
			log.Fatalf("Final flush failed: %v", err)
		}
	}
	batchedDuration := time.Since(start)
	fmt.Printf("Batched: %v (%.2f msg/s)\n", batchedDuration, float64(count)/batchedDuration.Seconds())

	fmt.Printf("\nUnbatched: %.2f msg/s\n", float64(count)/unbatchedDuration.Seconds())
	fmt.Printf("Batched:   %.2f msg/s\n", float64(count)/batchedDuration.Seconds())
	fmt.Println("Done.")
}
