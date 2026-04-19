package eamqp

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Channel wraps amqp.Channel with ego integration.
type Channel struct {
	amqpCh *amqp.Channel
	client ClientInterface
	mu     sync.RWMutex
}

// ClientInterface defines the methods needed by Channel.
type ClientInterface interface {
	GetLogger() Logger
	GetMetrics() MetricsCollector
}

// newChannel creates a new Channel wrapping an amqp.Channel.
func newChannel(amqpCh *amqp.Channel, client ClientInterface) *Channel {
	return &Channel{
		amqpCh: amqpCh,
		client: client,
	}
}

// Close closes the channel.
func (ch *Channel) Close() error {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.amqpCh == nil {
		return nil
	}

	err := ch.amqpCh.Close()
	ch.amqpCh = nil
	return err
}

// IsClosed returns true if the channel is closed.
func (ch *Channel) IsClosed() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.amqpCh == nil || ch.amqpCh.IsClosed()
}

// ExchangeDeclare declares an exchange.
func (ch *Channel) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	err := ch.amqpCh.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args)
	if err != nil {
		return err
	}

	if ch.client != nil {
		if log := ch.client.GetLogger(); log != nil {
			log.Debug("eamqp exchange declared", "name", name, "type", kind, "durable", durable)
		}
	}

	return nil
}

// ExchangeDeclarePassive declares an exchange (passive = check existence only).
func (ch *Channel) ExchangeDeclarePassive(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.ExchangeDeclarePassive(name, kind, durable, autoDelete, internal, noWait, args)
}

// ExchangeDelete deletes an exchange.
func (ch *Channel) ExchangeDelete(name string, ifUnused, noWait bool) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.ExchangeDelete(name, ifUnused, noWait)
}

// ExchangeBind binds an exchange to another exchange.
func (ch *Channel) ExchangeBind(destination, key, source string, noWait bool, args amqp.Table) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.ExchangeBind(destination, key, source, noWait, args)
}

// ExchangeUnbind unbinds an exchange from another exchange.
func (ch *Channel) ExchangeUnbind(destination, key, source string, noWait bool, args amqp.Table) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.ExchangeUnbind(destination, key, source, noWait, args)
}

// QueueDeclare declares a queue.
func (ch *Channel) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return amqp.Queue{}, fmt.Errorf("eamqp: channel is closed")
	}

	q, err := ch.amqpCh.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
	if err != nil {
		return amqp.Queue{}, err
	}

	if ch.client != nil {
		if log := ch.client.GetLogger(); log != nil {
			log.Debug("eamqp queue declared", "name", q.Name, "messages", q.Messages, "consumers", q.Consumers)
		}
	}

	return q, nil
}

// QueueDeclarePassive declares a queue (passive = check existence only).
func (ch *Channel) QueueDeclarePassive(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return amqp.Queue{}, fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.QueueDeclarePassive(name, durable, autoDelete, exclusive, noWait, args)
}

// QueueBind binds a queue to an exchange.
func (ch *Channel) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	err := ch.amqpCh.QueueBind(name, key, exchange, noWait, args)
	if err != nil {
		return err
	}

	if ch.client != nil {
		if log := ch.client.GetLogger(); log != nil {
			log.Debug("eamqp queue bound", "queue", name, "key", key, "exchange", exchange)
		}
	}

	return nil
}

// QueueUnbind unbinds a queue from an exchange.
func (ch *Channel) QueueUnbind(name, key, exchange string, args amqp.Table) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.QueueUnbind(name, key, exchange, args)
}

// QueuePurge purges all messages from a queue.
func (ch *Channel) QueuePurge(name string, noWait bool) (int, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return 0, fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.QueuePurge(name, noWait)
}

// QueueDelete deletes a queue.
func (ch *Channel) QueueDelete(name string, ifUnused, ifEmpty, noWait bool) (int, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return 0, fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.QueueDelete(name, ifUnused, ifEmpty, noWait)
}

// Publish publishes a message.
func (ch *Channel) Publish(exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	start := time.Now()
	err := ch.amqpCh.Publish(exchange, routingKey, mandatory, immediate, msg)
	duration := time.Since(start)

	if err != nil {
		if ch.client != nil {
			if log := ch.client.GetLogger(); log != nil {
				log.Error("eamqp publish failed", "exchange", exchange, "key", routingKey, "err", err)
			}
		}
		return err
	}

	if ch.client != nil {
		if m := ch.client.GetMetrics(); m != nil {
			m.RecordPublishLatency(duration)
		}
		if log := ch.client.GetLogger(); log != nil {
			log.Debug("eamqp published", "exchange", exchange, "key", routingKey, "size", len(msg.Body), "duration_ms", duration.Milliseconds())
		}
	}

	return nil
}

// PublishWithContext publishes a message with context.
func (ch *Channel) PublishWithContext(ctx context.Context, exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.PublishWithContext(ctx, exchange, routingKey, mandatory, immediate, msg)
}

