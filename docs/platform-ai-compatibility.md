# Platform AI 现状契约与迁移边界

## 1. 目的

本文记录当前 Platform AI 的真实代码能力、Luna/Lina 使用差异、迁入 Kael 时必须保留的契约，以及当前“只修改 Luna 和 Kael”与目标架构之间的硬约束。

本文与 [Kael AI Runtime Architecture](./ARCHITECTURE.md) 及 [AI Native Agent Runtime 迁移与演进方案](./ai-native-agent-runtime-migration.md) 共同构成迁移基线。

当前实现以 [ADR 0002](./adr/0002-core-component-and-store-port.md) 和 [ADR 0004](./adr/0004-jsonl-store-and-event-protocol.md) 为准：Kael 不连接数据库，使用本地 JSONL Store 保留由 Kael 创建的 Conversation 历史和 DomainEvent；前台 Platform capability 可用，后台 Run、活动执行跨重启续跑及持久 Approval 均未启用。

## 2. 真值来源

现状判断按以下优先级：

1. `jumpserver/apps/chat_ai` 当前代码；
2. 当前 Luna/Lina 客户端代码；
3. 当前自动化测试和可录制协议；
4. `jumpserver/docs/chat_ai_backend.md` 与 SSE Schema；
5. 产品设计稿。

现有 `chat_ai_backend.md` 和 SSE Schema 已与代码发生漂移，不能直接作为迁移黄金契约：

- 文档把 `general` 描述成全 Core 能力，当前代码中 `general` 是 chat-only；管理员全 Core 能力属于 `management`。
- 文档中的 Scheduled Report API、模型和任务在当前代码中不存在，不属于现有能力。
- 文档遗漏 PATCH、图片/文件下载、branch、multipart、部分后台参数和完整 Message 字段。
- SSE Schema 未覆盖实际的 `conversation_id` 和 `management`，不能作为唯一协议真值。

迁移前应从实际 Handler、Serializer、Runner 和客户端生成固定契约 fixture。

## 3. 与两类对话的映射

| 产品类型 | Platform AI 关系 |
|---|---|
| 普通对话 | 承接 Platform AI；不绑定当前 Luna Terminal/File/SQL/Script 资源 |
| Luna 能力对话 | 不继承 Platform 能力；只使用本次 PanelSession 明确注册的 Luna Session Capability |

普通对话允许不同 Assistant/Profile：

| Assistant | 当前代码语义 |
|---|---|
| `general` | JumpServer 通用问答；不访问实时 Core 数据 |
| `management` | 管理员可用；在当前权限和安全策略内访问允许的 Core 操作；写操作需 Approval |
| `asset` | 资产、节点、平台、协议等受限只读操作 |
| `session_audit` | 会话、命令、登录、访问、操作和工单审计只读操作 |
| `ops` | 作业、任务、组件指标和终端健康只读操作 |

Assistant 是可信策略与能力组合，不是 Conversation 类型，也不能替代 Registration。

## 4. 当前 HTTP 契约

旧逻辑前缀为 `/api/v1/chat-ai/`。它只用于记录现状契约，不成为 Kael 的新入口。Kael 的唯一权威业务根路径是 `/kael/api/v1`；Luna 在本次迁移中直接切换到新路径，未修改的 Lina 暂时继续由旧 Platform AI 服务承接。

迁移兼容的是业务语义和可观测协议，不是永久复制旧 URL。旧接口允许尾斜杠；Kael canonical 路径不带尾斜杠，并可在迁移期直接兼容两种形式，但不得依赖 POST、上传或 SSE Redirect。

### 4.1 Conversation 与 Message

| 方法 | 路径 | 当前行为 |
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

| 方法 | 路径 | 当前行为 |
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

### 4.3 Kael 目标资源映射

旧 URL 不做逐路径复制。Luna Compatibility Adapter 按下表把当前交互映射到 Kael 权威资源；Lina 在未来迁移时复用同一映射。

