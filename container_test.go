package eamqp

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gotomicro/ego/core/econf"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromEgoConfig(t *testing.T) {
	econf.Reset()
	defer econf.Reset()

	const conf = `
[amqp.default]
addr = "amqp://guest:guest@localhost:5672/"
vhost = "/"
heartbeat = "15s"
poolSize = 2
channelPoolSize = 8
channelPoolMaxIdle = 4
channelPoolMaxLife = "2m"
reconnectInterval = "3s"
reconnectMaxAttempts = 5
enableAccessInterceptor = true
enableMetricInterceptor = false
enableTraceInterceptor = false
clientName = "order-service"
onFail = "error"
`
	require.NoError(t, econf.LoadFromReader(strings.NewReader(conf), toml.Unmarshal))

	container := Load("amqp.default")

	assert.Equal(t, "amqp.default", container.name)
	assert.Equal(t, "amqp://guest:guest@localhost:5672/", container.config.Addr)
	assert.Equal(t, "/", container.config.Vhost)
	assert.Equal(t, 15*time.Second, container.config.Heartbeat)
	assert.Equal(t, 2, container.config.PoolSize)
	assert.Equal(t, 8, container.config.ChannelPoolSize)
	assert.Equal(t, 4, container.config.ChannelPoolMaxIdle)
	assert.Equal(t, 2*time.Minute, container.config.ChannelPoolMaxLife)
	assert.Equal(t, 3*time.Second, container.config.ReconnectInterval)
	assert.Equal(t, 5, container.config.ReconnectMaxAttempts)
	assert.True(t, container.config.EnableAccessInterceptor)
	assert.False(t, container.config.EnableMetricInterceptor)
	assert.False(t, container.config.EnableTraceInterceptor)
	assert.Equal(t, "order-service", container.config.ClientName)
	assert.Equal(t, "error", container.onFail)
}

func TestBuildEReturnsErrorAndBuildReturnsNilWithOnFailError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Addr = "invalid-uri"
	cfg.OnFail = "error"
	container := &Container{
		config: &cfg,
		name:   "amqp.invalid",
		logger: elog.EgoLogger,
		onFail: "error",
	}

	client, err := container.BuildE()
	require.Error(t, err)
	assert.Nil(t, client)

	assert.Nil(t, container.Build())
}

func TestBuildEDoesNotPanicWithDefaultOnFail(t *testing.T) {
	cfg := DefaultConfig()
	container := &Container{
		config: &cfg,
		name:   "amqp.invalid",
		logger: elog.EgoLogger,
		onFail: "panic",
	}

	assert.NotPanics(t, func() {
		client, err := container.BuildE()
		require.Error(t, err)
		assert.Nil(t, client)
	})
}

func TestLoadBuildFromEgoConfigIntegration(t *testing.T) {
	if os.Getenv("EAMQP_INTEGRATION") != "1" {
		t.Skip("set EAMQP_INTEGRATION=1 to verify config-driven RabbitMQ connection")
	}

	econf.Reset()
	defer econf.Reset()

	const conf = `
[amqp.default]
addr = "amqp://guest:guest@127.0.0.1:5672/"
clientName = "eamqp-config-test"
onFail = "error"
`
	require.NoError(t, econf.LoadFromReader(strings.NewReader(conf), toml.Unmarshal))

	client, err := Load("amqp.default").BuildE()
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Same(t, client, LoadInstance("amqp.default"))
	require.NoError(t, client.Ping(context.Background()))
	assert.True(t, client.Health())
	assert.Equal(t, HealthStatusUp, client.HealthStatus().Status)
	assert.Equal(t, 1, client.Stats().ConnectionsActive)
	assert.Equal(t, 1, client.Stats().ConnectionsTotal)
	require.NoError(t, client.Close())
}

func TestLoadBuildUsesChannelPoolIntegration(t *testing.T) {
	if os.Getenv("EAMQP_INTEGRATION") != "1" {
		t.Skip("set EAMQP_INTEGRATION=1 to verify channel pool reuse")
	}

	econf.Reset()
	defer econf.Reset()

	const conf = `
[amqp.pooltest]
addr = "amqp://guest:guest@127.0.0.1:5672/,amqp://guest:guest@127.0.0.1:5672/"
poolSize = 1
channelPoolSize = 2
channelPoolMaxIdle = 1
clientName = "eamqp-channel-pool-test"
onFail = "error"
`
	require.NoError(t, econf.LoadFromReader(strings.NewReader(conf), toml.Unmarshal))

	client, err := Load("amqp.pooltest").BuildE()
	require.NoError(t, err)
	require.NotNil(t, client.pool)

	ch, err := client.NewChannel()
	require.NoError(t, err)
	require.NoError(t, ch.Close())

	stats := client.pool.channelPools[0].Stats()
	assert.Equal(t, int64(1), stats.ChannelsReturned)

	require.NoError(t, client.Close())
}

func TestPooledStatefulChannelIsDiscardedIntegration(t *testing.T) {
	if os.Getenv("EAMQP_INTEGRATION") != "1" {
		t.Skip("set EAMQP_INTEGRATION=1 to verify stateful channel discard")
	}

	econf.Reset()
	defer econf.Reset()

	const conf = `
[amqp.statefulpool]
addr = "amqp://guest:guest@127.0.0.1:5672/"
channelPoolSize = 1
channelPoolMaxIdle = 1
clientName = "eamqp-stateful-pool-test"
onFail = "error"
`
	require.NoError(t, econf.LoadFromReader(strings.NewReader(conf), toml.Unmarshal))

	client, err := Load("amqp.statefulpool").BuildE()
	require.NoError(t, err)
	require.NotNil(t, client.pool)

	ch, err := client.NewChannel()
	require.NoError(t, err)
	require.NoError(t, ch.Confirm(false))
	require.NoError(t, ch.Close())

	stats := client.pool.channelPools[0].Stats()
	assert.Equal(t, int64(0), stats.ChannelsReturned)
	assert.Equal(t, 0, stats.ChannelsActive)

	require.NoError(t, client.Close())
}
