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
	channel *Channel
	mu      sync.Mutex
	enabled bool
	timeout time.Duration
}

// NewProducer creates a new producer.
func NewProducer(ch *Channel, opts ...ProducerOption) (*Producer, error) {
	if ch == nil {
		return nil, fmt.Errorf("eamqp: producer channel is nil")
	}

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
	if err := p.validateChannel(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return p.channel.Publish(exchange, routingKey, mandatory, immediate, msg)
	}

	confirmation, err := p.channel.PublishWithDeferredConfirm(exchange, routingKey, mandatory, immediate, msg)
	if err != nil {
		return err
	}
	return p.waitForDeferredConfirm(context.Background(), confirmation)
}

// PublishAsync publishes without waiting for confirm.
func (p *Producer) PublishAsync(exchange, routingKey string, msg amqp.Publishing) (*amqp.DeferredConfirmation, error) {
	if err := p.validateChannel(); err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.channel.PublishWithDeferredConfirm(exchange, routingKey, false, false, msg)
}

// PublishWithContext publishes with context.
func (p *Producer) PublishWithContext(ctx context.Context, exchange, routingKey string, msg amqp.Publishing) error {
	return p.PublishWithContextOptions(ctx, exchange, routingKey, false, false, msg)
}

// PublishWithContextOptions publishes with context and full options.
func (p *Producer) PublishWithContextOptions(ctx context.Context, exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error {
	if err := p.validateChannel(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return p.channel.PublishWithContext(ctx, exchange, routingKey, mandatory, immediate, msg)
	}

	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, exchange, routingKey, mandatory, immediate, msg)
	if err != nil {
		return err
	}
	return p.waitForDeferredConfirm(ctx, confirmation)
}

// waitForDeferredConfirm waits for a publish confirmation without registering a NotifyPublish listener.
func (p *Producer) waitForDeferredConfirm(ctx context.Context, confirmation *amqp.DeferredConfirmation) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if confirmation == nil {
		return fmt.Errorf("eamqp: deferred confirmation is nil")
	}

	timer := time.NewTimer(p.timeout)
	defer timer.Stop()

	select {
	case <-confirmation.Done():
		return p.recordConfirmation(confirmation.Acked(), confirmation.DeliveryTag)
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return fmt.Errorf("eamqp: confirm timeout after %v", p.timeout)
	}
}

func (p *Producer) recordConfirmation(ack bool, deliveryTag uint64) error {
	if !ack {
		p.recordMessageNacked()
		return fmt.Errorf("eamqp: message not acknowledged (tag=%d)", deliveryTag)
	}
	p.recordMessageConfirmed()
	return nil
}

// Close closes the producer's channel.
func (p *Producer) Close() error {
	if err := p.validateChannel(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.channel.Close()
}

func (p *Producer) validateChannel() error {
	if p == nil || p.channel == nil {
		return fmt.Errorf("eamqp: producer channel is nil")
	}
	return nil
}

func (p *Producer) recordMessageConfirmed() {
	if p.channel == nil || p.channel.client == nil {
		return
	}
	if metrics := p.channel.client.GetMetrics(); metrics != nil {
		metrics.RecordMessageConfirmed()
	}
}

func (p *Producer) recordMessageNacked() {
	if p.channel == nil || p.channel.client == nil {
		return
	}
	if metrics := p.channel.client.GetMetrics(); metrics != nil {
		metrics.RecordMessageNacked()
	}
}

// BatchProducer provides batch publishing support.
type BatchProducer struct {
	producer   *Producer
	batch      []amqp.Publishing
	exchange   string
	routingKey string
	maxSize    int
	mu         sync.Mutex
}

// NewBatchProducer creates a batch producer.
func NewBatchProducer(ch *Channel, exchange, routingKey string, maxSize int, opts ...ProducerOption) (*BatchProducer, error) {
	if maxSize <= 0 {
		return nil, fmt.Errorf("eamqp: maxSize must be > 0, got %d", maxSize)
	}

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
// It does not block or fail when the batch reaches maxSize; callers should use
// ShouldFlush after Add and call Flush when it returns true.
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
