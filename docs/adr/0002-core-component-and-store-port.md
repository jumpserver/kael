# ADR 0002：Core Component 与 Store Port

## 状态

已接受，2026-09-03；其中“当前使用进程内 Store”的实现选择已由 [ADR 0004](./0004-jsonl-store-and-event-protocol.md) 替代。本文关于 Store port、无数据库、组件身份和模型配置的边界继续有效。

## 背景

Kael 与 Koko 一样是 JumpServer Terminal component。模型配置已经由 Core 的 TerminalConfig 统一管理，不应在 Kael 配置文件中重复保存。现阶段也不允许 Kael 直接连接业务数据库，但 Runtime 需要保留可替换的状态存储边界。

## 决策

- Kael 使用 component type `kael` 调用 Core Terminal registration；首次注册使用 BootstrapToken，后续使用保存在私有文件中的 AccessKey。
- Kael 使用组件签名身份校验 AccessKey、读取 `/api/v1/terminal/terminals/config/` 并提交 heartbeat。
- 模型 Provider 只从 TerminalConfig 的 `CHAT_AI_*` 字段构建，定期刷新；模型 API Key 不出现在 Kael 配置文件、API、Event 或日志中。
- Runtime 和 application 仅依赖 `ports.Store` 与 `ports.Tx`，不导入数据库产品、ORM 或 Core 业务存储实现。
- 当前 Store adapter 的持久化与恢复语义见 ADR 0004。Runtime 仍不感知 adapter 是内存、本地 JSONL 或未来外部存储。
- 当前不提供多实例共享或 durable background Run。JSONL 可恢复历史，但不能恢复浏览器执行连接；bootstrap 返回 `storage.kind=jsonl`、`storage.durable=true` 和 `background=false`。
- Artifact 内容仍使用服务账号私有目录；组件 AccessKey 默认位于 `data/keys/.access_key`。这些文件不等同于 Runtime 状态数据库。
- 未来持久化只能通过新增 Store adapter 接入。替换 adapter 不得改变领域对象、HTTP API、Event schema 或 Luna 协议。

## 后果

Kael 可以像 Koko 一样由 Core 注册和集中下发模型配置，且本地启动不再要求 DSN。Conversation 历史由 ADR 0004 的 JSONL adapter 保留；浏览器能力和 SSE stream 仍是进程绑定的。任何要求后台运行、未决 Approval 续接或多实例无状态切换的部署，都必须先提供外部 Store adapter 和相应的安全恢复协议。
