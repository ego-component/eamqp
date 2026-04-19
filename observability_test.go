package eamqp

import (
	"testing"
	"time"

	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewEgoLoggerWritesStructuredFields(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	component := elog.DefaultContainer().Build(
		elog.WithZapCore(core),
		elog.WithEnableAsync(false),
	)

	logger := NewEgoLogger(component)
	logger.Info("publish", "exchange", "orders", "size", 12)

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, "publish", entry.Message)
	assert.Equal(t, "orders", entry.ContextMap()["exchange"])
	assert.Equal(t, int64(12), entry.ContextMap()["size"])
}

func TestNewEgoMetricsImplementsMetricsCollector(t *testing.T) {
	metrics := NewEgoMetrics("amqp.default", "amqp://localhost:5672/")
	var _ MetricsCollector = metrics

	metrics.RecordConnection(true)
	metrics.RecordConnectionError()
	metrics.RecordChannelAcquired()
	metrics.RecordChannelReturned()
	metrics.RecordMessagePublished(128)
	metrics.RecordMessageConfirmed()
	metrics.RecordMessageNacked()
	metrics.RecordMessageConsumed(256)
	metrics.RecordPublishLatency(10 * time.Millisecond)
	metrics.RecordConsumeLatency(20 * time.Millisecond)
}

func TestNewEgoMetricsRedactsPeerPassword(t *testing.T) {
	metrics := NewEgoMetrics("orders", "amqp://guest:secret@localhost:5672/")

	assert.Equal(t, "orders", metrics.name)
	assert.NotContains(t, metrics.peer, "secret")
	assert.Contains(t, metrics.peer, "guest:xxxxx@localhost:5672")
}

func TestContainerBuildClientOptionsInjectsEgoObservability(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Addr = "amqp://guest:guest@localhost:5672/"
	cfg.EnableAccessInterceptor = true
	container := &Container{
		config: &cfg,
		name:   "amqp.default",
		logger: elog.EgoLogger,
	}

	var opts Options
	for _, opt := range container.buildClientOptions() {
		opt(&opts)
	}

	assert.IsType(t, &egoLogger{}, opts.Logger)
	assert.IsType(t, &EgoMetricsCollector{}, opts.Metrics)
}
