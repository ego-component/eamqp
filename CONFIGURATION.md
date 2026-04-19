# eamqp 配置文件使用教程

`eamqp` 支持 Ego 标准配置驱动方式：

```go
client := eamqp.Load("amqp.default").Build()
```

`Build()` 遵循 Ego 组件习惯：启动失败时按 `onFail` 处理。默认 `onFail = "panic"`，连接失败直接 fail fast；如果业务需要自行处理错误，使用 `BuildE()`：

```go
client, err := eamqp.Load("amqp.default").BuildE(eamqp.WithOnFail("error"))
if err != nil {
	return err
}
```

## 本地 RabbitMQ

```bash
docker run -d --name rabbitmq-dev \
  -p 5672:5672 \
  -p 15672:15672 \
  rabbitmq:3-management
```

默认 AMQP 地址是 `amqp://guest:guest@127.0.0.1:5672/`，管理后台是 `http://127.0.0.1:15672`。

## TOML 配置

```toml
app.name = "order-service"
app.mode = "dev"

[amqp.default]
addr = "amqp://guest:guest@127.0.0.1:5672/"
vhost = "/"

heartbeat = "10s"
channelMax = 0
frameSize = 0
locale = "en_US"

poolSize = 1
poolMaxIdle = 2
poolMaxLife = "1h"

channelPoolSize = 1
channelPoolMaxIdle = 2
channelPoolMaxLife = "5m"

reconnectInterval = "5s"
reconnectMaxAttempts = 0

enableAccessInterceptor = false
enableMetricInterceptor = true
enableTraceInterceptor = true

clientName = "order-service"
onFail = "panic"
```

多实例放在不同 key 下：

```toml
[amqp.order]
addr = "amqp://guest:guest@127.0.0.1:5672/"
clientName = "order-service-order"

[amqp.audit]
addr = "amqp://guest:guest@127.0.0.1:5672/"
clientName = "order-service-audit"
```

```go
orderMQ := eamqp.Load("amqp.order").Build()
auditMQ := eamqp.Load("amqp.audit").Build()
```

## Ego 初始化

```go
package main

import (
	"context"

	"github.com/ego-component/eamqp"
	"github.com/gotomicro/ego"
	"github.com/gotomicro/ego/core/elog"
)

var rabbit *eamqp.Client

func main() {
	eg := ego.New(
		ego.WithHang(true),
		ego.WithBeforeStopClean(closeAMQP),
	)

	eg.Invoker(initAMQP)

	if err := eg.Run(); err != nil {
		elog.Panic("startup", elog.FieldErr(err))
	}
}

func initAMQP() error {
	rabbit = eamqp.Load("amqp.default").Build()
	if rabbit == nil {
		return nil
	}
	return rabbit.Ping(context.Background())
}

func closeAMQP() error {
	if rabbit == nil {
		return nil
	}
	return rabbit.Close()
}
```

如果你希望启动失败返回错误而不是 panic：

```go
func initAMQP() error {
	client, err := eamqp.Load("amqp.default").BuildE(eamqp.WithOnFail("error"))
	if err != nil {
		return err
	}
	rabbit = client
	return nil
}
```

## YAML 写法

```yaml
app:
  name: order-service
  mode: dev

amqp:
  default:
    addr: amqp://guest:guest@127.0.0.1:5672/
    vhost: /
    heartbeat: 10s
    poolSize: 1
    channelPoolSize: 1
    reconnectInterval: 5s
    reconnectMaxAttempts: 0
    enableAccessInterceptor: false
    enableMetricInterceptor: true
    enableTraceInterceptor: true
    clientName: order-service
    onFail: panic
```

## 字段说明

