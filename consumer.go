package eamqp

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// MessageHandler is a function that handles a delivery.
type MessageHandler func(delivery amqp.Delivery) error

const defaultConsumerShutdownTimeout = 5 * time.Second

// Consumer provides high-level message consumption.
type Consumer struct {
	channel *Channel
	queue   string
	opts    consumerOptions
}

// NewConsumer creates a new consumer.
func NewConsumer(ch *Channel, queue string, opts ...ConsumerOption) *Consumer {
	c := &Consumer{
		channel: ch,
		queue:   queue,
		opts:    consumerOptions{},
	}

	for _, opt := range opts {
		opt(&c.opts)
	}

	return c
}

// Consume starts consuming messages.
func (c *Consumer) Consume(consumerTag string) (<-chan amqp.Delivery, error) {
	return c.channel.Consume(
		c.queue,
		consumerTag,
		c.opts.autoAck,
		c.opts.exclusive,
		c.opts.noLocal,
		c.opts.noWait,
		c.opts.args,
	)
}

// ConsumeWithHandler starts consuming and processes messages with a handler.
func (c *Consumer) ConsumeWithHandler(ctx context.Context, consumerTag string, handler MessageHandler) error {
	deliveries, err := c.Consume(consumerTag)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return nil
			}

			if err := handler(delivery); err != nil {
				// Nack on error if not auto-ack.
				if !c.opts.autoAck {
					delivery.Nack(false, true)
				}
				continue
			}

			// Ack on success if not auto-ack.
			if !c.opts.autoAck {
				delivery.Ack(false)
			}
		}
	}
}

// ConsumeWithWorkers starts consuming with a worker pool.
func (c *Consumer) ConsumeWithWorkers(ctx context.Context, consumerTag string, numWorkers int, handler MessageHandler) error {
	if numWorkers <= 0 {
		numWorkers = 1
	}

	deliveries, err := c.Consume(consumerTag)
	if err != nil {
		return err
	}

	// Fan-out to workers.
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for delivery := range deliveries {
				if err := handler(delivery); err != nil {
					if !c.opts.autoAck {
						delivery.Nack(false, true)
					}
					continue
				}

				if !c.opts.autoAck {
					delivery.Ack(false)
				}
			}
		}()
	}

	// Wait for context cancellation.
	<-ctx.Done()

	cancelErr := c.channel.Cancel(consumerTag, false)
	closeErr := c.channel.Close()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(defaultConsumerShutdownTimeout):
		return fmt.Errorf("eamqp: consumer workers did not stop within %v", defaultConsumerShutdownTimeout)
	}

	if cancelErr != nil {
		return cancelErr
	}
	return closeErr
}

// ConsumeWithTimeout starts consuming with per-message timeout.
func (c *Consumer) ConsumeWithTimeout(consumerTag string, timeout time.Duration, handler MessageHandler) error {
	deliveries, err := c.Consume(consumerTag)
	if err != nil {
		return err
	}

	for delivery := range deliveries {
		done := make(chan error, 1)

		go func(d amqp.Delivery) {
			done <- handler(d)
		}(delivery)

		select {
		case err := <-done:
			if err != nil {
				if !c.opts.autoAck {
					delivery.Nack(false, true)
				}
				continue
			}

			if !c.opts.autoAck {
				delivery.Ack(false)
			}

		case <-time.After(timeout):
			// Timeout - reject and requeue.
			if !c.opts.autoAck {
				delivery.Nack(false, true)
			}
		}
	}

	return nil
}

// RetryConsumer provides consumption with automatic retry.
type RetryConsumer struct {
	consumer   *Consumer
	maxRetries int
	retryDelay time.Duration
}

// NewRetryConsumer creates a retry consumer.
func NewRetryConsumer(ch *Channel, queue string, maxRetries int, retryDelay time.Duration, opts ...ConsumerOption) (*RetryConsumer, error) {
	consumer := NewConsumer(ch, queue, opts...)

	return &RetryConsumer{
		consumer:   consumer,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}, nil
}

// ConsumeWithRetry starts consuming with automatic retry.
func (rc *RetryConsumer) ConsumeWithRetry(ctx context.Context, consumerTag string, handler MessageHandler) error {
	return rc.consumer.ConsumeWithHandler(ctx, consumerTag, func(delivery amqp.Delivery) error {
		var lastErr error
		for attempt := 0; attempt <= rc.maxRetries; attempt++ {
			if attempt > 0 {
				time.Sleep(rc.retryDelay)
			}

			err := handler(delivery)
			if err == nil {
				return nil
			}
			lastErr = err
		}

		return fmt.Errorf("eamqp: max retries exceeded: %w", lastErr)
	})
}

// Cancel cancels the consumer.
func (c *Consumer) Cancel(consumerTag string) error {
	return c.channel.Cancel(consumerTag, false)
}

// Close closes the consumer.
func (c *Consumer) Close() error {
	return c.channel.Close()
}
