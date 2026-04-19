# eamqp 历史实现计划

本文档保留为历史入口，不再作为首版能力说明或实施计划。

首版正式文档：

- [README.md](./README.md)
- [CONFIGURATION.md](./CONFIGURATION.md)
- [CAPABILITY_MATRIX.md](./CAPABILITY_MATRIX.md)

首版交付边界：

- `Load(...).Build()` 遵循 Ego 组件习惯，按 `onFail` 处理启动失败。
- `Load(...).BuildE()` 用于需要错误返回的场景，例如 examples 和集成测试。
- 可观测开关使用 `enableAccessInterceptor`、`enableMetricInterceptor`、`enableTraceInterceptor`。
- `reconnectInterval` 和 `reconnectMaxAttempts` 只描述显式重连 helper 策略。
- 组件不承诺后台自动恢复 topology、channel、consumer、exchange、queue 或 binding。
