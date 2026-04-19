package eamqp

import (
	"context"
	"testing"

	"github.com/gotomicro/ego/core/etrace"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceHeadersRoundTrip(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	etrace.SetGlobalTracer(tp)

	ctx, span := etrace.NewTracer(trace.SpanKindProducer).Start(context.Background(), "amqp.publish", nil)
	defer span.End()

	headers := InjectTraceHeaders(ctx, nil)

	traceParent, ok := headers["traceparent"].(string)
	require.True(t, ok)
	require.NotEmpty(t, traceParent)

	gotCtx := ExtractTraceContext(context.Background(), headers)
	assert.Equal(t, span.SpanContext().TraceID().String(), etrace.ExtractTraceID(gotCtx))
}

func TestInjectTraceHeadersPreservesExistingHeaders(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	etrace.SetGlobalTracer(tp)

	ctx, span := etrace.NewTracer(trace.SpanKindProducer).Start(context.Background(), "amqp.publish", nil)
	defer span.End()

	headers := InjectTraceHeaders(ctx, amqp.Table{"x-request-id": "req-1"})

	assert.Equal(t, "req-1", headers["x-request-id"])
	assert.NotEmpty(t, headers["traceparent"])
}

func TestStartPublishTraceHonorsDisabledSwitch(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	etrace.SetGlobalTracer(tp)

	ctx, span := etrace.NewTracer(trace.SpanKindProducer).Start(context.Background(), "amqp.publish", nil)
	defer span.End()

	_, headers, end := startPublishTrace(ctx, nil, false)
	defer end()

	assert.NotContains(t, headers, "traceparent")
}
