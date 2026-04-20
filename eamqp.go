package eamqp

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// MustNew creates a new client and panics on error.
func MustNew(config Config, opts ...Option) *Client {
	client, err := New(config, opts...)
	if err != nil {
		panic("eamqp: failed to create client: " + err.Error())
	}
	return client
}

// SimplePublish publishes a single message.
func SimplePublish(client *Client, exchange, routingKey string, body []byte) error {
	ch, err := client.NewChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	return ch.Publish(exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/octet-stream",
		DeliveryMode: Transient,
		Body:         body,
	})
}

// SimpleConsume returns a delivery channel for simple consumption.
func SimpleConsume(client *Client, queue, consumerTag string, autoAck bool) (<-chan amqp.Delivery, error) {
	ch, err := client.NewChannel()
	if err != nil {
		return nil, err
	}

	// Note: caller is responsible for closing the channel.
	return ch.Consume(queue, consumerTag, autoAck, false, false, false, nil)
}

// DeclareExchangeAndQueue declares an exchange and queue with binding.
func DeclareExchangeAndQueue(ch *Channel, exchange, kind, queue string, routingKeys []string, durable bool) (amqp.Queue, error) {
	// Declare exchange.
	if err := ch.ExchangeDeclare(exchange, kind, durable, false, false, false, nil); err != nil {
		return amqp.Queue{}, err
	}

	// Declare queue.
	q, err := ch.QueueDeclare(queue, durable, false, false, false, nil)
	if err != nil {
		return amqp.Queue{}, err
	}

	// Bind queue to exchange.
	for _, key := range routingKeys {
		if err := ch.QueueBind(queue, key, exchange, false, nil); err != nil {
			return amqp.Queue{}, err
		}
	}

	return q, nil
}

// DeclareWorkQueue declares a durable work queue.
func DeclareWorkQueue(ch *Channel, name string) (amqp.Queue, error) {
	return ch.QueueDeclare(name, true, false, false, false, nil)
}

// DeclarePubSub declares a fanout exchange with a unique queue.
func DeclarePubSub(ch *Channel, exchange string) (amqp.Queue, error) {
	if err := ch.ExchangeDeclare(exchange, ExchangeFanout, true, false, false, false, nil); err != nil {
		return amqp.Queue{}, err
	}

	// Exclusive queue - each subscriber gets its own.
	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		return amqp.Queue{}, err
	}

	if err := ch.QueueBind(q.Name, "", exchange, false, nil); err != nil {
		return amqp.Queue{}, err
	}

	return q, nil
}

// SimpleRPC performs a simple RPC-style request/response.
// It is a lightweight helper: the reply is acknowledged before the response
// body is returned, so callers that need acknowledgement control should build
// on Channel.Consume directly or use a higher-level RPC abstraction.
func SimpleRPC(client *Client, exchange, routingKey, replyTo string, body []byte, timeout time.Duration) ([]byte, error) {
	ch, err := client.NewChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	// Set up consumer first.
	deliveries, err := ch.Consume(replyTo, "", false, false, false, false, nil)
	if err != nil {
		return nil, err
	}

	// Publish request.
	err = ch.Publish(exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/octet-stream",
		DeliveryMode: Transient,
		ReplyTo:      replyTo,
		Body:         body,
	})
	if err != nil {
		return nil, err
	}

	// Wait for response.
	select {
	case delivery, ok := <-deliveries:
		if !ok {
			return nil, fmt.Errorf("eamqp: RPC reply consumer closed")
		}
		if err := delivery.Ack(false); err != nil {
			return nil, fmt.Errorf("eamqp: RPC reply ack failed: %w", err)
		}
		return delivery.Body, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("eamqp: RPC timeout after %v", timeout)
	}
}

// ReconnectLoop runs a reconnection loop, calling connect on each attempt.
// It blocks until the context is cancelled or reconnection is disabled.
func ReconnectLoop(ctx context.Context, client *Client, connect func() error) error {
	for {
		if err := connect(); err != nil {
			// Wait before retry.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}

		// Wait for connection close.
		closeChan := client.NotifyClose()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-closeChan:
			if err == nil {
				// Clean close.
				return nil
			}
			// Reconnect.
			continue
		}
	}
}
