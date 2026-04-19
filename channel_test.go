package eamqp

import (
	"context"
	"os"
	"strings"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

func TestChannel_IsClosed(t *testing.T) {
	t.Run("nil channel", func(t *testing.T) {
		ch := &Channel{amqpCh: nil}
		assert.True(t, ch.IsClosed())
	})
}

func TestChannel_Close(t *testing.T) {
	t.Run("nil channel", func(t *testing.T) {
		ch := &Channel{amqpCh: nil}
		err := ch.Close()
		assert.NoError(t, err)
	})
}

func TestChannel_CloseTwice(t *testing.T) {
	t.Run("nil channel after first close", func(t *testing.T) {
		ch := &Channel{amqpCh: nil}
		err := ch.Close()
		assert.NoError(t, err)
		err = ch.Close()
		assert.NoError(t, err)
	})
}

func TestChannel_RawChannel(t *testing.T) {
	ch := &Channel{}

	assert.Nil(t, ch.RawChannel())
}

func TestChannelCloseUsesReleaseOnce(t *testing.T) {
	calls := 0
	ch := &Channel{
		amqpCh:  &amqp.Channel{},
		release: func() { calls++ },
	}

	assert.NoError(t, ch.Close())
	assert.NoError(t, ch.Close())
	assert.Nil(t, ch.RawChannel())
	assert.Equal(t, 1, calls)
}

func TestChannelCloseDiscardsStatefulPooledChannel(t *testing.T) {
	releaseCalls := 0
	discardCalls := 0
	ch := &Channel{
		amqpCh:   &amqp.Channel{},
		release:  func() { releaseCalls++ },
		discard:  func() { discardCalls++ },
		stateful: true,
	}

	assert.NoError(t, ch.Close())
	assert.NoError(t, ch.Close())
	assert.Equal(t, 0, releaseCalls)
	assert.Equal(t, 1, discardCalls)
}

func TestRawChannelMarksPooledChannelStateful(t *testing.T) {
	ch := &Channel{amqpCh: &amqp.Channel{}}

	assert.NotNil(t, ch.RawChannel())
	assert.True(t, ch.stateful)
}

func TestChannelAcquirePreventsCloseRace(t *testing.T) {
	ch := &Channel{
		amqpCh:  &amqp.Channel{},
		release: func() {},
	}

	amqpCh, done, err := ch.acquireChannel(false)
	assert.NoError(t, err)
	assert.NotNil(t, amqpCh)

	closed := make(chan error, 1)
	go func() {
		closed <- ch.Close()
	}()

	select {
	case <-closed:
		t.Fatal("Close returned while a wrapper operation was still active")
	default:
	}

	done()
	assert.NoError(t, <-closed)
	assert.True(t, ch.IsClosed())
}

func TestChannelAcquireStatefulMarksBeforeClose(t *testing.T) {
	releaseCalls := 0
	discardCalls := 0
	ch := &Channel{
		amqpCh:  &amqp.Channel{},
		release: func() { releaseCalls++ },
		discard: func() { discardCalls++ },
	}

	_, done, err := ch.acquireChannel(true)
	assert.NoError(t, err)
	done()

	assert.NoError(t, ch.Close())
	assert.Equal(t, 0, releaseCalls)
	assert.Equal(t, 1, discardCalls)
}

func TestChannelOperationsDoNotHoldStateMutexDuringAMQPCall(t *testing.T) {
	data, err := os.ReadFile("channel.go")
	assert.NoError(t, err)
	source := string(data)

	for _, item := range []struct {
		name string
		next string
	}{
		{name: "func (ch *Channel) Publish(", next: "func (ch *Channel) PublishWithContext"},
		{name: "func (ch *Channel) Confirm(", next: "func (ch *Channel) GetNextPublishSeqNo"},
		{name: "func (ch *Channel) Ack(", next: "func (ch *Channel) Nack"},
	} {
		start := strings.Index(source, item.name)
		assert.NotEqual(t, -1, start, item.name)
		rest := source[start:]
		end := strings.Index(rest, item.next)
		assert.NotEqual(t, -1, end, item.next)
		body := rest[:end]
		assert.NotContains(t, body, "ch.mu.Lock()")
	}
}

func TestChannel_OfficialGapMethodsReturnClosedError(t *testing.T) {
	ch := &Channel{}

	_, err := ch.QueueInspect("orders")
	assert.Error(t, err)

	err = ch.Flow(true)
	assert.Error(t, err)

	confirmation, err := ch.PublishWithDeferredConfirmWithContext(
		context.Background(),
		"orders.exchange",
		"orders.created",
		false,
		false,
		amqp.Publishing{Body: []byte("test")},
	)
	assert.Error(t, err)
	assert.Nil(t, confirmation)
}
