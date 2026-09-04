# 旧版 Platform AI 契约与当前迁移边界

## 1. 目的

本文把已删除的 Core Platform AI Runtime 契约作为历史参考，并记录当前 Kael、Luna、Lina 的迁移边界。标为“旧版”的章节不描述当前可调用能力，也不构成兼容承诺。

本文与 [Kael AI Runtime Architecture](./ARCHITECTURE.md) 及 [AI Native Agent Runtime 迁移与演进方案](./ai-native-agent-runtime-migration.md) 共同构成迁移基线。

当前实现以 [ADR 0002](./adr/0002-core-component-and-store-port.md)、[ADR 0004](./adr/0004-jsonl-store-and-event-protocol.md) 和 [ADR 0006](./adr/0006-core-backed-runtime-journal.md) 为准：Kael 不连接数据库，默认使用 Core-backed Journal 保留由 Kael 创建的 Conversation 历史和 DomainEvent，本地 JSONL 只作为显式回退；前台 Platform capability 可用，后台 Run、活动执行跨重启续跑及持久 Approval 均未启用。

## 2. 真值来源

当前行为判断按以下优先级：

1. Kael 当前代码、OpenAPI 和可执行协议；
2. 当前 Luna/Lina 客户端代码；
3. Core 当前的 Runtime Store、组件配置、身份和委托接口；
4. 当前自动化测试和可录制协议。

旧 `jumpserver/apps/chat_ai` Runtime、models、worker 和业务 API 已删除，`/api/v1/chat-ai/` 下只剩仅供 Kael 组件访问的 `runtime-store/`。旧 `chat_ai_backend.md`、SSE Schema、历史代码和产品设计稿只用于理解已删除行为。

现有 `chat_ai_backend.md` 和 SSE Schema 已与代码发生漂移，不能直接作为迁移黄金契约：

- 旧 Platform 只有默认 `general` Assistant，并以固定 operation allowlist 承载授权 Core 搜索/调用；Kael 将当前 JumpServer 源码默认 allowlist 固化给 Lina 默认 `general`，同时保留管理员 `management` 和几个更窄 Profile，但不把整个 OpenAPI 暴露给普通用户。功能尚未上线，旧开发分支中的 Chat AI operation IDs、paths/tags 或 method policies 自定义配置不读取、不迁移；当前编译期策略是唯一生效配置。
- 文档中的 Scheduled Report API、模型和任务在当前代码中不存在，不属于现有能力。
- 文档遗漏 PATCH、图片/文件下载、branch、multipart、部分后台参数和完整 Message 字段。
- SSE Schema 未覆盖实际的 `conversation_id` 和 `management`，不能作为唯一协议真值。

旧版行为若需追溯，应从对应历史提交或已冻结 fixture 获取；当前验收以 Kael 与现有客户端协议为准。

## 3. 与两类对话的映射

| 产品类型 | Platform AI 关系 |
|---|---|
| 普通对话 | 承接 Platform AI；不绑定当前 Luna Terminal/File/SQL/Script 资源 |
| Luna 能力对话 | 不继承 Platform 能力；只使用本次 PanelSession 明确注册的 Luna Session Capability |

普通对话允许不同 Assistant/Profile：

| Assistant | 当前代码语义 |
|---|---|
| `general` | Lina 默认的旧统一 JumpServer Assistant；通用问答，以及与当前 JumpServer 源码默认 allowlist 等价的编译期固定范围内、静态权限元数据齐全且用户拥有全部 required permissions 的 Core 搜索/调用 |
| `management` | 管理员可用；在当前权限和安全策略内访问允许的 Core 操作；写操作需 Approval |
| `asset` | 资产、节点、平台、协议等受限只读操作 |
| `session_audit` | 会话、命令、登录、访问、操作和工单审计只读操作 |
| `ops` | 作业、任务、组件指标和终端健康只读操作 |

Assistant 是可信策略与能力组合，不是 Conversation 类型，也不能替代 Registration。

## 4. 旧版 HTTP 契约（历史参考）