| 配置项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `addr` | string | 必填 | AMQP URI。多个 URI 用英文逗号分隔。 |
| `vhost` | string | 空 | 覆盖 URI 中的 vhost；为空时保留 URI 自身 vhost。 |
| `username` | string | 空 | 覆盖 URI 中的用户名。 |
| `password` | string | 空 | 覆盖 URI 中的密码。 |
| `heartbeat` | duration | `10s` | AMQP heartbeat。 |
| `channelMax` | int | `0` | 最大 channel 数，`0` 表示服务端默认值。 |
| `frameSize` | int | `0` | 最大 frame 大小，`0` 表示服务端默认值。 |
| `locale` | string | `en_US` | AMQP locale。 |
| `poolSize` | int | `1` | 连接池大小。 |
| `poolMaxIdle` | int | `2` | 最大空闲连接数。 |
| `poolMaxLife` | duration | `1h` | 连接最大生命周期。 |
| `channelPoolSize` | int | `1` | 每个连接下的 channel 池大小；`> 0` 时启用池模式。 |
| `channelPoolMaxIdle` | int | `2` | 每个连接下最大空闲 channel 数。 |
| `channelPoolMaxLife` | duration | `5m` | channel 最大生命周期。 |
| `reconnectInterval` | duration | `5s` | 显式重连 helper 的初始间隔。 |
| `reconnectMaxAttempts` | int | `0` | 显式重连 helper 最大尝试次数，`0` 表示不限。 |
| `enableAccessInterceptor` | bool | `false` | 是否注入 Ego logger 记录组件访问/生命周期日志。 |
| `enableMetricInterceptor` | bool | `true` | 是否注入 Ego `emetric` 指标采集。 |
| `enableTraceInterceptor` | bool | `true` | 是否在 `PublishWithContext` 中自动注入 trace headers。 |
| `clientName` | string | 空 | RabbitMQ management UI 中展示的连接名。 |
| `onFail` | string | `panic` | 启动失败行为，支持 `panic` 或 `error`。 |
| `tlsCertFile` | string | 空 | 客户端证书文件路径。 |
| `tlsKeyFile` | string | 空 | 客户端私钥文件路径。 |
| `tlsCaCert` | string | 空 | CA 证书路径。 |
| `tlsServerName` | string | 空 | TLS SNI/server name。 |

## 可观测性

`Load(...).Build()` 会按开关注入 Ego adapter：

- `enableAccessInterceptor = true`：注入 `elog.Component`。
- `enableMetricInterceptor = true`：注入 `emetric` 指标采集器。
- `enableTraceInterceptor = true`：`PublishWithContext` 自动写入 AMQP trace headers。

`PublishWithContext` 的 context 当前主要用于 trace 传播；在 amqp091-go v1.9.0 下不要把它当成可靠的 publish 超时或取消机制。需要确认消息被 broker 接收时，应使用 publisher confirms。

组件还会注册 governor 端点：

```text
/debug/amqp/stats
```

`HealthStatus()` / `Health()` 是低成本内存状态检查；`Ping(ctx)` 会打开并关闭一个 AMQP channel，适合 readiness check。`NotifyBlocked()` 是 RabbitMQ 流控事件，不属于健康检查结果，应由业务自行降级处理。

## 重连边界

`eamqp` 不会后台自动恢复 channel、consumer、exchange、queue 或 binding。`Reconnect()` 只重建连接层；消费端如果需要完整恢复，应由业务 supervisor 监听 close，重新连接，重建 topology，再重新消费。

`reconnectInterval` 和 `reconnectMaxAttempts` 只用于显式重连 helper 的策略构造：

```go
policy := client.Config().ReconnectPolicy()
_ = policy
```

## Channel 池边界

Channel 池只复用包装层能判断为无状态的 channel。执行过 `Confirm`、`Qos`、`Tx`、`Consume`、`Notify*` 或 `RawChannel()` 的 channel 在 `Close()` 时会关闭丢弃，不会放回空闲池，避免 confirm、QoS、consumer、notify listener 等 AMQP channel 状态泄漏给下一次使用。

调用 `NotifyClose`、`NotifyPublish`、`NotifyReturn` 等 AMQP 异步通知后，调用方必须持续消费返回的 channel，直到它关闭。这是 amqp091-go 的基础契约，也是避免通知 goroutine 阻塞的前提。

## Examples

本仓库 examples 使用 `examples/config/local.toml`：

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
go run ./examples/dead-letter --config=examples/config/local.toml publisher
go run ./examples/retry-consumer-sender --config=examples/config/local.toml
go run ./examples/pubsub-fanout --config=examples/config/local.toml publish
go run ./examples/reconnect --config=examples/config/local.toml producer
```

`examples/producer` 与 `examples/consumer` 使用同一组 direct exchange / queue / routing key，可在两个终端配对验证发布和消费。`examples/batch-producer` 使用 publisher confirms，演示的是批量 helper 与可靠发布，不是吞吐基准。

也可以通过环境变量指定配置：

```bash
EAMQP_EXAMPLE_CONFIG=examples/config/local.toml go run ./examples/producer
```

## 拓扑配置

当前不建议把 exchange、queue、binding 放入 `eamqp.Config`。连接配置只描述连接、池化、TLS、显式重连策略和可观测性；业务拓扑、QoS、publisher confirm、消费并发应在业务初始化代码中显式声明。
