# eamqp 设计说明

本文档曾用于早期方案推演。首版交付后的正式能力边界以以下文档为准：

- [README.md](./README.md)
- [CONFIGURATION.md](./CONFIGURATION.md)
- [CAPABILITY_MATRIX.md](./CAPABILITY_MATRIX.md)

当前首版定位：

- 提供 Ego 配置驱动的 RabbitMQ AMQP 0-9-1 client component。
- 保留 `amqp091-go` 的连接、channel、confirm、notify、raw access 能力。
- 提供连接池、channel 池、Ego logger/metrics/trace adapter、health check、governor stats。
- 不在后台自动恢复 topology、channel、consumer、exchange、queue 或 binding。
- 需要完整消费端恢复时，由业务 supervisor 监听 close，重连，重建 topology，再重新消费。
