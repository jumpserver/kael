# ADR 0001：过渡 Headless Platform Gateway

## 状态

已接受，2026-09-03。

ADR 0002 已覆盖本文对后台 Run、持久 Approval 和 Kael 自有数据库的实现假设，ADR 0006 又将默认 Runtime Store 从本地 JSONL 改为 Core-backed Journal。Headless Gateway 当前只用于前台 service capability；隔离 Provider、HMAC 委托和 Core 最终 RBAC 决策不变。

## 背景

本期只能修改 Kael 和 Luna，但 Luna 切到 Kael 后仍要保留 Platform AI 的动态 Core OpenAPI、后台 Run、持久 Approval 和 Core 二次 RBAC。仅依赖浏览器 Panel 无法在页面关闭后继续这些任务；把 Core 业务规则放进 Runtime core 又会破坏产品无关边界。

## 决策

采用 Kael 仓库内的过渡 Headless Platform Gateway：

- Gateway 位于独立 adapter 包，只在 composition root 装配；Runtime 只依赖通用 `CapabilityProvider` 接口。
- `ExecutionBinding` 明确区分 `panel` 与 `service`。service 调用不经 Panel SSE 执行，Run 固定 registration ID、definition version/digest 和 binding ID。
- Gateway 启动时加载 Core OpenAPI，按内容 hash 版本化并保留最近四个版本；旧 Run 只能使用自己的准确快照。
- 模型只能搜索受 Profile、HTTP method、敏感路径和权限策略约束的 operation ID。`general` 使用与当前 JumpServer 源码默认 `CHAT_AI_ALLOWED_OPERATION_IDS` 等价的编译期固定范围，asset/session_audit/ops 继续叠加更窄 scope，management 仅管理员可用但保持较宽范围；默认 method 为 `GET/POST/PUT/PATCH`，`DELETE` 只有部署者显式开启才可用。Kael 不读取现网对 operation IDs、allowed/blocked paths/tags 或 method policies 的自定义值，生产切流前必须对比并显式处置差异。Method、Path、Query 和 Body 均由可信 registry 和 schema 构造，不能提交任意 URL。
- Registry 读取 Core OpenAPI 的 `x-jms-required-permissions` 与 `x-jms-permission-dynamic`；权限元数据缺失/非法或 dynamic operation 一律拒绝。搜索与最终 resolve/call 使用同一 predicate，并要求 Principal 拥有全部静态 required permissions。
- service Run 固化创建时的 superuser/org-admin flags 和权限列表，供异步模型轮次维持相同的搜索可见性；Core 接收 HMAC 委托后仍按该 user/org 的实时状态和 DRF RBAC 复核，因此旧快照不能在权限撤销后授权执行。
- 每次 Core 请求使用一次性 HMAC 委托，绑定 user、org、conversation、approval、operation、method、path、request hash、有效期和 nonce。Core 继续执行 nonce 防重放、用户状态检查、组织边界和最终 RBAC。
- HTTPS 可配置私有 CA 和客户端证书；Gateway 不继承进程代理。生产部署按 Core 的信任策略启用 TLS/mTLS。
- write/dangerous 调用使用 service-scoped Approval。审批只继续原 ToolCall 和参数 digest，不能替换 operation 或参数；当前决定只在 Kael 进程生命周期内有效。
- 外部调用结果未知时不自动重放。被中断 Run 只要已经产生 ToolCall，`resume` 返回 `execution_rebind_required`；用户可查看审计状态并取消，不能用恢复接口触发第二次执行。
- Core 响应先限长、脱敏，再进入 ToolResult、result card 和审计摘要。

## 数据与切流

- Luna/Lina 切流后创建的 Conversation、Message、Run、Approval、Artifact 元数据和 Event 只进入 Kael Runtime，并默认以 opaque Journal 保存在 Core Runtime Store。
- Core 的旧 Platform AI ORM Runtime、API 和 worker 已删除，migration 历史也已收敛为只创建 Runtime Store。本功能尚未上线，不提供旧数据导入、双写、只读入口或 Runtime 兼容；使用过旧开发分支的环境应删除旧 AI 表或重建开发数据库。
- Lina 已切换到 Kael API；这不改变本 ADR 的 Capability 安全边界，也不保留旧路径或旧 DTO 回退。
- 回滚时先停止创建新 Kael Run，排空或取消在途 Run/Approval，再成对恢复相互匹配的 Lina/Luna/Kael 构建；不得恢复已删除的 Core ChatAI Runtime，并须保留 Core Journal 与 Artifact 卷。`RUNTIME_STORE=jsonl` 不是 Core Journal 的自动 failover 或数据回滚手段。

## Feature 决策

Kael bootstrap 是服务端功能可用性的权威来源。本期启用 Conversation、Artifact、branch、regenerate、进程内 SSE replay、panel capability 和前台 service capability。`background`、Web Search、服务端 STT 和站内通知明确返回 `false`，Luna/Lina 不把它们显示为 Kael 已提供能力，也不回退旧 Runtime；Lina 可在浏览器支持时使用浏览器原生 `SpeechRecognition`，该能力不经过 Kael，也不表示服务端 STT 已启用。

Lina 未传 Profile 时默认选择 `general`，因此 Platform Gateway 是 Kael 的必备启动依赖，不再是可降级功能。`PLATFORM_GATEWAY_ENABLED` 默认且必须为 `true`；显式关闭、密钥缺失或 Registry 初始化失败都会阻止 Kael 监听端口。部署必须提供与 Core 匹配的 delegation key ID/secret，仓库和镜像不内置可工作的共享密钥。

## 后果

本决策完成原“只改 Kael/Luna”条件下的逻辑迁移，并保持 Runtime core 可替换。部署整体暂时仍有 Kael Gateway 到 Core 的业务连接。当前历史状态按 ADR 0006 由 Core-backed Runtime Journal 保留，活动 Panel 能力和未决 Approval 仍为进程绑定；未来 Lina/Core 提供正式 Provider 或分布式执行协调后，只替换对应端口 adapter，不改变 Conversation、Run、Approval 或 Event 状态机。