| 当前语义 | Kael canonical 路径 |
|---|---|
| Conversation 列表、创建、详情、修改、删除 | `/kael/api/v1/conversations`、`/kael/api/v1/conversations/{id}` |
| Message、Run 与待恢复状态 | `/kael/api/v1/conversations/{id}/messages`、`/kael/api/v1/conversations/{id}/runs`、`/kael/api/v1/conversations/{id}/approvals` |
| 图片和文件上传、鉴权读取、删除 | `/kael/api/v1/artifacts`、`/kael/api/v1/artifacts/{id}/content`、`/kael/api/v1/artifacts/{id}` |
| 前台 stream | 依次创建 Message、Run，再订阅 `/kael/api/v1/panel-sessions/{id}/events` |
| background | 当前 Kael 返回 `background_requires_durable_store`；如需该能力继续由旧服务承接，直至外部 Store adapter 完成并单独启用 |
| regenerate | `/kael/api/v1/messages/{id}/regenerations` |
| branch | `/kael/api/v1/conversations/{id}/branches`，请求体携带源 Message ID |
| cancel 与恢复 | `/kael/api/v1/runs/{id}/cancel`、`/kael/api/v1/runs/{id}/resume` |
| Assistant/Profile 发现 | `/kael/api/v1/assistants`、`/kael/api/v1/runtime-profiles` |
| Approval 查询与决定 | `/kael/api/v1/approvals/{id}`、`/kael/api/v1/approvals/{id}/decisions` |
| 动态 Platform Registry 刷新 | `/kael/api/v1/admin/platform-registry/refresh` |
| stats 与 audit | `/kael/api/v1/admin/stats`、`/kael/api/v1/admin/audit/conversations`、`/kael/api/v1/admin/audit/conversations/{id}` |
| STT | `/kael/api/v1/transcriptions` |

旧 `messages/stream/` 的“POST 同时创建消息、运行并保持响应流”被拆成可恢复的资源操作；断开 SSE 只取消订阅，取消 Run 必须显式调用 cancel。Adapter 可以维持首期 UI 行为，但不能在 Kael 中形成第二套 Run 生命周期。

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

迁移到 Kael 时需要保持：

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

Kael 目标模型应使用 Artifact 引用，避免把大对象放入 SSE Event。

### 5.3 前台与后台运行

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

Kael 完整替换 Platform AI 时，必须有等价后台 Worker、配额、取消、通知和僵尸 Run 回收语义。

### 5.4 Web Search

当前 Web Search：

- 默认关闭，由管理员功能开关与本次用户选择共同启用；
- 支持 Tavily 和 SearXNG；
- 使用独立 Base URL、凭据、代理、超时和结果上限；
- 不复用模型 API Key；
- 搜索结果视为不可信外部内容；
- 请求、来源、耗时和状态进入审计；
- 本轮执行过 Core API 后不再允许继续 Web Search；
- Luna 当前请求仍固定 `web_search=false`，Lina 已提供用户开关。

### 5.5 STT

当前 STT：

- 使用 OpenAI-compatible Audio Transcriptions；
- 支持常用音频格式和可选 ISO-639 language；
- 校验 Content-Length、实际大小和可选 ffprobe 时长；
- 有用户级速率、用户并发和全局并发限制；
- Provider 地址和凭据与模型服务隔离；
- 不写入业务存储。

迁到 Kael 时需同步部署 ffprobe 或显式关闭时长检查。

## 6. 当前 Agent 与 Platform Capability

### 6.1 动态 Core OpenAPI

当前 Platform Agent 会：

- 从 Core OpenAPI 构建带 hash 和 TTL 的 Registry；
- 按 Assistant、全局 allowlist、敏感路径和用户权限筛选 Operation；
- 先搜索候选 Operation，再让模型选择；
- 只允许模型提交 operation ID、path/query/body 参数；
- 由可信 Request Builder 决定 Method 和 URL；
- 校验 required、enum、array、nullable、组合 Schema、additional properties 和 query serialization；
- 将首次参数错误返回模型进行有限修正；
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

迁移后的 Kael Approval 必须达到同等或更强的绑定与一次性语义。

## 7. SSE 兼容基线

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

兼容要求：