旧版逻辑前缀为 `/api/v1/chat-ai/`。下列 4.1、4.2 表格只记录已删除接口，不表示 Core 当前仍提供这些路由。Kael 的唯一权威业务根路径是 `/kael/api/v1`；Luna 与 Lina 都直接使用新路径，不保留旧 Platform AI Runtime 回退。

旧契约只用于校验新实现覆盖的产品语义，不形成 URL 或请求格式兼容层。Kael 调用方必须使用不带尾斜杠的 canonical 路径；旧 Core URL、尾斜杠变体和 Redirect 行为均不作兼容承诺。

### 4.1 Conversation 与 Message

| 方法 | 路径 | 旧版行为 |
|---|---|---|
| GET | `conversations/` | 当前 user/org 的分页列表 |
| POST | `conversations/` | 创建对话并由服务端绑定 user、org、model、assistant |
| GET | `conversations/{id}/` | 对话详情 |
| PATCH | `conversations/{id}/` | 修改标题、assistant 等允许字段 |
| DELETE | `conversations/{id}/` | 删除；存在 queued/running Run 时返回 `409 CONVERSATION_BUSY` |
| GET | `conversations/{id}/messages/` | 分页历史，包含附件、Token、result cards 和再生成关系 |
| GET | `conversations/{id}/messages/{message_id}/images/{image_id}/` | 所有权校验后的内联图片 |
| GET | `conversations/{id}/messages/{message_id}/files/{file_id}/` | 所有权校验后的附件下载 |
| POST | `conversations/{id}/messages/stream/` | 文本、图片、文件和可选联网搜索；返回 SSE |
| POST | `conversations/{id}/messages/{message_id}/regenerate/` | 从原用户问题生成新的 Assistant Message |
| POST | `conversations/{id}/messages/{message_id}/branch/` | 复制有界历史与附件到新 Conversation；返回新 Conversation ID Header |
| POST | `conversations/{id}/messages/background/` | 创建后台 Run，返回 task/run/message ID |
| POST | `conversations/{id}/cancel/` | 取消 Run、Message、未决 Approval 和后台任务 |

### 4.2 管理、Approval 与语音

| 方法 | 路径 | 旧版行为 |
|---|---|---|
| GET | `assistants/` | 返回当前用户可用的 Assistant/Profile |
| GET | `approvals/{id}/` | 返回安全预览，不暴露签名、nonce 或凭据 |
| POST | `approvals/{id}/confirm/` | 锁定、复验并单次执行写操作 |
| POST | `approvals/{id}/cancel/` | 原子取消 Approval 及关联运行状态 |
| POST | `openapi/refresh/` | 超级管理员刷新动态 OpenAPI Registry |
| GET | `stats/` | 超级管理员查看组织内 Run、Token、API 和模型耗时统计 |
| GET | `audit/conversations/` | 超级管理员查看当前组织会话审计元数据 |
| GET | `audit/conversations/{id}/` | 查看脱敏消息并记录审计查看操作 |
| POST | `transcriptions/` | STT；校验格式、大小、时长和并发，不保存原始音频及转写文本 |

Scheduled Report 只存在于旧文档，不存在于当前代码，不纳入当前迁移基线。

### 4.3 Kael 当前资源

旧 URL 不做逐路径复制。Luna 可在自身适配层收敛既有 Controller；Lina 不再做 Legacy 映射，而是按 Message -> Run -> PanelSession SSE 的 Kael 原生流程调用，并直接消费 PanelDelivery dot 事件。

