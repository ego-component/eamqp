package eamqp

import (
	"errors"
	"net"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
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

	t.Run("override URI vhost with root vhost", func(t *testing.T) {
		c := Config{Addr: "amqp://localhost:5672/source", Vhost: "/"}
		uris, err := c.parseURIs()
		require.NoError(t, err)
		require.Len(t, uris, 1)
		assert.Equal(t, "/", uris[0].Vhost)
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

	t.Run("invalid URI redacts password", func(t *testing.T) {
		c := Config{Addr: "amqp://guest:secret@[::1"}
		_, err := c.parseURIs()
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "secret")
		assert.Contains(t, err.Error(), "guest:xxxxx@")
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

	assert.Empty(t, cfg.Vhost)
	assert.Equal(t, 10*time.Second, cfg.Heartbeat)
	assert.Equal(t, 1, cfg.PoolSize)
	assert.Equal(t, 1, cfg.ChannelPoolSize)
	assert.Equal(t, 5*time.Second, cfg.ReconnectInterval)
	assert.False(t, cfg.EnableAccessInterceptor)
	assert.True(t, cfg.EnableMetricInterceptor)
	assert.True(t, cfg.EnableTraceInterceptor)
}

func TestConfig_ReconnectPolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReconnectInterval = 3 * time.Second
	cfg.ReconnectMaxAttempts = 5

	policy := cfg.ReconnectPolicy()

	assert.True(t, policy.Enabled)
	assert.Equal(t, 3*time.Second, policy.Initial)
	assert.Equal(t, 5, policy.MaxAttempts)
}

func TestDefaultConfig_ReturnsConfigValue(t *testing.T) {
	var _ Config = DefaultConfig()
}

func TestConfig_BuildAmqpConfigAppliesOptions(t *testing.T) {
	uri, err := amqp.ParseURI("amqp://guest:guest@localhost:5672/base")
	require.NoError(t, err)

	dialErr := errors.New("custom dial called")
	customDial := func(network, addr string) (net.Conn, error) {
		return nil, dialErr
	}
	customAuth := &amqp.ExternalAuth{}

	cfg := Config{
		ClientName: "config-client",
		Heartbeat:  10 * time.Second,
		Locale:     "en_US",
	}
	got, err := cfg.buildAmqpConfig(uri, &Options{
		Dial:           customDial,
		Auth:           []amqp.Authentication{customAuth},
		ConnectionName: "option-client",
	})
	require.NoError(t, err)

	_, err = got.Dial("tcp", "localhost:5672")
	assert.ErrorIs(t, err, dialErr)
	require.Len(t, got.SASL, 1)
	assert.Same(t, customAuth, got.SASL[0])
	assert.Equal(t, "option-client", got.Properties["connection_name"])
}

func TestConfig_BuildAmqpConfigKeepsConfigConnectionName(t *testing.T) {
	uri, err := amqp.ParseURI("amqp://guest:guest@localhost:5672/base")
	require.NoError(t, err)

	cfg := Config{
		ClientName: "config-client",
		Heartbeat:  10 * time.Second,
		Locale:     "en_US",
	}
	got, err := cfg.buildAmqpConfig(uri, nil)
	require.NoError(t, err)

	assert.Equal(t, "config-client", got.Properties["connection_name"])
	assert.NotEmpty(t, got.Properties["product"])
}

func TestRedactAMQPAddress(t *testing.T) {
	addr := "amqp://guest:secret@localhost:5672/, amqps://admin:topsecret@example.com:5671/vhost"

	got := redactAMQPAddress(addr)

	assert.NotContains(t, got, "secret")
	assert.NotContains(t, got, "topsecret")
	assert.Contains(t, got, "guest:xxxxx@localhost:5672")
	assert.Contains(t, got, "admin:xxxxx@example.com:5671")
}
