package eamqp

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsReturnsRegisteredClientState(t *testing.T) {
	cfg := DefaultConfig()
	client := &Client{
		config:  &cfg,
		metrics: &NoOpMetrics{},
	}
	instances.Store("amqp.stats", client)
	defer instances.Delete("amqp.stats")

	got := stats()

	entry, ok := got["amqp.stats"]
	require.True(t, ok)
	assert.Equal(t, HealthStatusDown, entry.Health.Status)
	assert.Equal(t, 0, entry.Pool.ConnectionsActive)
}

func TestClientStatsReportsSingleConnection(t *testing.T) {
	cfg := DefaultConfig()
	client := &Client{config: &cfg}

	got := client.Stats()

	assert.Equal(t, 0, got.ConnectionsActive)
	assert.Equal(t, 0, got.ConnectionsTotal)
}

func TestConnectionPoolStatsAggregatesChannelPools(t *testing.T) {
	pool := newConnectionPool(&Config{PoolSize: 1}, nil, nil, nil)
	channelPool := &ChannelPool{}
	atomic.StoreInt64(&channelPool.active, 2)
	atomic.StoreInt64(&channelPool.acquired, 5)
	atomic.StoreInt64(&channelPool.returned, 3)
	pool.channelPools = []*ChannelPool{channelPool}

	got := pool.Stats()

	assert.Equal(t, 2, got.ChannelsActive)
	assert.Equal(t, int64(5), got.ChannelsAcquired)
	assert.Equal(t, int64(3), got.ChannelsReturned)
}
