# Eamqp Component Design

> RabbitMQ Component for Ego Framework - Based on amqp091-go

## Overview

The `eamqp` component integrates the official [amqp091-go](https://github.com/rabbitmq/amqp091-go) RabbitMQ client library into the Ego framework, providing complete RabbitMQ connectivity with Ego's standard dependency injection, configuration, logging, and metrics integration.

## Architecture

```mermaid
flowchart TB
    subgraph "Configuration Layer"
        Config["Config struct<br/>URI, TLS, Pool, Reconnect, etc."]
        Options["Options struct<br/>Custom dialer, auth, etc."]
    end
    
    subgraph "Client Layer"
        Client["Client<br/>Connection management<br/>Connection pool<br/>Reconnection"]
        ConnPool["ConnectionPool<br/>Multiple connections<br/>Round-robin distribution"]
        ChanPool["ChannelPool<br/>Per-connection channels<br/>Pooled acquisition"]
    end
    
    subgraph "Operational Layer"
        Channel["Channel wrapper<br/>Exchange/Queue operations<br/>Full amqp API surface"]
        Producer["Producer wrapper<br/>Publish with confirms"]
        Consumer["Consumer wrapper<br/>Consume with ack/nack"]
    end
    
    subgraph "Observability"
        Logger["Logger integration<br/>Ego logger wrapper"]
        Metrics["Metrics collection<br/>Connection, channel, msg stats"]
    end
    
    Config --> Client
    Options --> Client
    Client --> ConnPool
    Client --> ChanPool
    ChanPool --> Channel
    Channel --> Producer
    Channel --> Consumer
    Client --> Logger
    Client --> Metrics
```

## Design Decisions

### 1. URI-based Configuration

Uses the official library's URI parsing (`amqp.ParseURI`) as the primary configuration method, with supplemental fields for pool, reconnect, and observability options.

### 2. Connection Pool + Channel Pool

Two-tier pooling:
- **Connection Pool**: Multiple connections for high availability and true parallel processing
- **Channel Pool**: Per-connection channels for efficient channel reuse

Default: Single connection, single channel (zero-configuration simplicity).

### 3. TLS Configuration

Supports both:
- **Programmatic**: Pass `*tls.Config` directly
- **File-based**: Specify cert/key/ca file paths in config

### 4. Direct amqp Type Exposure

Uses native `amqp.Publishing`, `amqp.Delivery`, `amqp.Queue` types throughout. Thin wrapper adds Ego integration without hiding the library's power.

### 5. Automatic Reconnection

Built-in reconnection with exponential backoff. Configurable attempts, intervals, and behavior.

## File Structure

```
eamqp/
├── config.go        # Config struct with defaults, URI parsing
├── options.go       # Options pattern (TLS files, custom dialer, auth)
├── client.go       # Client with connection/channel pool management
├── pool.go         # ConnectionPool and ChannelPool implementations
├── channel.go      # Channel wrapper (Exchange/Queue operations)
├── producer.go     # Publishing with confirms support
├── consumer.go     # Consuming with ack/nack helpers
├── reconnect.go    # Automatic reconnection with backoff
├── errors.go       # Error type with ego context
├── constants.go    # Exchange types, delivery modes, queue args
├── types.go        # Additional types (QueueArgs, ConsumerOptions)
├── examples/
│   ├── pubsub/     # Publish/Subscribe example
│   ├── rpc/        # RPC over RabbitMQ example
│   └── confluent/  # Publisher confirms example
└── README.md
```

## Configuration

### Config Struct

```go
type Config struct {
    // Connection
    Addr     string   // AMQP URI(s), comma-separated for HA
    Vhost    string   // Override virtual host
    
    // TLS - Programmatic
    TLSConfig *tls.Config
    
    // TLS - File-based (alternative to TLSConfig)
    TLSCertFile   string
    TLSKeyFile    string
    TLSCACert     string
    TLSServerName string
    
    // Auth
    Username string
    Password string
    
    // Tuning
    Heartbeat time.Duration
    ChannelMax uint16
    FrameSize  int
    Locale     string
    
    // Connection pool
    PoolSize    int
    PoolMaxIdle int
    PoolMaxLife time.Duration
    
    // Channel pool
    ChannelPoolSize     int
    ChannelPoolMaxIdle int
    ChannelPoolMaxLife  time.Duration
    
    // Reconnection
    Reconnect            bool
    ReconnectInterval    time.Duration
    ReconnectMaxAttempts int
    
    // Observability
    EnableLogger   bool
    EnableMetrics  bool
    
    // Debug
    ClientName string
}
```

### Options Struct

```go
type Options struct {
    Dial            func(network, addr string) (net.Conn, error)
    Auth            []amqp.Authentication
    ConnectionName  string
    ChannelOptions  func(*amqp.Channel) error
}
```

### Default Values

| Field | Default |
|-------|---------|
| Vhost | "/" |
| Heartbeat | 10s |
| PoolSize | 1 (single connection) |
| ChannelPoolSize | 1 (single channel) |
| Reconnect | true |
| ReconnectInterval | 5s |
| ReconnectMaxAttempts | 0 (infinite) |
| EnableLogger | true |
| EnableMetrics | true |

## Key Interfaces

### Client

```go
type Client interface {
    Close() error
    IsClosed() bool
    NewChannel() (*Channel, error)
    AcquireChannel() (*Channel, func(), error)
    NotifyClose() <-chan *Error
    Config() *Config
}
```

### Channel

The Channel wrapper provides all amqp.Channel methods with Ego integration:

- **Exchange**: Declare, Delete, Bind, Unbind
- **Queue**: Declare, Bind, Unbind, Purge, Delete
- **Publish**: Basic, WithContext, WithDeferredConfirm
- **Consume**: Basic, WithContext, Cancel, Get
- **QoS**: Qos, Flow
- **Transactions**: Tx, TxCommit, TxRollback
- **Confirms**: Confirm, GetNextPublishSeqNo
- **Notifications**: NotifyClose, NotifyFlow, NotifyReturn, NotifyCancel, NotifyPublish, NotifyConfirm

## Metrics

When `EnableMetrics=true`:

| Metric | Type | Description |
|--------|------|-------------|
| eamqp_connections_active | Gauge | Active connections |
| eamqp_connections_total | Counter | Total connections opened |
| eamqp_connections_errors_total | Counter | Connection errors |
| eamqp_channels_active | Gauge | Active channels |
| eamqp_channels_acquired_total | Counter | Channels acquired from pool |
| eamqp_channels_returned_total | Counter | Channels returned to pool |
| eamqp_messages_published_total | Counter | Messages published |
| eamqp_messages_confirmed_total | Counter | Messages confirmed |
| eamqp_messages_nacked_total | Counter | Messages nacked |
| eamqp_messages_consumed_total | Counter | Messages consumed |
| eamqp_publish_latency_ms | Histogram | Publish latency |
| eamqp_consume_latency_ms | Histogram | Consume latency |

## Error Handling

```go
type Error struct {
    *amqp.Error
    Component string
    Op       string
}

func (e *Error) Error() string
func (e *Error) IsRetryable() bool
```

## Reconnection Strategy

1. Detect connection close via `NotifyClose`
2. Apply exponential backoff (configurable initial/max/multiplier)
3. Re-establish connection(s)
4. Re-create channel pool
5. Resume operations

## Constants

### Exchange Types
- `ExchangeDirect`
- `ExchangeFanout`
- `ExchangeTopic`
- `ExchangeHeaders`

### Delivery Modes
- `Transient` (1) - Higher throughput, lost on restart
- `Persistent` (2) - Restored on restart

### Queue Arguments (RabbitMQ)
- `QueueTypeArg` / `QueueTypeClassic` / `QueueTypeQuorum` / `QueueTypeStream`
- `QueueMaxLenArg` / `QueueMaxLenBytesArg`
- `QueueOverflowArg` / `QueueOverflowDropHead` / `QueueOverflowRejectPublish`
- `QueueMessageTTLArg` / `QueueTTLArg`
- `StreamMaxAgeArg` / `StreamMaxSegmentSizeBytesArg`
- `ConsumerTimeoutArg` / `SingleActiveConsumerArg`

## Usage Examples

### Basic Usage

```go
client, _ := eamqp.New(eamqp.Config{
    Addr: "amqp://guest:guest@localhost:5672/",
})

ch, _ := client.NewChannel()
ch.ExchangeDeclare("events", "topic", true, false, false, false, nil)
q, _ := ch.QueueDeclare("my-queue", true, false, false, false, nil)
ch.QueueBind("my-queue", "orders.#", "events", false, nil)

ch.Publish("events", "orders.created", false, false, amqp.Publishing{
    ContentType:  "application/json",
    DeliveryMode: amqp.Persistent,
    Body:         body,
})

deliveries, _ := ch.Consume(q.Name, "consumer-1", false, false, false, false, nil)
for d := range deliveries {
    d.Ack(false)
}
```

### With Publisher Confirms

```go
ch, _ := client.NewChannel()
ch.Confirm(false)

confirms := ch.NotifyPublish()
dc, _ := ch.PublishWithDeferredConfirm("exchange", "key", false, false, msg)

select {
case confirm := <-confirms:
    if confirm.Ack {
        // Message acknowledged
    }
case <-time.After(5 * time.Second):
    // Timeout
}
```

### With Channel Pool

```go
client, _ := eamqp.New(eamqp.Config{
    Addr:              "amqp://guest:guest@localhost:5672/",
    ChannelPoolSize:   10,
    ChannelPoolMaxIdle: 5,
})

ch, release, _ := client.AcquireChannel()
defer release()
// Use channel...
```

### With TLS

```go
client, _ := eamqp.New(eamqp.Config{
    Addr:           "amqps://guest:guest@localhost:5671/",
    TLSCertFile:    "/path/to/client.crt",
    TLSKeyFile:     "/path/to/client.key",
    TLSCACert:      "/path/to/ca.crt",
    TLSServerName:  "rabbitmq.example.com",
})
```

## Implementation Notes

1. All amqp.Channel methods are wrapped with error handling and metrics collection
2. Channel pool acquisition returns a release function for proper cleanup
3. Connection pool uses round-robin distribution by default
4. Reconnection preserves the original config and re-dials automatically
5. Logger integration uses ego's structured logging pattern
6. All native amqp types (Publishing, Delivery, Queue, etc.) are used directly
