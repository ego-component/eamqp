package eamqp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testLogger struct{}

func (testLogger) Debug(msg string, keyvals ...any) {}
func (testLogger) Info(msg string, keyvals ...any)  {}
func (testLogger) Warn(msg string, keyvals ...any)  {}
func (testLogger) Error(msg string, keyvals ...any) {}

type testMetrics struct {
	NoOpMetrics
}

func TestWithLoggerAndMetricsStoreOptions(t *testing.T) {
	var opts Options
	logger := &testLogger{}
	metrics := &testMetrics{}

	WithLogger(logger)(&opts)
	WithMetrics(metrics)(&opts)

	assert.Same(t, logger, opts.Logger)
	assert.Same(t, metrics, opts.Metrics)
}