| 当前语义 | Kael canonical 路径 |
|---|---|
| Conversation 列表、创建、详情、修改、删除 | `/kael/api/v1/conversations`、`/kael/api/v1/conversations/{id}` |
| Message、Run 与待恢复状态 | `/kael/api/v1/conversations/{id}/messages`、`/kael/api/v1/conversations/{id}/runs`、`/kael/api/v1/conversations/{id}/approvals` |
| 图片和文件上传、鉴权读取、删除 | `/kael/api/v1/artifacts`、`/kael/api/v1/artifacts/{id}/content`、`/kael/api/v1/artifacts/{id}` |
| 前台 stream | 依次创建 Message、Run，再订阅 `/kael/api/v1/panel-sessions/{id}/events` |
| background | Kael 当前返回 `background_requires_durable_store`；Lina 已删除 UI、状态和请求，不回退旧 Runtime |
| regenerate | `/kael/api/v1/messages/{id}/regenerations` |
| branch | `/kael/api/v1/conversations/{id}/branches`，请求体携带源 Message ID |
| cancel 与恢复 | `/kael/api/v1/runs/{id}/cancel`、`/kael/api/v1/runs/{id}/resume` |
| Assistant/Profile 发现 | `/kael/api/v1/assistants`、`/kael/api/v1/runtime-profiles` |
| Approval 查询与决定 | `/kael/api/v1/approvals/{id}`、`/kael/api/v1/approvals/{id}/decisions` |
| 动态 Platform Registry 刷新 | `/kael/api/v1/admin/platform-registry/refresh` |
| stats 与 audit | Kael 保留 `/kael/api/v1/admin/stats?days={1..365}` 服务端接口，但 Lina 已删除 stats 面板和请求；Lina audit 使用 `/kael/api/v1/admin/audit/conversations/{id}` |
| 服务端 STT | `/kael/api/v1/transcriptions` 是固定 unavailable 的禁用占位；Lina 不调用，浏览器原生 `SpeechRecognition` 不经过该路径 |

旧 `messages/stream/` 的“POST 同时创建消息、运行并保持响应流”已拆成独立资源操作；断开 SSE 只取消订阅，取消 Run 必须显式调用 cancel。Lina 不保留旧 DTO adapter，Kael 中也只有一套 Run 生命周期。

## 5. 旧 Platform 数据与运行语义

### 5.1 旧服务持久对象

旧 Platform AI 至少持久化：

- Conversation；
- Message；
- MessageImage；
- MessageFile 及提取文本；
- AgentRun；
- ApiCallAudit；
- Approval。

Kael 新创建的数据保留了下列业务语义，但不会导入这些旧记录：

- user/org 所有权；
- title、assistant、model、status；
- Message role、status、Token、error 和 result cards；
- regenerate/branch 关系；
- Run 状态、step/api call 计数、模型耗时和 task ID；
- Approval 的请求摘要、hash、nonce、签名版本、风险和状态；
- API 调用的请求/响应摘要、状态、耗时和风险审计。

### 5.2 附件

当前支持图片和普通文件：

- 消息可以只有附件而没有文本；
- 图片校验 MIME、实际格式、像素、动画、单文件和总大小；
- 文件限制数量、单文件和总大小；
- 文本提取有单文件和总字符上限；
- 文件名与提取文本执行敏感信息检查；
- branch/regenerate 按当前语义复制或继承附件；
- 下载始终校验 Conversation、Message、附件所有权。

Kael 当前模型使用 Artifact 引用，避免把大对象放入 SSE Event。

### 5.3 旧版前台与后台运行

前台 Run：

- 一个 Conversation 同时只允许一个非终态 Run；
- 客户端断开触发 Provider 取消并保存 cancelled；
- SSE 心跳不属于业务 Event；
- 模型与工具状态进入 Message、Run 和审计记录。

后台 Run：

- 当前通过 Celery 排队；
- Run 从 queued 原子转为 running 后才执行；
- 保存 task ID；
- cancel 同时写数据库取消标记并 revoke 任务；
- 受用户级速率、待执行数量和每日 Token 配额约束；
- 可以在浏览器关闭后继续；
- 完成、失败或等待 Approval 后可触发站内信。

这些后台语义不属于当前迁移的兼容目标。Kael 当前明确禁用后台 Run，Lina 已删除相关入口，也不会回退到已删除的 Core Worker。

### 5.4 旧版 Web Search

旧版 Web Search：

- 默认关闭，由管理员功能开关与本次用户选择共同启用；
- 支持 Tavily 和 SearXNG；
- 使用独立 Base URL、凭据、代理、超时和结果上限；
- 不复用模型 API Key；
- 搜索结果视为不可信外部内容；
- 请求、来源、耗时和状态进入审计；
- 本轮执行过 Core API 后不再允许继续 Web Search；
- Luna 请求固定 `web_search=false`，旧 Lina 曾提供用户开关；当前 Lina 已删除该 UI、状态和请求。

