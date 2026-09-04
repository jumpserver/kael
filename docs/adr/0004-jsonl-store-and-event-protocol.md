# ADR 0004：JSONL Store 与 Event 协议

## 状态

已接受，2026-09-04。本文定义 JSONL 记录格式、状态恢复和 Event 协议；“本地 JSONL 是默认 Store”的实现选择已由 [ADR 0006](./0006-core-backed-runtime-journal.md) 替代。本地 JSONL 继续作为显式回退。

## 背景

Kael 已把长期对话与浏览器执行连接拆开。若只保留内存 Store，Conversation 历史和领域事件会在重启后消失；同时，旧式 Agent Session 把消息历史、上下文、工具连接、运行状态与一个 SSE cursor 混成单一对象，不适合多 Panel、长期 Conversation 和安全恢复。

## 决策

### JSONL 回退 Store

- Runtime 继续只依赖 `ports.Store`/`ports.Tx`；本 ADR 定义的 adapter 是单进程、本地 JSONL Store，不连接数据库。它现在只在显式设置 `RUNTIME_STORE=jsonl` 时启用，默认 Store 见 ADR 0006。
- `data/store/runtime.jsonl` 是事务状态 journal，保存 Conversation、Message、Run、DomainEvent、幂等索引、审计及其它 Store 状态。记录带版本与校验和，启动时只截断未完整写入的最后一条记录，并按阈值原子压缩为快照。
- `data/events/<conversation-id>.jsonl` 是按 Conversation 分隔、可直接读取的 DomainEvent 归档。这里的 `seq` 是 Conversation 事件序列，不是某个 Panel 的 SSE cursor。
- DomainEvent 归档作为历史保留；bootstrap 中的 `event_retention_seconds` 当前约束 PanelDelivery/SSE 重放窗口，不删除 Conversation DomainEvent 归档。
- 领域状态与新 DomainEvent 先持久写入，再对订阅者发布；持久化失败时该事务不对 Runtime 可见。
- Kael 不读取或导入 Koko `data/agent/events/*.jsonl`。JSONL 回退 Store 只保存启用该 adapter 后由 Kael 自身创建的 Runtime 状态与事件，也不会与 Core-backed Journal 双写。

### 重启恢复

- Conversation、Message、已完成 Run、DomainEvent 和审计可从同一份已配置 Journal 跨 Kael 重启恢复；JSONL 回退要求复用同一数据目录，Core-backed adapter 则从 Core 重新加载。
- 启动时尚未完成的 Run 统一收敛为 `interrupted/process_restarted`；相关未完成 Message 标为 `cancelled`，活动 ToolCall 标为 `cancelled`，未决 Approval 标为 `expired`。
- PanelSession 与 Registration 标为 `expired`，因为浏览器连接、executor channel 和 lease owner 不可能由磁盘安全恢复。Luna 重新打开会话时必须建立新 PanelSession、提交最新 Context 并重新注册工具。
- PanelDelivery 的历史可以留在 Store 中作审计，但旧 PanelSession 的 cursor 不能作为新连接的续传能力；新 PanelSession 从自己的 sequence 开始。

## 新协议对象与旧 Agent Session 的职责拆分

| 旧 Agent Session 职责 | Kael 对象 | 生命周期与真值 |
|---|---|---|
| 会话标识、消息历史 | `Conversation` + `Message` | 可持久化；与浏览器 Tab 生命周期解耦 |
| 一次模型/工具编排 | `Run` | 属于 Conversation；重启中的 Run 只收敛，不自动续跑 |
| 当前页面与资源上下文 | `ContextSnapshot` | 属于 PanelSession 的版本化快照 |
| 当前 Tab 的工具清单 | `Registration` | lease 约束的临时能力；重连或重启后重新注册 |
| 浏览器 executor 与路由 | `PanelSession` | 临时连接/能力附件；一个 Panel 只绑定一个 Conversation，一个 Conversation 可被多个 Panel 打开 |
| 领域事件真值 | `DomainEvent` | 按 Conversation 持久化，使用 Conversation 内 `seq` |
| SSE sequence/cursor | `PanelDelivery` | DomainEvent 针对某个 Panel 的投影视图；每个 Panel 有独立 sequence |

PanelSession 不是旧 Session 的改名，也不是会话历史容器。它只回答“此刻哪个 UI/Tab 可以接收事件并执行哪些本地能力”；Conversation 回答“用户在和谁进行哪段对话以及历史是什么”。因此关闭 Tab 或重启 Kael 会使 PanelSession 失效，但不会删除 Conversation 历史。

## 协议流程

1. Luna 创建或打开 Conversation。
2. Luna 为当前 Tab 创建 PanelSession，上传版本化 Context，并原子替换 Registration。
3. 用户创建 Message/Run；Kael 写入领域状态和 DomainEvent。
4. Event Projector 为有权限且仍有效的 PanelSession 生成 PanelDelivery。
5. Luna 通过 `GET /kael/api/v1/panel-sessions/:id/events` 按该 Panel 的 cursor 消费 SSE；历史页面分别读取 Conversation、Message 和 Run API，不能把 SSE 当长期历史来源。

主要事件名从“整个 Agent Session 状态”改为“领域事实 + Panel 投影”：

| 兼容事件 | Kael 事件 |
|---|---|
| `session.created` / `session.closed` | `panel.ready` / `panel.lease_expiring` |
| `approval.requested` | `approval.required` |
| 单一 `tool.result` | `tool.progress` / `tool.completed` / `tool.failed` / `tool.cancelled` |
| `message.*`、`run.*`、`model.*` | 保留领域名称，但增加稳定的 Conversation/Run/Message 引用 |

Luna 当前的 `AgentClient` 为降低一次性 UI 改动，仍在内部暴露 `createSession`、`session_id` 等兼容命名，并把 Kael 事件映射回旧 `AgentEvent` 名称；其中 `session_id` 实际是 `panel_session_id`。这只是 Luna 边界 Adapter，不是 Kael 的服务端协议，也不能再被当作持久会话 ID。消息展示继续使用 AI SDK 的 `UIMessage` 数据结构，但权威状态来自 Kael Conversation/Run/Event。

## 后果

Kael 不需要直接连接数据库即可保留自身会话与事件。本地 JSONL 回退只支持单写实例；Core-backed Journal 同样尚不提供状态同步、分布式 ownership 或非幂等工具恢复。后台 Run、跨实例共享、未决 Approval 续接和浏览器 ToolCall 恢复仍未启用。旧 Koko 历史的保留或迁移不属于 Kael 启动流程。
