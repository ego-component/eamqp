package eamqp

import (
	"fmt"
	"time"

	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/core/emetric"
)

const metricTypeAMQP = "amqp"

// NewEgoLogger adapts an Ego logger component to the eamqp Logger interface.
func NewEgoLogger(component *elog.Component) Logger {
	if component == nil {
		return &NopLogger{}
	}
	return &egoLogger{component: component}
}

type egoLogger struct {
	component *elog.Component
}

func (l *egoLogger) Debug(msg string, keyvals ...any) {
	l.component.Debug(msg, keyvalsToFields(keyvals...)...)
}

func (l *egoLogger) Info(msg string, keyvals ...any) {
	l.component.Info(msg, keyvalsToFields(keyvals...)...)
}

func (l *egoLogger) Warn(msg string, keyvals ...any) {
	l.component.Warn(msg, keyvalsToFields(keyvals...)...)
}

func (l *egoLogger) Error(msg string, keyvals ...any) {
	l.component.Error(msg, keyvalsToFields(keyvals...)...)
}

func keyvalsToFields(keyvals ...any) []elog.Field {
	fields := make([]elog.Field, 0, (len(keyvals)+1)/2)
	for i := 0; i < len(keyvals); i += 2 {
		key := fmt.Sprintf("arg%d", i)
		if text, ok := keyvals[i].(string); ok && text != "" {
			key = text
		}

		if i+1 >= len(keyvals) {
			fields = append(fields, elog.Any(key, nil))
			continue
		}
		fields = append(fields, elog.Any(key, keyvals[i+1]))
	}
	return fields
}

// EgoMetricsCollector records AMQP client metrics using Ego's shared metrics.
type EgoMetricsCollector struct {
	name string
	peer string
}

// NewEgoMetrics creates a MetricsCollector backed by Ego emetric.
func NewEgoMetrics(name, peer string) *EgoMetricsCollector {
	if name == "" {
		name = ComponentName
	}
	return &EgoMetricsCollector{name: name, peer: redactAMQPAddress(peer)}
}

func (m *EgoMetricsCollector) RecordConnection(active bool) {
	value := 0.0
	if active {
		value = 1.0
	}
	emetric.ClientStatsGauge.Set(value, metricTypeAMQP, m.name, "connection_active")
}

func (m *EgoMetricsCollector) RecordConnectionError() {
	m.inc("connection", "Error")
}

func (m *EgoMetricsCollector) RecordChannelAcquired() {
	m.inc("channel_acquire", "OK")
}

func (m *EgoMetricsCollector) RecordChannelReturned() {
	m.inc("channel_return", "OK")
}

func (m *EgoMetricsCollector) RecordMessagePublished(size int) {
	m.inc("publish", "OK")
	emetric.ClientStatsGauge.Set(float64(size), metricTypeAMQP, m.name, "last_publish_size")
}

func (m *EgoMetricsCollector) RecordMessageConfirmed() {
	m.inc("confirm", "OK")
}

func (m *EgoMetricsCollector) RecordMessageNacked() {
	m.inc("confirm", "Nack")
}

func (m *EgoMetricsCollector) RecordMessageConsumed(size int) {
	m.inc("consume", "OK")
	emetric.ClientStatsGauge.Set(float64(size), metricTypeAMQP, m.name, "last_consume_size")
}

func (m *EgoMetricsCollector) RecordPublishLatency(duration time.Duration) {
	m.observe("publish", duration)
}

func (m *EgoMetricsCollector) RecordConsumeLatency(duration time.Duration) {
	m.observe("consume", duration)
}

func (m *EgoMetricsCollector) inc(method, code string) {
	emetric.ClientHandleCounter.Inc(metricTypeAMQP, m.name, method, m.peer, code)
}

func (m *EgoMetricsCollector) observe(method string, duration time.Duration) {
	emetric.ClientHandleHistogram.Observe(duration.Seconds(), metricTypeAMQP, m.name, method, m.peer)
}
