# eamqp - RabbitMQ Component for Ego Framework

`eamqp` is a production-ready RabbitMQ client component for the [Ego framework](https://github.com/ego-component/ego). It provides full integration with the official `amqp091-go` library while adhering to Ego's conventions for dependency injection, configuration management, logging, and metrics.

## Features

- Full AMQP 0-9-1 protocol support via `amqp091-go`
- Connection pooling and channel pooling for high performance
- Automatic reconnection with exponential backoff
- Publisher confirms for reliable message delivery
- TLS/SSL support (programmatic and file-based certificates)
- Multiple URIs for basic load balancing
- Structured logging with Ego integration
- Metrics collection for observability
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

	// Reconnection.
	Reconnect:            true,
	ReconnectInterval:    5 * time.Second,
	ReconnectMaxAttempts: 0,  // 0 = infinite

	// Observability.
	EnableLogger:  true,
	EnableMetrics: true,
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

## Publisher Confirms

```go
// Enable confirms for reliable publishing.
if err := ch.Confirm(false); err != nil {
	log.Fatal(err)
}

confirms := ch.NotifyPublish()

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

## Examples

See the `examples/` directory for complete examples:

- `examples/pubsub` - Publish/subscribe pattern
- `examples/producer` - Publisher confirms
- `examples/consumer` - Message consumption
- `examples/rpc` - RPC-style communication

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
