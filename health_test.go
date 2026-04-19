package eamqp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestClientHealthStatus(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		var client *Client

		health := client.HealthStatus()

		assert.Equal(t, HealthStatusDown, health.Status)
		assert.Equal(t, "client is nil", health.Reason)
		assert.False(t, client.Health())
	})

	t.Run("closed client", func(t *testing.T) {
		client := &Client{closed: true}

		health := client.HealthStatus()

		assert.Equal(t, HealthStatusClosed, health.Status)
		assert.Equal(t, "client is closed", health.Reason)
		assert.False(t, client.Health())
	})

	t.Run("no connection", func(t *testing.T) {
		client := &Client{}

		health := client.HealthStatus()

		assert.Equal(t, HealthStatusDown, health.Status)
		assert.Equal(t, "no connection available", health.Reason)
		assert.False(t, client.Health())
	})

	t.Run("closed pool", func(t *testing.T) {
		pool := newConnectionPool(&Config{PoolSize: 2}, nil, nil, []amqp.URI{
			{Scheme: "amqp", Host: "localhost:5672"},
			{Scheme: "amqp", Host: "localhost:5673"},
		})
		require.NoError(t, pool.Close())
		client := &Client{pool: pool}

		health := client.HealthStatus()

		assert.Equal(t, HealthStatusClosed, health.Status)
		assert.Equal(t, "connection pool is closed", health.Reason)
		assert.Equal(t, 0, health.ConnectionsActive)
		assert.Equal(t, 2, health.ConnectionsTotal)
		assert.False(t, client.Health())
	})

	t.Run("pool without active connections", func(t *testing.T) {
		pool := newConnectionPool(&Config{PoolSize: 2}, nil, nil, []amqp.URI{
			{Scheme: "amqp", Host: "localhost:5672"},
			{Scheme: "amqp", Host: "localhost:5673"},
		})
		client := &Client{pool: pool}

		health := client.HealthStatus()

		assert.Equal(t, HealthStatusDown, health.Status)
		assert.Equal(t, "no active connections", health.Reason)
		assert.Equal(t, 0, health.ConnectionsActive)
		assert.Equal(t, 2, health.ConnectionsTotal)
		assert.False(t, client.Health())
	})
}

func TestClientPing(t *testing.T) {
	t.Run("closed client", func(t *testing.T) {
		client := &Client{closed: true}

		err := client.Ping(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "client is closed")
	})

	t.Run("no connection", func(t *testing.T) {
		client := &Client{}

		err := client.Ping(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no connection available")
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		client := &Client{}

		err := client.Ping(ctx)

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
