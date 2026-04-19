package eamqp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		c := Config{Addr: "amqp://guest:guest@localhost:5672/"}
		err := c.Validate()
		require.NoError(t, err)
	})

	t.Run("missing Addr", func(t *testing.T) {
		c := Config{}
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Addr is required")
	})

	t.Run("default reconnect interval", func(t *testing.T) {
		c := Config{Addr: "amqp://localhost", ReconnectInterval: 0}
		err := c.Validate()
		require.NoError(t, err)
		assert.Equal(t, 5*time.Second, c.ReconnectInterval)
	})
}

func TestConfig_ParseURIs(t *testing.T) {
	t.Run("single URI", func(t *testing.T) {
		c := Config{Addr: "amqp://guest:guest@localhost:5672/"}
		uris, err := c.parseURIs()
		require.NoError(t, err)
		require.Len(t, uris, 1)
		assert.Equal(t, "localhost", uris[0].Host)
		assert.Equal(t, "guest", uris[0].Username)
	})

	t.Run("multiple URIs", func(t *testing.T) {
		c := Config{Addr: "amqp://localhost:5672, amqp://localhost:5673"}
		uris, err := c.parseURIs()
		require.NoError(t, err)
		require.Len(t, uris, 2)
	})

	t.Run("override vhost", func(t *testing.T) {
		c := Config{Addr: "amqp://localhost:5672/", Vhost: "/test"}
		uris, err := c.parseURIs()
		require.NoError(t, err)
		require.Len(t, uris, 1)
		assert.Equal(t, "/test", uris[0].Vhost)
	})

	t.Run("override credentials", func(t *testing.T) {
		c := Config{Addr: "amqp://localhost:5672/", Username: "admin", Password: "secret"}
		uris, err := c.parseURIs()
		require.NoError(t, err)
		require.Len(t, uris, 1)
		assert.Equal(t, "admin", uris[0].Username)
		assert.Equal(t, "secret", uris[0].Password)
	})

	t.Run("invalid URI", func(t *testing.T) {
		c := Config{Addr: "invalid-uri"}
		_, err := c.parseURIs()
		assert.Error(t, err)
	})
}

func TestQueueArgs(t *testing.T) {
	t.Run("chain methods", func(t *testing.T) {
		args := NewQueueArgs().
			WithQueueType(QueueTypeQuorum).
			WithMaxLength(1000).
			WithOverflow(QueueOverflowDropHead).
			WithMessageTTL(5 * time.Second).
			WithSingleActiveConsumer()

		assert.Equal(t, QueueTypeQuorum, args[QueueTypeArg])
		assert.Equal(t, 1000, args[QueueMaxLenArg])
		assert.Equal(t, QueueOverflowDropHead, args[QueueOverflowArg])
		assert.Equal(t, int64(5000), args[QueueMessageTTLArg])
		assert.Equal(t, true, args[SingleActiveConsumerArg])
	})

	t.Run("with dead letter", func(t *testing.T) {
		args := NewQueueArgs().
			WithDeadLetterExchange("dlx").
			WithDeadLetterRoutingKey("dlq")

		assert.Equal(t, "dlx", args["x-dead-letter-exchange"])
		assert.Equal(t, "dlq", args["x-dead-letter-routing-key"])
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "/", cfg.Vhost)
	assert.Equal(t, 10*time.Second, cfg.Heartbeat)
	assert.Equal(t, 1, cfg.PoolSize)
	assert.Equal(t, 1, cfg.ChannelPoolSize)
	assert.True(t, cfg.Reconnect)
	assert.Equal(t, 5*time.Second, cfg.ReconnectInterval)
	assert.True(t, cfg.EnableLogger)
	assert.True(t, cfg.EnableMetrics)
}