- `message_delta` 只发送新增文本；
- 默认通过 SSE comment 发送心跳；
- `message_done` 能表达 completed、awaiting approval、cancelled；
- 错误只返回安全 code/detail，不返回堆栈；
- `api_call_result` 保留 presentation/result card；
- Lina/Luna 未识别的新字段只能 additive；
- branch 继续返回新 Conversation ID Header。

目标统一 Event Envelope 可以与 Legacy 事件不同，但 Luna Compatibility Adapter 必须为现有 Platform Panel 无损投影。这里的兼容不要求 Kael 暴露 `/api/v1/chat-ai/`。

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

### 8.2 Lina 当前使用的完整面

Lina 已使用：

- multipart 图片/文件；
- Web Search 开关；
- regenerate 和 branch；
- 后台执行；
- STT；
- stats 与审计；
- Message Token、附件、result card 和版本字段。

本期不修改 Lina，因此生产网关必须继续把 Lina 的 `/api/v1/chat-ai/` 流量留在旧 Platform AI 服务。Kael 不暴露该旧顶级路由。未来修改 Lina 时，应直接迁移到 `/kael/api/v1`，不能再新增另一套兼容状态。即使如此，也不能只按 Luna 的较小子集判断 Platform AI 已完整迁移。

## 9. 迁移能力矩阵

| 能力 | 目标归属 | 只改 Luna/Kael 的可行性 | 结论 |
|---|---|---|---|
| 新 Conversation/Message/Run | Kael | 可行 | 必须迁移 |
| 模型 Provider、普通 Agent Loop | Kael | 可行 | 必须迁移 |
| Assistant/Profile | Kael policy + 权限事实 | 有前置条件 | 必须迁移 |
| SSE、历史、取消、恢复 | Kael | 可行 | 必须迁移 |
| 图片、文件、提取、Artifact | Kael | 可行 | 完整替换时必须迁移 |
| branch、regenerate | Kael | 可行 | 完整替换时必须迁移 |
| result cards | Kael + Panel renderer | 可行 | 必须迁移 |
| Web Search | Kael | 可行 | 完整替换时必须迁移 |
| STT | Kael | 可行，需部署依赖 | 完整替换时必须迁移 |
| 前台固定 Platform semantic tools | Luna Adapter | 在线时可行 | 可作为过渡 |
| 动态全 Core OpenAPI | Platform Gateway | 已采用隔离 Headless Gateway | 已实现，待部署启用 |
| 前台 Core 调用 | Luna Adapter 或 Platform Gateway | 在线子集可行 | 需明确方案 |
| 后台 Core 工具 Run | 未来 Headless Platform Gateway + 外部 Store | 当前缺少 durable Store | 未启用 |
| Approval 后执行 | Headless Platform Gateway | 当前只在同一 Kael 进程内绑定原调用 | 前台已实现，不跨重启 |
| 配额、并发、清理 | Kael | 可行 | 必须迁移 |
| 站内信 | Core 正式接口 | 当前依赖 Core 内部函数 | 当前阻塞 |
| 管理员审计查看日志 | Core 正式接口 | 当前依赖 Core 内部函数 | 当前阻塞 |
| 旧历史、附件、Approval、Audit | 旧服务只读 | ADR 冻结为不双写的只读保留 | 已决策 |
| Scheduled Report | 无 | 当前代码不存在 | 不迁移 |

## 10. 必须正视的架构冲突

以下三项不能同时无条件成立：

1. Kael/agentd 不连接 Core 业务 API；
2. 只修改 Luna 和 Kael；
3. 当前 Platform AI 所有前台、后台、Approval、历史和审计能力一次性完整迁移。

原因：

- 浏览器关闭后，Luna Capability Adapter 不再在线；
- 后台 Run 无法通过 Luna 继续调用 Core；
- 持久 Approval 无可信服务端 Executor 时无法在稍后确认并执行；
- `management` 依赖动态 Core OpenAPI、请求构造、policy 和 sanitizer；
- 历史与附件仍在 Core 数据库和媒体存储；
- 站内信和审计查看日志依赖 Core 内部能力。

