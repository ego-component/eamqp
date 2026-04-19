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
	amqpCh   *amqp.Channel
	client   ClientInterface
	release  func()
	discard  func()
	stateful bool
	closing  bool
	inflight sync.WaitGroup
	mu       sync.Mutex
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

func newChannelWithRelease(amqpCh *amqp.Channel, client ClientInterface, release func(), discard func()) *Channel {
	return &Channel{
		amqpCh:  amqpCh,
		client:  client,
		release: release,
		discard: discard,
	}
}

// RawChannel returns the underlying AMQP channel.
// The wrapper treats raw access as stateful and will discard pooled channels on Close.
func (ch *Channel) RawChannel() *amqp.Channel {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.amqpCh != nil {
		ch.stateful = true
	}
	return ch.amqpCh
}

// Close closes the channel.
func (ch *Channel) Close() error {
	ch.mu.Lock()
	if ch.amqpCh == nil {
		ch.mu.Unlock()
		return nil
	}

	amqpCh := ch.amqpCh
	release := ch.release
	discard := ch.discard
	stateful := ch.stateful
	ch.closing = true
	ch.amqpCh = nil
	ch.release = nil
	ch.discard = nil
	ch.stateful = false
	ch.mu.Unlock()

	ch.inflight.Wait()

	if release != nil {
		if stateful && discard != nil {
			discard()
			return nil
		}
		release()
		return nil
	}

	err := amqpCh.Close()
	return err
}

// IsClosed returns true if the channel is closed.
func (ch *Channel) IsClosed() bool {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.amqpCh == nil || ch.amqpCh.IsClosed()
}

func (ch *Channel) acquireChannel(stateful bool) (*amqp.Channel, func(), error) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.amqpCh == nil || ch.closing {
		return nil, nil, fmt.Errorf("eamqp: channel is closed")
	}
	if stateful {
		ch.stateful = true
	}
	ch.inflight.Add(1)
	return ch.amqpCh, ch.inflight.Done, nil
}

// ExchangeDeclare declares an exchange.
func (ch *Channel) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	err = amqpCh.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args)
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
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.ExchangeDeclarePassive(name, kind, durable, autoDelete, internal, noWait, args)
}

// ExchangeDelete deletes an exchange.
func (ch *Channel) ExchangeDelete(name string, ifUnused, noWait bool) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.ExchangeDelete(name, ifUnused, noWait)
}

// ExchangeBind binds an exchange to another exchange.
func (ch *Channel) ExchangeBind(destination, key, source string, noWait bool, args amqp.Table) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.ExchangeBind(destination, key, source, noWait, args)
}

// ExchangeUnbind unbinds an exchange from another exchange.
func (ch *Channel) ExchangeUnbind(destination, key, source string, noWait bool, args amqp.Table) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.ExchangeUnbind(destination, key, source, noWait, args)
}

// QueueDeclare declares a queue.
func (ch *Channel) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return amqp.Queue{}, err
	}
	defer done()

	q, err := amqpCh.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
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
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return amqp.Queue{}, err
	}
	defer done()

	return amqpCh.QueueDeclarePassive(name, durable, autoDelete, exclusive, noWait, args)
}

// QueueInspect passively inspects a queue by name.
func (ch *Channel) QueueInspect(name string) (amqp.Queue, error) {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return amqp.Queue{}, err
	}
	defer done()

	return amqpCh.QueueInspect(name)
}

// QueueBind binds a queue to an exchange.
func (ch *Channel) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	err = amqpCh.QueueBind(name, key, exchange, noWait, args)
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
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.QueueUnbind(name, key, exchange, args)
}

// QueuePurge purges all messages from a queue.
func (ch *Channel) QueuePurge(name string, noWait bool) (int, error) {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return 0, err
	}
	defer done()

	return amqpCh.QueuePurge(name, noWait)
}

// QueueDelete deletes a queue.
func (ch *Channel) QueueDelete(name string, ifUnused, ifEmpty, noWait bool) (int, error) {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return 0, err
	}
	defer done()

	return amqpCh.QueueDelete(name, ifUnused, ifEmpty, noWait)
}

