package eamqp

import (
	"testing"

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
