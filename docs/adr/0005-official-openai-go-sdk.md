# ADR 0005：模型 Adapter 使用官方 OpenAI Go SDK

## 状态

已被 [ADR 0007](./0007-codex-harness.md) 替代，2026-09-06。以下为历史决策；harness 分支已删除此 Adapter。

## 决策

- 保留 Kael 的 Model Provider port，避免 Runtime、领域对象和测试依赖具体模型 SDK。
- Provider Adapter 统一使用官方 `github.com/openai/openai-go`。OpenAI 使用 Responses API；OpenAI-compatible 与 DeepSeek 使用 Chat Completions。
- 删除手写请求结构、HTTP 调用、SSE 解码、流式工具参数拼接和 OpenAI API 错误结构。
- 代理、TLS 1.2 和超时通过 SDK 的 HTTP client 注入；关闭 SDK 自动重试，由 Runtime 的请求预算和幂等策略决定是否重试。
- DeepSeek 的 `reasoning_content` 只作为 SDK 未建模的兼容扩展字段读取，不另建一套协议类型。

## 结果

模型拒绝通过 SDK 的 `refusal` 字段作为正常输出交付，不再被误判为整个 AI 运行失败。SDK 响应仍映射为 Kael 的统一 Result、Usage 和 ProviderError，具体 SDK 类型不会进入 domain/application。
