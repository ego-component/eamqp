package eamqp

import (
	"time"
)

// MetricsCollector collects AMQP metrics.
type MetricsCollector interface {
	RecordConnection(active bool)
	RecordConnectionError()
	RecordChannelAcquired()
	RecordChannelReturned()
	RecordMessagePublished(size int)
	RecordMessageConfirmed()
	RecordMessageNacked()
	RecordMessageConsumed(size int)
	RecordPublishLatency(duration time.Duration)
	RecordConsumeLatency(duration time.Duration)
}

// NoOpMetrics is a no-op metrics collector.
type NoOpMetrics struct{}

func (m *NoOpMetrics) RecordConnection(active bool)                {}
func (m *NoOpMetrics) RecordConnectionError()                      {}
func (m *NoOpMetrics) RecordChannelAcquired()                      {}
func (m *NoOpMetrics) RecordChannelReturned()                      {}
func (m *NoOpMetrics) RecordMessagePublished(size int)             {}
func (m *NoOpMetrics) RecordMessageConfirmed()                     {}
func (m *NoOpMetrics) RecordMessageNacked()                        {}
func (m *NoOpMetrics) RecordMessageConsumed(size int)              {}
func (m *NoOpMetrics) RecordPublishLatency(duration time.Duration) {}
func (m *NoOpMetrics) RecordConsumeLatency(duration time.Duration) {}
