# eamqp - RabbitMQ Component for Ego Framework

`eamqp` is a RabbitMQ client component for the [Ego framework](https://github.com/ego-component/ego). It keeps the official `amqp091-go` connection/channel model visible, while adding Ego-style configuration loading and injectable logging/metrics hooks.

See [CAPABILITY_MATRIX.md](./CAPABILITY_MATRIX.md) for the current support matrix and the boundary between the base AMQP wrapper, Ego observability, and future consumer lifecycle work.

## Features

- AMQP 0-9-1 support via `amqp091-go` thin wrappers and raw accessors
- Ego config-file loading with `eamqp.Load(...).Build()`
- Connection pooling and channel pooling
- Reconnect policy primitives and manual reconnect
- Publisher confirms for reliable message delivery
- TLS/SSL support (programmatic and file-based certificates)
- Multiple URIs for basic load balancing
- Ego `elog`/`emetric` adapters when loaded through `Load(...).Build()`
- AMQP header trace propagation helpers
- Lightweight health status and ping checks
- High-level producer and consumer helpers

## Installation

```bash
go get github.com/ego-component/eamqp
```

## Quick Start

```go
package main

import (
	"log"

	"github.com/ego-component/eamqp"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	// Create client.
	client, err := eamqp.New(eamqp.Config{
		Addr: "amqp://guest:guest@localhost:5672/",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create channel.
	ch, err := client.NewChannel()
	if err != nil {
		log.Fatal(err)
	}
	defer ch.Close()

	// Declare exchange and queue.
	if err := ch.ExchangeDeclare("my-exchange", "topic", true, false, false, false, nil); err != nil {
		log.Fatal(err)
	}

	q, err := ch.QueueDeclare("my-queue", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	if err := ch.QueueBind(q.Name, "order.*", "my-exchange", false, nil); err != nil {
		log.Fatal(err)
	}

	// Publish a message.
	err = ch.Publish("my-exchange", "order.created", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        []byte(`{"order_id": 123}`),
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

## Configuration

For Ego config-file usage with `eamqp.Load("amqp.default").Build()`, see [CONFIGURATION.md](./CONFIGURATION.md).

```go
config := eamqp.Config{
	// Connection URI(s). Multiple URIs separated by comma enable load balancing.
	Addr: "amqp://guest:guest@localhost:5672/",

	// TLS configuration.
	TLSConfig: &tls.Config{ServerName: "localhost"},

	// Or use file-based TLS.
	TLSCertFile: "/path/to/cert.pem",
	TLSKeyFile:  "/path/to/key.pem",
	TLSCACert:   "/path/to/ca.pem",

	// Tuning.
	Heartbeat: 10 * time.Second,
	ChannelMax: 0,   // 0 = server default
	FrameSize:  0,   // 0 = server default

	// Connection pool.
	PoolSize:    1,   // Number of connections
	PoolMaxIdle: 2,
	PoolMaxLife: time.Hour,

	// Channel pool (per connection).
	ChannelPoolSize:    1,
	ChannelPoolMaxIdle: 2,
	ChannelPoolMaxLife: 5 * time.Minute,

	// Explicit reconnect helper policy. No background topology/consumer
	// supervisor is started by this component.
	ReconnectInterval:    5 * time.Second,
	ReconnectMaxAttempts: 0,

	// Observability.
	EnableAccessInterceptor: false,
	EnableMetricInterceptor: true,
	EnableTraceInterceptor:  true,
}
```

## Connection Pooling

```go
// Enable connection pooling for high availability.
client, err := eamqp.New(eamqp.Config{
	// Multiple URIs enable basic load balancing across connections.
	Addr: "amqp://localhost:5672,amqp://localhost:5673",
	PoolSize: 2,  // Creates 2 connections
})
```

## Channel Pooling

```go
// Enable channel pooling for efficient channel reuse.
client, err := eamqp.New(eamqp.Config{
	Addr: "amqp://localhost:5672",
	ChannelPoolSize: 10,  // Pool of 10 channels per connection
})
```

`Channel` wraps the official `amqp091-go` channel. Wrapper methods only protect
`eamqp` state briefly; AMQP operation serialization is delegated to
`amqp091-go`. If you use `RawChannel()`, keep the official rule: a raw channel
is not safe to share across goroutines without your own synchronization. Pooled
channels that enter stateful modes such as confirm, QoS, transactions,
consumption, notify listeners, or raw access are closed on `Close()` instead of
being returned to the idle pool.

## Publisher Confirms

```go
// Enable confirms for reliable publishing.
if err := ch.Confirm(false); err != nil {
	log.Fatal(err)
}

confirms := ch.NotifyPublish()
// Always consume NotifyPublish/NotifyClose style channels until they close.

// Publish with confirmation.
err = ch.Publish("exchange", "key", false, false, amqp.Publishing{
	Body: []byte("message"),
})
if err != nil {
	log.Fatal(err)
}

// Wait for confirmation.
confirm := <-confirms
if !confirm.Ack {
	log.Printf("Message rejected")
}
```

## Producer Helper

```go
// Create producer with confirms.
producer, err := eamqp.NewProducer(ch, eamqp.WithConfirm(5*time.Second))
if err != nil {
	log.Fatal(err)
}

// Publish with automatic confirm waiting.
err = producer.Publish("exchange", "key", amqp.Publishing{
	Body: []byte("message"),
})
```

## Consumer Helper

```go
// Create consumer.
consumer := eamqp.NewConsumer(ch, "my-queue",
	eamqp.WithConsumerAutoAck(),  // Enable auto-ack
)

// Start consuming.
deliveries, err := consumer.Consume("consumer-1")
if err != nil {
	log.Fatal(err)
}

for delivery := range deliveries {
	fmt.Println(string(delivery.Body))
	delivery.Ack(false)
}
```

## Exchange Types

```go
// Direct exchange - routing by exact key match.
ch.ExchangeDeclare("direct-exchange", eamqp.ExchangeDirect, true, false, false, false, nil)

// Fanout exchange - routing to all bound queues.
ch.ExchangeDeclare("fanout-exchange", eamqp.ExchangeFanout, true, false, false, false, nil)

// Topic exchange - routing by pattern matching.
ch.ExchangeDeclare("topic-exchange", eamqp.ExchangeTopic, true, false, false, false, nil)

// Headers exchange - routing by header values.
ch.ExchangeDeclare("headers-exchange", eamqp.ExchangeHeaders, true, false, false, false, nil)
```

## Queue Types

```go
// Classic queue.
q, _ := ch.QueueDeclare("my-classic-queue", true, false, false, false, nil)

// Quorum queue (recommended for most use cases).
q, _ := ch.QueueDeclare("my-quorum-queue", true, false, false, false, amqp.Table{
	eamqp.QueueTypeArg: eamqp.QueueTypeQuorum,
})

// Stream queue (for replay-style scenarios).
q, _ := ch.QueueDeclare("my-stream-queue", true, false, false, false, amqp.Table{
	eamqp.QueueTypeArg: eamqp.QueueTypeStream,
})
```

## Error Handling

```go
// Errors are wrapped with context.
ch.Publish("exchange", "key", false, false, amqp.Publishing{Body: []byte("msg")})

// Check if error is retryable.
if err != nil {
	if e, ok := err.(*eamqp.Error); ok && e.IsRetryable() {
		// Can retry
	}
}
```

## Logging and Metrics

`Load(...).Build()` injects Ego `elog`, `emetric`, and trace adapters according
to `enableAccessInterceptor`, `enableMetricInterceptor`, and
`enableTraceInterceptor`. Direct `New(...)` usage can inject custom hooks
explicitly:

```go
// Implement the Logger interface.
type MyLogger struct{}

func (l *MyLogger) Debug(msg string, keyvals ...any) { /* ... */ }
func (l *MyLogger) Info(msg string, keyvals ...any)  { /* ... */ }
func (l *MyLogger) Warn(msg string, keyvals ...any)  { /* ... */ }
func (l *MyLogger) Error(msg string, keyvals ...any) { /* ... */ }

// Implement the MetricsCollector interface.
type MyMetrics struct{}

func (m *MyMetrics) RecordPublishLatency(d time.Duration) { /* ... */ }
func (m *MyMetrics) RecordMessageConsumed(size int)       { /* ... */ }
// ... implement all methods

client, err := eamqp.New(cfg,
	eamqp.WithLogger(&MyLogger{}),
	eamqp.WithMetrics(&MyMetrics{}),
)
```

## Trace Headers

`PublishWithContext` injects the active Ego/OpenTelemetry trace context into
AMQP headers when a global Ego tracer is registered. With amqp091-go v1.9.0,
the context should not be treated as a guaranteed publish timeout or
cancellation mechanism. Consumer code can extract the context explicitly from a
delivery:

```go
ctx = eamqp.ExtractTraceContext(ctx, delivery.Headers)
```

For custom publishing paths, use:

```go
msg.Headers = eamqp.InjectTraceHeaders(ctx, msg.Headers)
```

## Health Checks

`HealthStatus()` is a lightweight in-memory check based on the current
connection or connection pool state. `Ping(ctx)` opens and closes an AMQP
channel, so use it when readiness must prove that RabbitMQ is accepting channel
operations:

```go
if err := client.Ping(ctx); err != nil {
	return err
}

health := client.HealthStatus()
if !client.Health() {
	log.Printf("amqp unhealthy: %s", health.Reason)
}
```

RabbitMQ blocked/unblocked events are still exposed separately through
`NotifyBlocked()`. `Reconnect()` is explicit; this component does not silently
rebuild channels, consumers, or topology in the background.

## Raw AMQP Access

The wrapper exposes raw accessors so uncommon `amqp091-go` features are not
blocked by `eamqp`:

```go
conn := client.RawConnection()
rawCh := ch.RawChannel()
```

## Examples

Examples use Ego-style config loading through `eamqp.Load(...).Build()`. Start
RabbitMQ locally, then run examples with the provided config file:

```bash
go run ./examples/producer --config=examples/config/local.toml
go run ./examples/consumer --config=examples/config/local.toml
go run ./examples/pubsub --config=examples/config/local.toml
go run ./examples/connection-pool --config=examples/config/local.toml
go run ./examples/producer-confirm --config=examples/config/local.toml
go run ./examples/batch-producer --config=examples/config/local.toml
go run ./examples/transaction --config=examples/config/local.toml
go run ./examples/workqueue-publisher --config=examples/config/local.toml
go run ./examples/workqueue-worker --config=examples/config/local.toml
go run ./examples/rpc --config=examples/config/local.toml
go run ./examples/qos --config=examples/config/local.toml publish
go run ./examples/qos --config=examples/config/local.toml inspect
go run ./examples/dead-letter --config=examples/config/local.toml publisher
go run ./examples/dead-letter --config=examples/config/local.toml inspect-dlq
go run ./examples/retry-consumer-sender --config=examples/config/local.toml
go run ./examples/pubsub-fanout --config=examples/config/local.toml publish
go run ./examples/reconnect --config=examples/config/local.toml producer
```

The example helper also checks `EAMQP_EXAMPLE_CONFIG` and, when running from the
repository root, falls back to `examples/config/local.toml`. Role-based examples
strip `--config` before parsing their own arguments, so either
`--config=examples/config/local.toml publish` or
`publish --config=examples/config/local.toml` works.

- `examples/pubsub` - Publish/subscribe pattern
- `examples/producer` / `examples/consumer` - Paired direct exchange example
- `examples/producer-confirm` - High-level producer confirms
- `examples/batch-producer` - Batch helper with publisher confirms
- `examples/connection-pool` - Configured connection pool
- `examples/transaction` - AMQP transactions
- `examples/workqueue-publisher` / `examples/workqueue-worker` - Work queue pattern
- `examples/rpc` - RPC-style communication
- `examples/qos` - Consumer prefetch and queue inspection
- `examples/dead-letter` - DLX and DLQ handling
- `examples/retry-consumer-listener` / `examples/retry-consumer-sender` - Retry pattern
- `examples/pubsub-fanout` - Fanout broadcast publish/subscribe
- `examples/reconnect` - Close notifications and explicit reconnect boundary

## Roadmap

### v0.1.0

Initial production candidate:

- Ego-style config loading with `Load(...).Build()` and `BuildE()`
- Configurable connection, channel pool, TLS, logging, metrics, and trace options
- Thin AMQP wrappers with raw access to `amqp091-go`
- Publisher confirms, QoS, transactions, dead-letter, RPC, pub/sub, and work queue examples
- Health checks, ping, pool stats, and `/debug/amqp/stats`
- Explicit reconnect boundary without automatic topology or consumer recovery

### v0.2.0

Consumer lifecycle hardening:

- Consumer supervisor helper for common worker patterns
- Graceful shutdown, handler panic recovery, and structured ack/nack handling
- Consumer metrics for deliveries, ack/nack/reject, handler errors, and processing latency
- Reconnect template that rebuilds connection, channel, topology, QoS, and consumers in application code
- Separate producer and consumer connection examples for RabbitMQ TCP pushback isolation
- `CONSUMER_GUIDE.md` with production consumption patterns

### v0.3.0

Advanced operational helpers:

- Optional topology declaration helper for applications that want a repeatable bootstrap step
- Batch publisher confirm helper with explicit nack and retry hooks
- RabbitMQ blocked/unblocked event handling examples
- Refined metric labels and dashboard guidance
- Compatibility review for newer `amqp091-go` releases

## Architecture

```
Client
  ├── Connection (single mode)
  │     └── Channel
  │           ├── ExchangeDeclare/Bind
  │           ├── QueueDeclare/Bind
  │           ├── Publish
  │           └── Consume
  │
  └── ConnectionPool (multi-connection mode)
        ├── Connection 1
        │     └── ChannelPool
        │           ├── Channel
        │           └── Channel
        ├── Connection 2
        │     └── ChannelPool
        └── ...
```

## License

BSD 3-Clause License. See LICENSE file.
