// rpc demonstrates RPC-style request/response over RabbitMQ using the Direct Reply-To
// pattern. The client creates an exclusive reply queue for each RPC call, and the
// server publishes the response through the exchange with the correlation ID.
//
// Key concepts:
//   - CorrelationId: links request and response
//   - ReplyTo: tells the server which queue to reply to
//   - Exclusive reply queue: auto-deleted when client disconnects
//
// Usage:
//
//	go run ./examples/rpc --config=examples/config/local.toml
//
// The server starts in a goroutine and processes 5 concurrent RPC calls.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ego-component/eamqp/examples/internal/exampleconfig"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	exchange = "rpc.exchange"
	rpcQueue = "rpc.queue"
)

func main() {
	client, err := exampleconfig.LoadClient(exampleconfig.DefaultComponentKey)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Server-side channel: declares exchange, queue, and starts consuming.
	serverCh, err := client.NewChannel()
	if err != nil {
		log.Fatalf("Failed to create server channel: %v", err)
	}
	defer serverCh.Close()

	if err := serverCh.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}
	if _, err := serverCh.QueueDeclare(rpcQueue, true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}
	if err := serverCh.QueueBind(rpcQueue, rpcQueue, exchange, false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	// Start RPC server.
	go func() {
		deliveries, err := serverCh.Consume(rpcQueue, "", false, false, false, false, nil)
		if err != nil {
			log.Fatalf("Failed to start consuming: %v", err)
		}

		for delivery := range deliveries {
			request := string(delivery.Body)
			fmt.Printf("Server received: %s\n", request)

			response, err := json.Marshal(struct {
				Result   string `json:"result"`
				ServerTS int64  `json:"server_ts"`
			}{
				Result:   fmt.Sprintf("processed %s", request),
				ServerTS: time.Now().Unix(),
			})
			if err != nil {
				log.Printf("Failed to encode response: %v", err)
				delivery.Nack(false, true)
				continue
			}

			// Publish response through the exchange using the reply-to queue name as routing key.
			// The reply queue must be bound to the exchange with the same routing key.
			replyTo := delivery.ReplyTo
			if replyTo == "" {
				replyTo = "default.reply"
			}
			err = serverCh.Publish(exchange, replyTo, false, false, amqp.Publishing{
				ContentType:   "application/json",
				DeliveryMode:  amqp.Persistent,
				CorrelationId: delivery.CorrelationId,
				Body:          response,
			})
			if err != nil {
				log.Printf("Failed to send response: %v", err)
				delivery.Nack(false, true)
				continue
			}
			delivery.Ack(false)
		}
	}()

	// Make 5 concurrent RPC calls.
	var wg sync.WaitGroup
	var successCount int64
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each call uses its own channel and exclusive reply queue.
			ch, err := client.NewChannel()
			if err != nil {
				log.Printf("Call %d: failed to create channel: %v", id, err)
				return
			}
			defer ch.Close()

			// Exclusive reply queue: auto-deleted when this channel closes.
			replyQ, err := ch.QueueDeclare("", false, true, true, false, nil)
			if err != nil {
				log.Printf("Call %d: failed to declare reply queue: %v", id, err)
				return
			}
			// Bind the exclusive reply queue to the exchange so server can publish to it.
			if err := ch.QueueBind(replyQ.Name, replyQ.Name, exchange, false, nil); err != nil {
				log.Printf("Call %d: failed to bind reply queue: %v", id, err)
				return
			}

			// Start consuming replies.
			deliveries, err := ch.Consume(replyQ.Name, "", false, false, false, false, nil)
			if err != nil {
				log.Printf("Call %d: failed to start consumer: %v", id, err)
				return
			}

			// Send request.
			request := fmt.Sprintf(`{"request_id": %d, "action": "process"}`, id)
			correlationId := fmt.Sprintf("corr-%d", id)
			err = ch.Publish(exchange, rpcQueue, false, false, amqp.Publishing{
				ContentType:   "application/json",
				DeliveryMode:  amqp.Persistent,
				CorrelationId: correlationId,
				ReplyTo:       replyQ.Name,
				Body:          []byte(request),
			})
			if err != nil {
				log.Printf("Call %d: failed to send request: %v", id, err)
				return
			}
			fmt.Printf("Client %d sent: %s\n", id, request)

			// Wait for response with timeout.
			select {
			case delivery, ok := <-deliveries:
				if !ok {
					fmt.Printf("Client %d: channel closed\n", id)
					return
				}
				if delivery.CorrelationId == correlationId {
					fmt.Printf("Client %d received: %s\n", id, string(delivery.Body))
					atomic.AddInt64(&successCount, 1)
					delivery.Ack(false)
				}
			case <-ctx.Done():
				fmt.Printf("Client %d: timeout\n", id)
			}
		}(i)
	}

	wg.Wait()
	fmt.Printf("\nRPC complete. Success: %d/5\n", atomic.LoadInt64(&successCount))
}
