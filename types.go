package eamqp

import (
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// QueueArgs holds optional arguments for QueueDeclare.
type QueueArgs map[string]any

// NewQueueArgs creates a new QueueArgs with common defaults.
func NewQueueArgs() QueueArgs {
	return make(QueueArgs)
}

// WithDurable sets the durable argument.
func (a QueueArgs) WithDurable(durable bool) QueueArgs {
	if durable {
		a["x-queue-type"] = QueueTypeClassic
	}
	return a
}

// WithQueueType sets the queue type (classic, quorum, stream).
func (a QueueArgs) WithQueueType(qt string) QueueArgs {
	a[QueueTypeArg] = qt
	return a
}

// WithMaxLength sets the maximum number of messages.
func (a QueueArgs) WithMaxLength(n int) QueueArgs {
	a[QueueMaxLenArg] = n
	return a
}

// WithMaxLengthBytes sets the maximum total body size.
func (a QueueArgs) WithMaxLengthBytes(n int) QueueArgs {
	a[QueueMaxLenBytesArg] = n
	return a
}

// WithOverflow sets the overflow behavior.
func (a QueueArgs) WithOverflow(behavior string) QueueArgs {
	a[QueueOverflowArg] = behavior
	return a
}

// WithMessageTTL sets the per-message TTL in milliseconds.
func (a QueueArgs) WithMessageTTL(ttl time.Duration) QueueArgs {
	a[QueueMessageTTLArg] = int64(ttl.Milliseconds())
	return a
}

// WithQueueTTL sets the queue TTL (auto-delete after idle time).
func (a QueueArgs) WithQueueTTL(ttl time.Duration) QueueArgs {
	a[QueueTTLArg] = int64(ttl.Milliseconds())
	return a
}

// WithSingleActiveConsumer enables single active consumer.
func (a QueueArgs) WithSingleActiveConsumer() QueueArgs {
	a[SingleActiveConsumerArg] = true
	return a
}

// WithDeadLetterExchange sets the dead letter exchange.
func (a QueueArgs) WithDeadLetterExchange(dlx string) QueueArgs {
	a["x-dead-letter-exchange"] = dlx
	return a
}

// WithDeadLetterRoutingKey sets the dead letter routing key.
func (a QueueArgs) WithDeadLetterRoutingKey(key string) QueueArgs {
	a["x-dead-letter-routing-key"] = key
	return a
}

// ConsumerOption configures a consumer.
type ConsumerOption func(*consumerOptions)

type consumerOptions struct {
	autoAck   bool
	exclusive bool
	noLocal   bool
	noWait    bool
	args      amqp.Table
}

// WithConsumerAutoAck sets auto-acknowledge mode.
func WithConsumerAutoAck() ConsumerOption {
	return func(o *consumerOptions) {
		o.autoAck = true
	}
}

// WithConsumerExclusive sets exclusive consumer.
func WithConsumerExclusive() ConsumerOption {
	return func(o *consumerOptions) {
		o.exclusive = true
	}
}

// WithConsumerArgs sets consumer arguments.
func WithConsumerArgs(args amqp.Table) ConsumerOption {
	return func(o *consumerOptions) {
		o.args = args
	}
}

// PoolStats holds connection/channel pool statistics.
type PoolStats struct {
	ConnectionsActive int
	ConnectionsTotal int
	ChannelsActive   int
	ChannelsAcquired int64
	ChannelsReturned int64
	Reconnects      int64
}
