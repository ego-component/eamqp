package eamqp

import (
	"context"
	"fmt"

	"github.com/gotomicro/ego/core/etrace"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// InjectTraceHeaders injects the current trace context into AMQP headers.
func InjectTraceHeaders(ctx context.Context, headers amqp.Table) amqp.Table {
	if headers == nil {
		headers = amqp.Table{}
	}
	if !etrace.IsGlobalTracerRegistered() {
		return headers
	}

	carrier := amqpTableCarrier(headers)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return headers
}

// ExtractTraceContext extracts trace context from AMQP headers.
func ExtractTraceContext(ctx context.Context, headers amqp.Table) context.Context {
	if headers == nil || !etrace.IsGlobalTracerRegistered() {
		return ctx
	}
	carrier := amqpTableCarrier(headers)
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

func startPublishTrace(ctx context.Context, headers amqp.Table, enabled bool) (context.Context, amqp.Table, func()) {
	if headers == nil {
		headers = amqp.Table{}
	}
	if !enabled || !etrace.IsGlobalTracerRegistered() {
		return ctx, headers, func() {}
	}

	carrier := amqpTableCarrier(headers)
	ctx, span := etrace.NewTracer(trace.SpanKindProducer).Start(ctx, "amqp.publish", carrier)
	return ctx, headers, func() { span.End() }
}

type amqpTableCarrier amqp.Table

func (c amqpTableCarrier) Get(key string) string {
	value, ok := c[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func (c amqpTableCarrier) Set(key string, value string) {
	c[key] = value
}

func (c amqpTableCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}
	return keys
}

var _ propagation.TextMapCarrier = amqpTableCarrier{}
