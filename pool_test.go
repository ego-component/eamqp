package eamqp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestPoolStats(t *testing.T) {
	stats := PoolStats{
		ConnectionsActive: 1,
		ConnectionsTotal:  5,
		ChannelsActive:    2,
		ChannelsAcquired:  100,
		ChannelsReturned:  95,
		Reconnects:        3,
	}

	assert.Equal(t, 1, stats.ConnectionsActive)
	assert.Equal(t, 5, stats.ConnectionsTotal)
	assert.Equal(t, 2, stats.ChannelsActive)
	assert.Equal(t, int64(100), stats.ChannelsAcquired)
	assert.Equal(t, int64(95), stats.ChannelsReturned)
	assert.Equal(t, int64(3), stats.Reconnects)
}

func TestChannelPoolClose(t *testing.T) {
	// Test that closing an already-closed pool doesn't panic.
	pool := &ChannelPool{
		channels: make(chan *amqp.Channel, 2),
		closed:   true, // Already closed
	}

	err := pool.Close()
	assert.NoError(t, err)
	assert.True(t, pool.IsClosed())
}

func TestChannelPoolPermitsLimitActiveLeases(t *testing.T) {
	pool := &ChannelPool{
		permits: make(chan struct{}, 2),
	}

	assert.True(t, pool.acquirePermit())
	assert.True(t, pool.acquirePermit())
	assert.False(t, pool.acquirePermit())
	assert.Equal(t, 2, pool.Stats().ChannelsActive)

	pool.releasePermit()

	assert.Equal(t, 1, pool.Stats().ChannelsActive)
	assert.True(t, pool.acquirePermit())
	assert.Equal(t, 2, pool.Stats().ChannelsActive)
}

func TestConnectionPoolClose(t *testing.T) {
	// Test that closing an empty pool doesn't error.
	pool := newConnectionPool(&Config{PoolSize: 2}, nil, nil, nil)
	pool.closed = true

	err := pool.Close()
	assert.NoError(t, err)
	assert.True(t, pool.IsClosed())
}

func TestConnectionPoolCloseClosesChannelPools(t *testing.T) {
	pool := newConnectionPool(&Config{PoolSize: 1}, nil, nil, []amqp.URI{{Scheme: "amqp", Host: "localhost:5672"}})
	channelPool := &ChannelPool{
		channels: make(chan *amqp.Channel, 1),
	}
	pool.channelPools = []*ChannelPool{channelPool}

	require.NoError(t, pool.Close())

	assert.True(t, channelPool.IsClosed())
}

func TestConnectionPoolLen(t *testing.T) {
	uris := []amqp.URI{
		{Scheme: "amqp", Host: "localhost:5672"},
		{Scheme: "amqp", Host: "localhost:5673"},
		{Scheme: "amqp", Host: "localhost:5674"},
	}
	pool := newConnectionPool(&Config{PoolSize: 3}, nil, nil, uris)
	assert.Equal(t, 3, pool.Len())
}

func TestConnectionPoolLenZero(t *testing.T) {
	pool := newConnectionPool(&Config{PoolSize: 0}, nil, nil, nil)
	assert.Equal(t, 0, pool.Len())
}