// Publish publishes a message.
func (ch *Channel) Publish(exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	start := time.Now()
	err = amqpCh.Publish(exchange, routingKey, mandatory, immediate, msg)
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
			m.RecordMessagePublished(len(msg.Body))
			m.RecordPublishLatency(duration)
		}
		if log := ch.client.GetLogger(); log != nil {
			log.Debug("eamqp published", "exchange", exchange, "key", routingKey, "size", len(msg.Body), "duration_ms", duration.Milliseconds())
		}
	}

	return nil
}

// PublishWithContext publishes a message with context.
// Context currently carries trace propagation into AMQP headers; amqp091-go
// v1.9.0 does not guarantee publish timeout or cancellation from this context.
// Use publisher confirms when the caller must know whether the broker accepted
// the message.
func (ch *Channel) PublishWithContext(ctx context.Context, exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	var endTrace func()
	ctx, msg.Headers, endTrace = startPublishTrace(ctx, msg.Headers, ch.traceEnabled())
	defer endTrace()

	start := time.Now()
	err = amqpCh.PublishWithContext(ctx, exchange, routingKey, mandatory, immediate, msg)
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
			m.RecordMessagePublished(len(msg.Body))
			m.RecordPublishLatency(duration)
		}
	}
	return nil
}

// PublishWithDeferredConfirm publishes a message and returns a deferred confirmation.
func (ch *Channel) PublishWithDeferredConfirm(exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) (*amqp.DeferredConfirmation, error) {
	return ch.PublishWithDeferredConfirmWithContext(context.Background(), exchange, routingKey, mandatory, immediate, msg)
}

// PublishWithDeferredConfirmWithContext publishes and returns a deferred confirm.
// Context currently carries trace propagation into AMQP headers; amqp091-go
// v1.9.0 also accepts the context for future cancellation semantics.
func (ch *Channel) PublishWithDeferredConfirmWithContext(ctx context.Context, exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) (*amqp.DeferredConfirmation, error) {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return nil, err
	}
	defer done()

	var endTrace func()
	ctx, msg.Headers, endTrace = startPublishTrace(ctx, msg.Headers, ch.traceEnabled())
	defer endTrace()

	start := time.Now()
	confirmation, err := amqpCh.PublishWithDeferredConfirmWithContext(ctx, exchange, routingKey, mandatory, immediate, msg)
	duration := time.Since(start)
	if err != nil {
		if ch.client != nil {
			if log := ch.client.GetLogger(); log != nil {
				log.Error("eamqp publish failed", "exchange", exchange, "key", routingKey, "err", err)
			}
		}
		return nil, err
	}
	if ch.client != nil {
		if m := ch.client.GetMetrics(); m != nil {
			m.RecordMessagePublished(len(msg.Body))
			m.RecordPublishLatency(duration)
		}
	}
	return confirmation, nil
}

func (ch *Channel) traceEnabled() bool {
	if ch == nil || ch.client == nil {
		return true
	}
	if client, ok := ch.client.(interface{ traceEnabled() bool }); ok {
		return client.traceEnabled()
	}
	return true
}

// Consume starts consuming messages from a queue.
func (ch *Channel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		return nil, err
	}
	defer done()

	deliveries, err := amqpCh.Consume(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
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
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		return nil, err
	}
	defer done()

	deliveries, err := amqpCh.ConsumeWithContext(ctx, queue, consumer, autoAck, exclusive, noLocal, noWait, args)
	if err != nil {
		return nil, err
	}
	return deliveries, nil
}

// Cancel cancels a consumer.
func (ch *Channel) Cancel(consumer string, noWait bool) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	err = amqpCh.Cancel(consumer, noWait)
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
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return amqp.Delivery{}, false, err
	}
	defer done()

	return amqpCh.Get(queue, autoAck)
}

