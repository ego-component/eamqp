// rpc example demonstrates RPC-style communication over RabbitMQ.
package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/ego-component/eamqp"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	exchange   = "rpc.exchange"
	rpcQueue   = "rpc.queue"
	replyQueue = "rpc.reply"
)

func main() {
	// Create client.
	client, err := eamqp.New(eamqp.Config{
		Addr: getAddr(),
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create RPC channel.
	rpcCh, err := client.NewChannel()
	if err != nil {
		log.Fatalf("Failed to create channel: %v", err)
	}
	defer rpcCh.Close()

	// Declare exchange.
	if err := rpcCh.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	// Declare RPC queue.
	_, err = rpcCh.QueueDeclare(rpcQueue, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Declare reply queue.
	replyQ, err := rpcCh.QueueDeclare(replyQueue, true, false, true, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare reply queue: %v", err)
	}

	// Bind RPC queue.
	if err := rpcCh.QueueBind(rpcQueue, rpcQueue, exchange, false, nil); err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	// Start RPC server.
	go func() {
		deliveries, err := rpcCh.Consume(rpcQueue, "", false, false, false, false, nil)
		if err != nil {
			log.Fatalf("Failed to start consuming: %v", err)
		}

		for delivery := range deliveries {
			// Process request.
			request := string(delivery.Body)
			fmt.Printf("RPC Request: %s\n", request)

			// Generate response.
			response := fmt.Sprintf(`{"result": "processed %s", "id": %d}`, request, rand.Int())

			// Get reply-to queue.
			replyTo := delivery.ReplyTo
			if replyTo == "" {
				replyTo = replyQ.Name
			}

			// Send response.
			err = rpcCh.Publish(exchange, replyTo, false, false, amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				CorrelationId: delivery.CorrelationId,
				Body:         []byte(response),
			})
			if err != nil {
				log.Printf("Failed to send response: %v", err)
				delivery.Nack(false, true)
				continue
			}

			delivery.Ack(false)
		}
	}()

	// Make RPC calls.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Create dedicated channel for this RPC.
			ch, err := client.NewChannel()
			if err != nil {
				log.Printf("Failed to create channel: %v", err)
				return
			}
			defer ch.Close()

			// Set up reply consumer.
			deliveries, err := ch.Consume(replyQ.Name, "", true, false, false, false, nil)
			if err != nil {
				log.Printf("Failed to start reply consumer: %v", err)
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
				log.Printf("Failed to send request: %v", err)
				return
			}

			// Wait for response.
			select {
			case delivery := <-deliveries:
				fmt.Printf("RPC Response [%d]: %s\n", id, string(delivery.Body))
			case <-time.After(5 * time.Second):
				fmt.Printf("RPC Response [%d]: timeout\n", id)
			}
		}(i)
	}

	wg.Wait()
	fmt.Println("All RPC calls completed")
}

func getAddr() string {
	if addr := os.Getenv("AMQP_ADDR"); addr != "" {
		return addr
	}
	return "amqp://guest:guest@localhost:5672/"
}
