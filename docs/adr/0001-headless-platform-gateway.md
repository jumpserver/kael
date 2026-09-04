# ADR 0001：过渡 Headless Platform Gateway

## 状态

已接受，2026-09-03。

ADR 0002 已覆盖本文对后台 Run、持久 Approval 和 Kael 自有数据库的实现假设。Headless Gateway 当前只用于前台 service capability；隔离 Provider、HMAC 委托和 Core 最终 RBAC 决策不变。

## 背景

本期只能修改 Kael 和 Luna，但 Luna 切到 Kael 后仍要保留 Platform AI 的动态 Core OpenAPI、后台 Run、持久 Approval 和 Core 二次 RBAC。仅依赖浏览器 Panel 无法在页面关闭后继续这些任务；把 Core 业务规则放进 Runtime core 又会破坏产品无关边界。

## 决策

采用 Kael 仓库内的过渡 Headless Platform Gateway：

- Gateway 位于独立 adapter 包，只在 composition root 装配；Runtime 只依赖通用 `CapabilityProvider` 接口。
- `ExecutionBinding` 明确区分 `panel` 与 `service`。service 调用不经 Panel SSE 执行，Run 固定 registration ID、definition version/digest 和 binding ID。
- Gateway 启动时加载 Core OpenAPI，按内容 hash 版本化并保留最近四个版本；旧 Run 只能使用自己的准确快照。
- 模型只能搜索受 Profile、HTTP method 和敏感路径策略约束的 operation ID。Method、Path、Query 和 Body 均由可信 registry 和 schema 构造，不能提交任意 URL。
- 每次 Core 请求使用一次性 HMAC 委托，绑定 user、org、conversation、approval、operation、method、path、request hash、有效期和 nonce。Core 继续执行 nonce 防重放、用户状态检查、组织边界和最终 RBAC。
- HTTPS 可配置私有 CA 和客户端证书；Gateway 不继承进程代理。生产部署按 Core 的信任策略启用 TLS/mTLS。
- write/dangerous 调用使用 service-scoped Approval。审批只继续原 ToolCall 和参数 digest，不能替换 operation 或参数；当前决定只在 Kael 进程生命周期内有效。
- 外部调用结果未知时不自动重放。被中断 Run 只要已经产生 ToolCall，`resume` 返回 `execution_rebind_required`；用户可查看审计状态并取消，不能用恢复接口触发第二次执行。
- Core 响应先限长、脱敏，再进入 ToolResult、result card 和审计摘要。

## Legacy 数据与切流

- Luna 切流后创建的新 Conversation、Message、Run、Approval、Artifact 和 Event 只写 Kael。
- 旧 Platform AI 数据继续由旧服务只读持有；本期不双写、不猜测字段映射、不静默导入。
- Lina 本期不修改，仍由生产网关访问旧 `/api/v1/chat-ai/`。这不构成 Kael 的兼容业务路径。
- 回滚时将 Luna 的 `/kael/` 流量恢复到上一版本，并停止新 Kael Run；已经进入 Kael 的对象不交给旧 Runtime 继续推进。

## Feature 决策

Kael bootstrap 是功能可用性的权威来源。本期启用 Conversation、Artifact、branch、regenerate、进程内 SSE replay、panel capability 和按配置启用的前台 service capability。`background`、Web Search、STT 和站内通知明确返回 `false`，Luna 不把它们显示为 Kael 已提供能力；Lina 对应旧功能仍由旧服务承接。

## 后果

本决策完成只改 Kael/Luna 条件下的逻辑迁移，并保持 Runtime core 可替换。部署整体暂时仍有 Kael Gateway 到 Core 的业务连接。当前历史状态由 ADR 0004 的本地 JSONL Store 保留，活动 Panel 能力和未决 Approval 仍为进程绑定；未来 Lina/Core 提供正式 Provider 或外部 Store 后，只替换对应端口 adapter，不改变 Conversation、Run、Approval 或 Event 状态机。