### 5.5 旧版服务端 STT

旧版服务端 STT：

- 使用 OpenAI-compatible Audio Transcriptions；
- 支持常用音频格式和可选 ISO-639 language；
- 校验 Content-Length、实际大小和可选 ffprobe 时长；
- 有用户级速率、用户并发和全局并发限制；
- Provider 地址和凭据与模型服务隔离；
- 不写入业务存储。

当前 Lina 已删除服务端 STT UI 和请求；Kael 不部署这套旧依赖。浏览器原生 `SpeechRecognition` 保留。

## 6. 当前 Agent 与 Platform Capability

### 6.1 动态 Core OpenAPI

当前 Platform Agent 会：

- 从 Core OpenAPI 构建带 hash 和 TTL 的 Registry；
- 使用组件身份读取 Core OpenAPI；Core 的 Schema 缓存使用进程级版本前缀，部署重启后不会继续命中旧契约；
- 按引用环而不是普通对象层级截断 `$ref`，并把共享响应 Schema 转成请求 Schema：移除 `readOnly` 字段、同步清理 `required`、规范化 `nullable`，关联对象明确要求真实 `id` 及正确主键类型；
- 按 Assistant/Profile 固定范围、全局 method allowlist、敏感路径和用户权限筛选 Operation；OpenAPI 权限元数据缺失、非法或标记 dynamic 的 Operation fail closed；
- Run 使用创建时 admin flags/permissions 快照保持搜索与调用选择一致；delegated Core 请求仍以用户实时状态和 RBAC 做最终复核，快照不能绕过权限撤销；
- 先搜索候选 Operation，再让模型选择；搜索结果只返回有界的紧凑 Schema，常用中文资产意图会映射到稳定的 Operation 关键词；
- 只允许模型提交 operation ID、path/query/body 参数；
- 由可信 Request Builder 决定 Method 和 URL；
- 校验 required、enum、array、nullable、组合 Schema、additional properties 和 query serialization；
- 将首次参数错误返回模型进行有限修正；
- 创建 Linux 主机时只把地址视为不可推断值，名称可派生，平台 ID 通过只读 Operation 查询，协议和节点优先使用 Core 默认值；写入只由 Approval 卡确认一次，不再要求用户在对话里重复输入“确认”；
- 对 Core 响应做大小限制、摘要和敏感信息清理；
- 生成 table、timeline、detail、metric、sources、progress 等 result card。

这不是少量静态 API wrapper 能完全等价替代的能力。

### 6.2 Core 调用委托

当前服务不把用户 Cookie、Bearer 或 Access Key 重放给 Core，而是为每次请求生成短期、一次性 HMAC 委托。委托绑定：

- user、org；
- Conversation、Approval；
- operation ID；
- Method、Path；
- Query 和 Body hash；
- issuer、audience、key ID、时间和 nonce。

Core 验证签名、请求绑定和一次性 nonce，恢复真实用户后再次执行正常 RBAC、Serializer 和业务逻辑。

### 6.3 Approval

当前写操作默认需要 Approval。确认时：

- 锁定 Approval；
- 校验当前用户、组织、状态和过期时间；
- 校验签名与 request hash；
- 重新执行 policy、schema 和敏感字段检查；
- 只允许执行一次；
- 记录结果摘要和 API 审计；
- 结束等待中的 AgentRun。

当前 Kael Approval 使用同等或更强的绑定与一次性语义。

## 7. 旧版 SSE 契约与当前事件边界

Legacy 事件包括：

- `message_start`；
- `message_delta`；
- `agent_plan`；
- `agent_progress`；
- `web_search_start`、`web_search_result`；
- `api_search_start`、`api_search_result`；
- `api_call_start`、`api_call_result`；
- `approval_required`；
- `message_done`；
- `message_error`。

保留但当前不发送的 knowledge search 事件不算现有功能。

