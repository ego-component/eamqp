package eamqp

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Producer provides high-level publishing with confirms support.
type Producer struct {
	channel   *Channel
	confirms  <-chan amqp.Confirmation
	mu        sync.Mutex
	enabled   bool
	timeout   time.Duration
}

// NewProducer creates a new producer.
func NewProducer(ch *Channel, opts ...ProducerOption) (*Producer, error) {
	p := &Producer{
		channel: ch,
		enabled: false,
		timeout: 5 * time.Second,
	}

	for _, opt := range opts {
		opt(p)
	}

	if p.enabled {
		if err := p.channel.Confirm(false); err != nil {
			return nil, err
		}
		p.confirms = p.channel.NotifyPublish()
	}

	return p, nil
}

// ProducerOption configures a producer.
type ProducerOption func(*Producer)

// WithConfirm enables publisher confirms with the given timeout.
func WithConfirm(timeout time.Duration) ProducerOption {
	return func(p *Producer) {
		p.enabled = true
		p.timeout = timeout
	}
}

// Publish publishes a message.
func (p *Producer) Publish(exchange, routingKey string, msg amqp.Publishing) error {
	return p.PublishWithOptions(exchange, routingKey, false, false, msg)
}

// PublishWithOptions publishes a message with full control.
func (p *Producer) PublishWithOptions(exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error {
	if err := p.channel.Publish(exchange, routingKey, mandatory, immediate, msg); err != nil {
		return err
	}

	// Wait for confirm if enabled.
	if p.enabled {
		return p.waitForConfirm()
	}

	return nil
}

// PublishAsync publishes without waiting for confirm.
func (p *Producer) PublishAsync(exchange, routingKey string, msg amqp.Publishing) (*amqp.DeferredConfirmation, error) {
	return p.channel.PublishWithDeferredConfirm(exchange, routingKey, false, false, msg)
}

// PublishWithContext publishes with context.
func (p *Producer) PublishWithContext(ctx context.Context, exchange, routingKey string, msg amqp.Publishing) error {
	return p.PublishWithContextOptions(ctx, exchange, routingKey, false, false, msg)
}

// PublishWithContextOptions publishes with context and full options.
func (p *Producer) PublishWithContextOptions(ctx context.Context, exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error {
	if err := p.channel.PublishWithContext(ctx, exchange, routingKey, mandatory, immediate, msg); err != nil {
		return err
	}

	// Wait for confirm if enabled.
	if p.enabled {
		return p.waitForConfirmWithContext(ctx)
	}

	return nil
}

// waitForConfirm waits for a publish confirmation.
func (p *Producer) waitForConfirm() error {
	select {
	case confirm, ok := <-p.confirms:
		if !ok {
			return fmt.Errorf("eamqp: confirm channel closed")
		}
		if !confirm.Ack {
			return fmt.Errorf("eamqp: message not acknowledged (tag=%d)", confirm.DeliveryTag)
		}
	case <-time.After(p.timeout):
		return fmt.Errorf("eamqp: confirm timeout after %v", p.timeout)
	}
	return nil
}

// waitForConfirmWithContext waits for a publish confirmation with context.
func (p *Producer) waitForConfirmWithContext(ctx context.Context) error {
	select {
	case confirm, ok := <-p.confirms:
		if !ok {
			return fmt.Errorf("eamqp: confirm channel closed")
		}
		if !confirm.Ack {
			return fmt.Errorf("eamqp: message not acknowledged")
		}
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(p.timeout):
		return fmt.Errorf("eamqp: confirm timeout after %v", p.timeout)
	}
	return nil
}

// Close closes the producer's channel.
func (p *Producer) Close() error {
	return p.channel.Close()
}

// BatchProducer provides batch publishing support.
type BatchProducer struct {
	producer   *Producer
	batch     []amqp.Publishing
	exchange  string
	routingKey string
	maxSize   int
	mu        sync.Mutex
}

// NewBatchProducer creates a batch producer.
func NewBatchProducer(ch *Channel, exchange, routingKey string, maxSize int, opts ...ProducerOption) (*BatchProducer, error) {
	producer, err := NewProducer(ch, opts...)
	if err != nil {
		return nil, err
	}

	return &BatchProducer{
		producer:   producer,
		batch:      make([]amqp.Publishing, 0, maxSize),
		exchange:   exchange,
		routingKey: routingKey,
		maxSize:    maxSize,
	}, nil
}

// Add adds a message to the batch.
func (bp *BatchProducer) Add(msg amqp.Publishing) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.batch = append(bp.batch, msg)
}

// ShouldFlush returns true if the batch should be flushed.
func (bp *BatchProducer) ShouldFlush() bool {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return len(bp.batch) >= bp.maxSize
}

// Size returns the current batch size.
func (bp *BatchProducer) Size() int {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return len(bp.batch)
}

// Flush publishes all batched messages.
func (bp *BatchProducer) Flush() error {
	bp.mu.Lock()
	batch := bp.batch
	bp.batch = make([]amqp.Publishing, 0, bp.maxSize)
	bp.mu.Unlock()

	for _, msg := range batch {
		if err := bp.producer.Publish(bp.exchange, bp.routingKey, msg); err != nil {
			return err
		}
	}

	return nil
}

// Close closes the batch producer.
func (bp *BatchProducer) Close() error {
	return bp.producer.Close()
}
