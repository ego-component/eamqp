// listener demonstrates automatic message retry with configurable max attempts
// using a worker pool for concurrent processing. Failed messages are requeued
// with a retry count header. After maxRetries, they are rejected to the DLQ.
//
// Usage:
//
//	# Terminal 1 - start the listener:
//	go run ./examples/retry-consumer-listener --config=examples/config/local.toml
//
//	# Terminal 2 - send messages:
//	go run ./examples/retry-consumer-sender --config=examples/config/local.toml
//
// Messages containing "fail" retry up to 3 times.
// Messages containing "die" go to DLQ immediately.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/ego-component/eamqp"
	"github.com/ego-component/eamqp/examples/internal/exampleconfig"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	exchange    = "retry.ex"
	queueName   = "retry.queue"
	retryHeader = "x-retry-count"
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

	if err := ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}
	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}
	if err := ch.QueueBind(queueName, "retry.test", exchange, false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	consumer := eamqp.NewConsumer(ch, queueName)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		cancel()
	}()

	maxRetries := 3
	fmt.Printf("Retry listener ready: maxRetries=%d, workers=5\n", maxRetries)
	fmt.Println("'fail' messages retry up to 3 times. 'die' messages go to DLQ immediately.")

	err = consumer.ConsumeWithWorkers(ctx, "retry-consumer", 5, func(delivery amqp.Delivery) error {
		body := string(delivery.Body)
		retryCount := getRetryCount(delivery.Headers)
		attempt := retryCount + 1

		if strings.Contains(body, "die") {
			fmt.Printf("[attempt=%d] UNRECOVERABLE, sending to DLQ: %s\n", attempt, body)
			delivery.Nack(false, false) // no requeue -> DLQ
			return nil
		}

		fmt.Printf("[attempt=%d] Processing: %s\n", attempt, body)

		if strings.Contains(body, "fail") {
			if retryCount >= maxRetries {
				fmt.Printf("[attempt=%d] Max retries exceeded, DLQ: %s\n", attempt, body)
				delivery.Nack(false, false)
				return nil
			}
			fmt.Printf("[attempt=%d] FAILED, will retry\n", attempt)
			// Ack original and publish retry message with incremented count.
			newHeaders := copyHeaders(delivery.Headers)
			newHeaders[retryHeader] = int64(retryCount + 1)
			delivery.Ack(false)
			ch.Publish(exchange, "retry.test", false, false, amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Headers:      newHeaders,
				Body:         []byte(body),
			})
			return nil
		}

		fmt.Printf("[attempt=%d] SUCCESS: %s\n", attempt, body)
		delivery.Ack(false)
		return nil
	})
	if err != nil && err != context.Canceled {
		log.Fatalf("Consumer error: %v", err)
	}
	fmt.Println("Listener shutdown.")
}

func getRetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	if v, ok := headers[retryHeader]; ok {
		switch n := v.(type) {
		case int64:
			return int(n)
		case int:
			return n
		case string:
			if i, err := strconv.Atoi(n); err == nil {
				return i
			}
		}
	}
	return 0
}

func copyHeaders(h amqp.Table) amqp.Table {
	if h == nil {
		return amqp.Table{}
	}
	result := make(amqp.Table, len(h))
	for k, v := range h {
		result[k] = v
	}
	return result
}