本期已在 [ADR 0001](./adr/0001-headless-platform-gateway.md) 中选择隔离的 Headless Platform Gateway，并由 ADR 0002/0004 将其当前范围收敛为前台调用、持久历史和进程绑定的执行能力；旧 Platform 数据选择旧服务只读保留，Koko Agent Session JSONL 不导入 Kael。生产启用条件见 [迁移实施状态](./MIGRATION_STATUS.md)。

### 10.1 严格目标架构

- 本期 Kael 完成普通聊天和 Luna Session Capability；
- 旧 Platform 服务暂时保留动态 Core、后台、历史和审计能力；
- 后续允许修改 Lina/Core，建立正式 Platform Capability Gateway；
- 达到等价后再关闭旧服务。

优点是符合目标边界；缺点是 Platform AI 不是一次性全部迁走。

### 10.2 过渡 Headless Platform Gateway

- 在 Kael 仓库建立与通用 Runtime 隔离的独立 Gateway 进程或模块；
- Runtime 仍只调用通用 Capability Broker；
- Gateway 复用 Core 已支持的短期 HMAC 委托和 mTLS；
- Gateway 当前承担前台动态 OpenAPI、Core Tool、进程内 Approval 和结果审计；外部 Store 完成后才可评估后台 Core Tool 与持久 Approval；
- 未来由 Lina/Core 正式 Gateway 替换。

优点是在只改 Kael/Luna 时更接近完整迁移；缺点是 Kael 部署整体仍存在到 Core 的业务连接，是目标架构的明确过渡例外。

### 10.3 Luna 前台 Adapter

- Platform semantic tool 由当前 Luna Panel 执行；
- 只支持浏览器/Electron 在线期间的固定能力；
- 后台 Core Tool、关闭页面后的 Approval 和动态全 Core 管理能力继续留在旧服务或明确延期。

优点是 Runtime 边界最简单；缺点是单独使用时不能称为完整 Platform AI 迁移。

## 11. Legacy 数据与切流

如果迁移旧数据，至少保留：

- 原 UUID；
- user/org；
- assistant、model、status；
- Message 顺序、状态、Token、错误；
- 图片、文件和提取文本；
- result cards；
- regenerate/branch 关系；
- Run、Approval、ApiCallAudit；
- 原保留期和删除语义。

切流前必须：

- 停止创建新的旧后端 Run；
- drain、取消或明确保留 queued/running/awaiting approval Run；
- 防止同一 Run 在 Core 和 Kael 双重推进；
- 保证 Lina 旧客户端仍可访问其需要的协议；
- 为旧数据制定迁移、只读 read-through 或明确不迁移策略；
- 对每种数据状态提供回滚规则。

若不迁移旧数据，应在产品和发布说明中明确旧历史的只读入口与保留期限，不能静默丢失。

## 12. 最小测试主题

只保留少量跨层高价值测试，不复制现有 Platform AI 测试：

| ID | 主题 | 必须覆盖 |
|---|---|---|
| P1 | Legacy API fixture | CRUD、分页、附件、stream、branch、regenerate、进程内 Approval、stats/audit 的协议投影，以及 background/STT 的明确禁用 |
| P2 | Legacy SSE fixture | 全事件、心跳、错误、Header 与未知字段前向兼容 |
| P3 | Assistant policy | general 无 Core；asset/audit/ops 范围；management 写操作必须 Approval |
| P4 | Artifact | 图片/文件限制、所有权、敏感信息、branch/regenerate 引用 |
| P5 | Platform Tool 安全 | 禁止任意 URL/Method、schema/policy、请求绑定、Core 二次 RBAC |
| P6 | Store/Recovery | JSONL 事务恢复、进程内 queue/cancel/Panel cursor、重启安全收敛、Approval 进程绑定和非重复执行 |
| P7 | 数据切流 | UUID、消息顺序、附件、result card、Approval、Audit 和 rollback |

每个主题使用一个表驱动或端到端场景覆盖多个状态，禁止按每个字段、路由或 SSE 事件复制独立测试文件。