下列要求仅记录旧版协议行为：

- `message_delta` 只发送新增文本；
- 默认通过 SSE comment 发送心跳；
- `message_done` 能表达 completed、awaiting approval、cancelled；
- 错误只返回安全 code/detail，不返回堆栈；
- `api_call_result` 保留 presentation/result card；
- Lina/Luna 未识别的新字段只能 additive；
- branch 继续返回新 Conversation ID Header。

当前 Kael 发送 dot 命名的原生 PanelDelivery。Lina 直接读取 `delivery.type` 和 `delivery.payload`，不再映射成 `message_delta`、`message_done`、`approval_required` 等 Legacy 事件，也不维护旧 DTO 字段别名。Luna 自身的 Compatibility Adapter 不适用于 Lina；Kael 不暴露旧 `/api/v1/chat-ai/` Runtime 路由。

## 8. Luna 与 Lina 的实际差异

### 8.1 Luna 当前使用的子集

Luna 当前 Platform Panel 使用：

- Conversation CRUD；
- Assistant 列表与选择；
- Message 历史；
- 文本 SSE；
- Conversation cancel；
- Approval 查询、确认和拒绝；
- result card/activity 恢复。

Luna 当前没有完整暴露图片、文件、branch、regenerate、后台消息、STT、stats 和 audit UI，并固定关闭 Web Search。

### 8.2 Lina 当前使用面

Lina 当前使用：

- multipart 图片/文件；
- regenerate 和 branch；
- 前台 Run、cancel 与 Approval；
- 脱敏 Conversation audit；
- Message Token、附件、result card 和版本字段。

Lina 只调用 `/kael/api/v1`，会话、消息、Artifact、Panel/前台 Run/SSE、cancel、Approval、branch、regenerate 和 audit 均使用 Kael 原生资源，并直接消费 PanelDelivery dot 事件。iframe/embed、background、服务端 STT、Web Search 的 UI/状态/请求、旧 DTO/SSE adapter 及 stats 面板均已删除；浏览器支持时仍可使用不经过 Kael 的原生 `SpeechRecognition`。Core 旧 ChatAI Runtime 已删除，既有遗留表不会自动进入 Core Journal，也没有内置兼容查询入口。

## 9. 迁移能力矩阵

| 能力 | 目标归属 | 只改 Luna/Kael 的可行性 | 结论 |
|---|---|---|---|
| 新 Conversation/Message/Run | Kael | 可行 | 已迁移 |
| 模型 Provider、普通 Agent Loop | Kael | 可行 | 已迁移 |
| Assistant/Profile | Kael policy + 权限事实 | 已满足当前前置条件 | 已迁移 |
| SSE、历史、取消、恢复 | Kael | 可行 | 已迁移 |
| 图片、文件、提取、Artifact | Kael | 可行 | 已迁移 |
| branch、regenerate | Kael | 可行 | 已迁移 |
| result cards | Kael + Panel renderer | 可行 | 已迁移 |
| Web Search | 无当前产品入口 | Lina 已删除 UI/请求，Kael feature=false | 不迁移 |
| 服务端 STT | 无当前产品入口 | Lina 已删除 UI/请求，Kael仅保留 unavailable 占位 | 不迁移；保留浏览器语音识别 |
| 前台固定 Platform semantic tools | Luna Adapter | 在线时可行 | 可作为过渡 |
| 动态 Core OpenAPI | Platform Gateway | 已采用隔离 Headless Gateway | 已实现并完成开发链路验证 |
| 前台 Core 调用 | Headless Platform Gateway | 已实现 | 已完成开发链路验证 |
| 后台 Core 工具 Run | Headless Platform Gateway + 未来分布式协调 | Core Journal 已持久化历史，但缺少分布式 ownership、状态同步和安全工具恢复 | 未启用 |
| Approval 后执行 | Headless Platform Gateway | 当前只在同一 Kael 进程内绑定原调用 | 前台已实现，不跨重启 |
| 配额、并发、清理 | Kael | 可行 | 当前 Runtime 负责 |
| 站内信 | 无当前产品入口 | Lina 已删除旧通知请求 | 不迁移 |
| 管理员会话审计 | Kael | Lina 使用 Kael 原生 audit | 已迁移 |
| 旧开发分支 AI 表/数据 | 开发环境清理 | 不导入、不双写、不提供只读入口 | 清理旧 AI 表或重建开发库 |
| Scheduled Report | 无 | 当前代码不存在 | 不迁移 |