// PublishWithDeferredConfirm publishes a message and returns a deferred confirmation.
func (ch *Channel) PublishWithDeferredConfirm(exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) (*amqp.DeferredConfirmation, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return nil, fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.PublishWithDeferredConfirm(exchange, routingKey, mandatory, immediate, msg)
}

// Consume starts consuming messages from a queue.
func (ch *Channel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return nil, fmt.Errorf("eamqp: channel is closed")
	}

	deliveries, err := ch.amqpCh.Consume(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
	if err != nil {
		return nil, err
	}

	if ch.client != nil {
		if log := ch.client.GetLogger(); log != nil {
			log.Info("eamqp consumer started", "queue", queue, "consumer", consumer, "auto_ack", autoAck)
		}
	}

	return deliveries, nil
}

// ConsumeWithContext starts consuming messages with context.
func (ch *Channel) ConsumeWithContext(ctx context.Context, queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return nil, fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.ConsumeWithContext(ctx, queue, consumer, autoAck, exclusive, noLocal, noWait, args)
}

// Cancel cancels a consumer.
func (ch *Channel) Cancel(consumer string, noWait bool) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	err := ch.amqpCh.Cancel(consumer, noWait)
	if err != nil {
		return err
	}

	if ch.client != nil {
		if log := ch.client.GetLogger(); log != nil {
			log.Info("eamqp consumer cancelled", "consumer", consumer)
		}
	}

	return nil
}

// Get synchronously retrieves a message from a queue.
func (ch *Channel) Get(queue string, autoAck bool) (amqp.Delivery, bool, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return amqp.Delivery{}, false, fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.Get(queue, autoAck)
}

// Qos sets QoS (Quality of Service) parameters.
func (ch *Channel) Qos(prefetchCount, prefetchSize int, global bool) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.Qos(prefetchCount, prefetchSize, global)
}

// Tx starts a transaction.
func (ch *Channel) Tx() error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.Tx()
}

// TxCommit commits a transaction.
func (ch *Channel) TxCommit() error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.TxCommit()
}

// TxRollback rolls back a transaction.
func (ch *Channel) TxRollback() error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.TxRollback()
}

// Confirm enables publisher confirms.
func (ch *Channel) Confirm(noWait bool) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.Confirm(noWait)
}

// GetNextPublishSeqNo returns the sequence number for the next publish.
func (ch *Channel) GetNextPublishSeqNo() uint64 {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return 0
	}
	return ch.amqpCh.GetNextPublishSeqNo()
}

// Ack acknowledges a message.
func (ch *Channel) Ack(tag uint64, multiple bool) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.Ack(tag, multiple)
}

// Nack negatively acknowledges a message.
func (ch *Channel) Nack(tag uint64, multiple, requeue bool) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.Nack(tag, multiple, requeue)
}

// Reject rejects a message.
func (ch *Channel) Reject(tag uint64, requeue bool) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.Reject(tag, requeue)
}

// Recover redelivers unacknowledged messages.
func (ch *Channel) Recover(requeue bool) error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		return fmt.Errorf("eamqp: channel is closed")
	}

	return ch.amqpCh.Recover(requeue)
}

// NotifyClose returns a channel that receives close notifications.
func (ch *Channel) NotifyClose() <-chan *amqp.Error {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		closed := make(chan *amqp.Error)
		close(closed)
		return closed
	}
	return ch.amqpCh.NotifyClose(make(chan *amqp.Error, 1))
}

// NotifyFlow returns a channel that receives flow control notifications.
func (ch *Channel) NotifyFlow() <-chan bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		closed := make(chan bool)
		close(closed)
		return closed
	}
	return ch.amqpCh.NotifyFlow(make(chan bool, 1))
}

// NotifyReturn returns a channel that receives undeliverable messages.
func (ch *Channel) NotifyReturn() <-chan amqp.Return {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		closed := make(chan amqp.Return)
		close(closed)
		return closed
	}
	return ch.amqpCh.NotifyReturn(make(chan amqp.Return, 1))
}

// NotifyCancel returns a channel that receives consumer cancel notifications.
func (ch *Channel) NotifyCancel() <-chan string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		closed := make(chan string)
		close(closed)
		return closed
	}
	return ch.amqpCh.NotifyCancel(make(chan string, 1))
}

// NotifyPublish returns a channel that receives publish confirmations.
func (ch *Channel) NotifyPublish() <-chan amqp.Confirmation {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		closed := make(chan amqp.Confirmation)
		close(closed)
		return closed
	}
	return ch.amqpCh.NotifyPublish(make(chan amqp.Confirmation, 1))
}

// NotifyConfirm returns ack and nack channels for publisher confirms.
func (ch *Channel) NotifyConfirm(ack, nack chan uint64) (chan uint64, chan uint64) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if ch.amqpCh == nil {
		close(ack)
		close(nack)
		return ack, nack
	}
	return ch.amqpCh.NotifyConfirm(ack, nack)
}
