# eamqp Capability Matrix

This document records the current design boundary for `eamqp`. The component
should preserve `github.com/rabbitmq/amqp091-go` semantics first, then add Ego
configuration and observability around the official client.

## Design Position

`eamqp` is a thin RabbitMQ component for Ego projects. It should not hide AMQP
0-9-1 concepts such as connection, channel, exchange, queue, binding, ack,
nack, qos, tx, confirm, and notify channels.

Configuration files should drive connection-level component setup: address,
vhost, TLS, heartbeat, pool sizing, client name, logging, metrics, and startup
failure behavior. Business topology such as exchange, queue, binding, routing
key, ack policy, retry policy, and dead-letter routing can be configured later
as optional higher-level helpers, but should remain code-first in the base API.

## Current Coverage

| Area | amqp091-go ability | Current eamqp state | Decision |
| --- | --- | --- | --- |
| Connection dialing | `DialConfig`, custom `Config.Dial` | Uses `DialConfig`; `Options.Dial` is wired into `amqp.Config` | Keep covered by tests |
| Authentication | URI auth, `Config.SASL` | Username/password and `Options.Auth` are supported | Keep URI auth when no override is set |
| Client name | `Table.SetClientConnectionName` | `ClientName` and `Options.ConnectionName` are wired through official properties | Keep covered by tests |
| TLS | `TLSClientConfig`, `DialTLS` behavior | Programmatic TLS and file-based CA roots are supported | Keep CA-only and cert/key tests |
| Raw access | `*amqp.Connection`, `*amqp.Channel` | `RawConnection` and `RawChannel` are available | Use for uncommon official features |
| Queue inspect | `QueueInspect` | Thin wrapper exists | Keep wrapper thin |
| Flow control | `Flow`, `NotifyFlow`, `NotifyBlocked` | Channel `Flow`/`NotifyFlow` and connection `NotifyBlocked` are available | Keep wrappers thin |
| Publisher confirms | `Confirm`, `NotifyPublish`, deferred confirm | Confirm APIs and context deferred confirm wrapper are available | Keep wrapper thin |
| Logging | Caller-integrated logging | `WithLogger` injects a custom logger; `Load(...).Build()` injects an Ego `elog` adapter when `enableAccessInterceptor` is enabled | Keep low-level logger interface; do not force body logging on raw calls |
| Metrics | Caller-integrated metrics | `WithMetrics` injects a custom collector; `Load(...).Build()` injects an Ego `emetric` adapter when `enableMetricInterceptor` is enabled | Record connection, channel, publish latency, publish count, and confirm ack/nack |
| Tracing | Header propagation is application-level | `PublishWithContext` injects trace headers; `InjectTraceHeaders` and `ExtractTraceContext` are available for manual use | Keep AMQP headers explicit; do not hide consumer handler context semantics |
| Reconnect | Application responsibility | Reconnect policy exists and `Reconnect()` is explicit; no background lifecycle supervisor | Do not claim full auto recovery in the base component |
| Health | Notify close/blocked, optional lightweight checks | `Health`, `HealthStatus`, and `Ping(ctx)` are available | Use for readiness/liveness; keep blocked events and management API health separate |

## Phase 1 Scope

Phase 1 is intentionally small and covers the base compatibility layer:

- Restore `DefaultConfig()` value-return API compatibility.
- Make `WithLogger` and `WithMetrics` apply to `Client`.
- Wire `Options.Dial`, `Options.Auth`, and `Options.ConnectionName`.
- Load `TLSCACert` into a real certificate pool.
- Add raw connection/channel accessors.
- Add wrappers for `QueueInspect`, `Flow`, `PublishWithDeferredConfirmWithContext`,
  and connection `NotifyBlocked`.
- Adjust docs so they do not claim full Ego observability or full automatic
  recovery before those features exist.

## Phase 2 Scope

Phase 2 adds the Ego observability adapter layer:

- Adapt `*elog.Component` to the `Logger` interface.
- Adapt Ego `emetric` counters, histograms, and gauges to `MetricsCollector`.
- Inject those adapters from `Load(...).Build()` when
  `enableAccessInterceptor`, `enableMetricInterceptor`, and
  `enableTraceInterceptor` are enabled.
- Add AMQP header trace helpers and automatic trace injection in
  `PublishWithContext`.
- Record publisher confirm ack/nack metrics in the high-level `Producer`.

## Phase 3 Scope

Phase 3 adds explicit health boundaries:

- Add `HealthStatus()` and `Health()` for low-cost connection state checks.
- Add `Ping(ctx)` for readiness checks that open and close an AMQP channel.
- Keep `NotifyBlocked()` as the separate RabbitMQ flow-control signal.
- Keep `Reconnect()` explicit; do not introduce a background reconnection or
  consumer topology rebuild supervisor in the base component.

## Out Of Scope For Current Phases

- Full consumer supervisor with topology rebuild after reconnect.
- Automatic trace context delivery into every consumer handler.
- Full Ego access-log formatting for every raw AMQP operation.
- Configuring every exchange, queue, binding, producer, and consumer from config.
- Rewriting examples before the component boundary is stable.