## 10. 当前架构边界

本期已经采用唯一实现，不再保留草案中的多种过渡方案：

- Kael 负责统一 Runtime；Lina 直接使用 Kael 原生 DTO 与 PanelDelivery，Luna 通过当前 Panel Adapter 接入同一 Runtime；
- 隔离的 Headless Platform Gateway 负责动态 Core OpenAPI、前台 Core Tool、进程内 Approval 和结果审计，Runtime core 不直接包含 Core 业务实现；
- Core 只保留组件签名的 Runtime Store 以及身份、配置和委托校验接口，不再运行 ChatAI Runtime/API/models/worker；
- Core-backed Journal 只保存 Kael 新产生的历史，旧 Platform ORM 数据和 Koko Agent Session JSONL 均不读取、不导入、不双写；
- 后台 Run、跨重启继续活动 ToolCall/Approval 和多实例 writer 仍明确禁用，不通过旧 Core 或旧 agentd 补齐。

Koko、Chen 的 Terminal、File、SQL、Script 会话与 MCP 能力继续作为 Luna Session Capability Provider。Koko 非 AI 路由不在本次迁移范围；旧 agentd 即使暂时仍存在于 Koko 仓库，也不是 Luna/Lina 的客户端回退目标，其物理清理属于 Koko 仓库的独立工作。

## 11. 开发环境数据处理

该功能尚未上线，因此不存在需要兼容的生产 ChatAI 历史，也不执行旧开发数据迁移。Kael 只从空的 Core Runtime Journal 开始写入新数据。

使用过旧开发分支的环境必须在联调前二选一：

- 清理旧 ChatAI 业务表和仅归属于旧 Runtime 的开发数据；
- 直接重建开发数据库。

本期不提供旧数据导入、只读入口、read-through、双写或旧客户端查询 API。环境回退也不得恢复已删除的 Core ChatAI Runtime/API/models/worker，或把 Koko agentd 当作客户端替代路径。清理后应验证：旧业务路由不可用，`/api/v1/chat-ai/` 下只有 Kael 组件可访问的 `runtime-store/`，Lina/Luna 的 AI 请求只进入 `/kael/api/v1`。

## 12. 最小测试主题

只保留少量跨层高价值测试，不复制现有 Platform AI 测试：

| ID | 主题 | 必须覆盖 |
|---|---|---|
| P1 | Lina 原生 API | CRUD、分页、附件、Message -> Run、branch/regenerate、进程内 Approval、audit，以及无 background/Web Search/服务端 STT/stats UI 请求 |
| P2 | PanelDelivery | dot 事件、心跳、错误、cursor 与未知字段前向兼容；不得恢复 Legacy SSE 映射 |
| P3 | Assistant policy | general 使用当前源码默认固定 allowlist；asset/audit/ops 更窄范围；所有 Operation 静态权限全量匹配且 dynamic fail closed；management 写操作必须 Approval |
| P4 | Artifact | 图片/文件限制、所有权、敏感信息、branch/regenerate 引用 |
| P5 | Platform Tool 安全 | 禁止任意 URL/Method、schema/policy、请求绑定、Core 二次 RBAC |
| P6 | Store/Recovery | Core Journal 分页/CAS、JSONL 回退恢复、进程内 queue/cancel/Panel cursor、重启安全收敛、Approval 进程绑定和非重复执行 |
| P7 | 开发环境边界 | 旧 AI 表已清理或开发库已重建；无旧数据导入、只读入口、双写、旧客户端 API 或 agentd 回退流量 |

每个主题使用一个表驱动或端到端场景覆盖多个状态，禁止按每个字段、路由或 SSE 事件复制独立测试文件。