// Qos sets QoS (Quality of Service) parameters.
func (ch *Channel) Qos(prefetchCount, prefetchSize int, global bool) error {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		return err
	}
	defer done()

	if err := amqpCh.Qos(prefetchCount, prefetchSize, global); err != nil {
		return err
	}
	return nil
}

// Flow pauses or resumes deliveries on this channel.
func (ch *Channel) Flow(active bool) error {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		return err
	}
	defer done()

	if err := amqpCh.Flow(active); err != nil {
		return err
	}
	return nil
}

// Tx starts a transaction.
func (ch *Channel) Tx() error {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		return err
	}
	defer done()

	if err := amqpCh.Tx(); err != nil {
		return err
	}
	return nil
}

// TxCommit commits a transaction.
func (ch *Channel) TxCommit() error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.TxCommit()
}

// TxRollback rolls back a transaction.
func (ch *Channel) TxRollback() error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.TxRollback()
}

// Confirm enables publisher confirms.
func (ch *Channel) Confirm(noWait bool) error {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		return err
	}
	defer done()

	if err := amqpCh.Confirm(noWait); err != nil {
		return err
	}
	return nil
}

// GetNextPublishSeqNo returns the sequence number for the next publish.
func (ch *Channel) GetNextPublishSeqNo() uint64 {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return 0
	}
	defer done()
	return amqpCh.GetNextPublishSeqNo()
}

// Ack acknowledges a message.
func (ch *Channel) Ack(tag uint64, multiple bool) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.Ack(tag, multiple)
}

// Nack negatively acknowledges a message.
func (ch *Channel) Nack(tag uint64, multiple, requeue bool) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.Nack(tag, multiple, requeue)
}

// Reject rejects a message.
func (ch *Channel) Reject(tag uint64, requeue bool) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.Reject(tag, requeue)
}

// Recover redelivers unacknowledged messages.
func (ch *Channel) Recover(requeue bool) error {
	amqpCh, done, err := ch.acquireChannel(false)
	if err != nil {
		return err
	}
	defer done()

	return amqpCh.Recover(requeue)
}

// NotifyClose returns a channel that receives close notifications.
// The returned channel must be consumed until it is closed, matching
// amqp091-go's asynchronous notification contract.
func (ch *Channel) NotifyClose() <-chan *amqp.Error {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		closed := make(chan *amqp.Error)
		close(closed)
		return closed
	}
	defer done()
	return amqpCh.NotifyClose(make(chan *amqp.Error, 1))
}

// NotifyFlow returns a channel that receives flow control notifications.
func (ch *Channel) NotifyFlow() <-chan bool {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		closed := make(chan bool)
		close(closed)
		return closed
	}
	defer done()
	return amqpCh.NotifyFlow(make(chan bool, 1))
}

// NotifyReturn returns a channel that receives undeliverable messages.
func (ch *Channel) NotifyReturn() <-chan amqp.Return {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		closed := make(chan amqp.Return)
		close(closed)
		return closed
	}
	defer done()
	return amqpCh.NotifyReturn(make(chan amqp.Return, 1))
}

// NotifyCancel returns a channel that receives consumer cancel notifications.
func (ch *Channel) NotifyCancel() <-chan string {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		closed := make(chan string)
		close(closed)
		return closed
	}
	defer done()
	return amqpCh.NotifyCancel(make(chan string, 1))
}

// NotifyPublish returns a channel that receives publish confirmations.
func (ch *Channel) NotifyPublish() <-chan amqp.Confirmation {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		closed := make(chan amqp.Confirmation)
		close(closed)
		return closed
	}
	defer done()
	return amqpCh.NotifyPublish(make(chan amqp.Confirmation, 1))
}

// NotifyConfirm returns ack and nack channels for publisher confirms.
func (ch *Channel) NotifyConfirm(ack, nack chan uint64) (chan uint64, chan uint64) {
	amqpCh, done, err := ch.acquireChannel(true)
	if err != nil {
		close(ack)
		close(nack)
		return ack, nack
	}
	defer done()
	return amqpCh.NotifyConfirm(ack, nack)
}
