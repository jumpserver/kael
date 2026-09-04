# Kael AI Native Agent Runtime 迁移与演进方案

## 1. 文档状态

| 项目 | 内容 |
|---|---|
| 状态 | 设计基线 |
| 日期 | 2026-09-04 |
| 目标仓库 | Kael、Luna、Lina、JumpServer Core（仅保留 ChatAI Runtime Store） |
| 参考 | 《JumpServer AI Native Agent Runtime 完整方案》与 Koko/Luna/Lina/Kael 当前实现 |
| 本期代码范围 | Kael、Luna、Lina，以及 JumpServer Core 的旧 Runtime 删除与组件持久化接口 |
| 实施状态 | ADR 0002/0004/0006 范围内的代码级逻辑、Lina 原生协议切换和 Core-backed Runtime Journal 已完成；生产切流 Gate 见 `MIGRATION_STATUS.md` |

本文是后续 AI 改造的实现、评审和验收基线。若代码与本文冲突，应先明确并记录新的架构决策，再修改本文和代码。

[ADR 0002](./adr/0002-core-component-and-store-port.md)、[ADR 0004](./adr/0004-jsonl-store-and-event-protocol.md) 与 [ADR 0006](./adr/0006-core-backed-runtime-journal.md) 已冻结当前运行形态：Kael 与 Koko 一样注册为 Core Terminal component，模型配置来自 TerminalConfig，只有 `CHAT_AI_ENABLED` 控制启停；`CHAT_AI_METHOD` 和 `CHAT_AI_EMBED_URL` 已删除。Kael 不连接数据库，默认通过组件签名 API 把 Runtime snapshot/delta Journal 保存到 Core，`RUNTIME_STORE=jsonl` 仅为显式回退。Core 旧 ChatAI Runtime/API/models/worker 已删除，`/api/v1/chat-ai/` 下只保留 `runtime-store/`。Kael 不读取或导入 Koko `data/agent/events/*.jsonl`，也不导入旧 Platform 数据。功能尚未上线；使用过旧开发分支的环境应清理旧 AI 表或重建开发库。后台 Run、多实例共享、活动 Panel 能力和未决 Approval 的跨重启续接仍禁用。

长期稳定的组件边界、所有权和协议不变量见 [Kael AI Runtime Architecture](./ARCHITECTURE.md)。

## 2. 已确认的产品决策

### 2.1 Platform AI 纳入迁移

现有 Platform AI 与 Koko agentd 能力都迁入 Kael，不能只迁移 Terminal、File、SQL、Script 四个工作区场景。

Luna 首期对用户呈现两类对话：

| 对话类型 | 定义 |
|---|---|
| 普通对话 | 不绑定当前 Luna 资源会话的长期对话 |
| Luna 能力对话 | 绑定当前 Luna Panel、动态上下文和本地能力的对话 |

“普通对话”不等于“永远没有工具”。它可以是纯模型对话，也可以按 assistant/preset 使用经过授权的平台语义能力；它只是不能隐式获得当前 Terminal、File、SQL 或 Script 会话的能力。

### 2.2 两类对话共用一个 Runtime

Kael 不建设两套 Conversation、两套 Run 状态机或两套模型调用链。两类对话共用：

- Conversation、Message、Run 和 Event；
- 模型 Provider 与路由；
- 流式输出、取消、幂等和恢复；
- Approval、审计和可观测性；
- PanelSession 与版本化协议。

两类对话通过产品类型、可信 Profile、本次 Run 的能力模式以及有效 Registration 区分，而不是通过不同 Runtime 分叉。

### 2.3 当前改造范围

本期允许：

- 在 Kael 中建立新的通用 Agent Runtime；
- 将 Koko agentd 的通用 AI 能力迁入 Kael；
- 将 Platform AI 的对话能力迁入 Kael；
- 修改 Luna，使普通对话和 Luna 能力对话都连接 Kael；
- 修改 Lina，使会话、消息、Artifact、前台 Run/SSE、审批和 audit 直接使用 Kael 原生 DTO 与 PanelDelivery dot 事件，并删除 iframe/embed、旧 DTO/SSE adapter、background/Web Search/服务端 STT 请求和旧 stats 面板；
- 删除 Core 旧 ChatAI Runtime/API/models/worker，只保留供 Kael 组件签名访问的 Runtime Store Journal API；
- 在 Luna 中建立 Platform/Luna Capability Adapter；
- 保持 Luna 到 Koko、Chen 和本地工具的既有执行链。

本期不允许：

- 修改或删除 Koko 中现有 agentd 代码；
- 修改 Koko、Chen 的 MCP/会话工具协议；
- 修改 Magnus 或 Lion，或让 Core 执行 Kael Runtime；
- 让 Kael 直接执行终端、文件、数据库或页面操作；
- 让 Kael 持有资源连接凭据。

因此本期是“在 Kael 重建、由 Core 持久化 Runtime Journal 并由前端单向切换”的逻辑迁移。Koko 中旧 agentd 的物理删除或启动开关调整属于后续独立变更；旧 agentd 不作为 Luna/Lina 客户端的回退服务。Koko 的 Terminal、MCP、WebSocket 等非 AI 路由不在本期范围。

## 3. 架构原则

目标架构为：

```text
AI Panel Host + Headless Agent Runtime + Local Capability Gateway
```

调用关系：

```text
User
  |
  +--> Luna AI Panel -- HTTP + SSE ---------------------+
  |                                                     |
  +--> Lina AI Panel -- HTTP + native PanelDelivery ----+
                                                        |
                                                        v
                                             Kael Agent Runtime
                                                        |
                 +--------------------------------------+------------------+
                 |                                                         |
                 +-- panel binding --> 原 Luna PanelSession                +-- service binding --> Headless Platform Gateway --> Core API
                                         +-- Luna MCP Adapter --> Koko / Chen                                        （当前用户权限）
                                         +-- Luna Local Adapter --> Script / UI
                                         +------------- tool.result --> Kael
```

必须始终满足：

1. Kael 是决策与编排 Runtime，不是业务能力执行器。
2. Luna 是当前环境的 Agent Host、Context Provider 和 Capability Gateway。
3. Koko、Chen 和 Luna UI 是 panel binding 后面的能力来源；Core API 位于隔离的 service provider 后面，对 Runtime core 保持不可见。
4. MCP 只用于 Luna 与 Session Component 之间，不是 Kael 的 Runtime 协议。
5. AI 不获得当前用户之外的任何权限。
6. Registration 只描述能力；真正执行时仍由 Core、Koko 或 Chen 复验权限。
7. ToolCall 必须回到发起 Run 的准确 PanelSession，禁止广播或跨 Tab 自动转移。
8. Conversation 历史可从 Core Runtime Journal 跨 Kael 节点恢复；PanelSession 与 executor 不可跨进程恢复，正在执行的 Run 不能在环境之间漂移或自动重放。

## 4. 组件职责

### 4.1 Kael

Kael 负责：

- 通过 Store port 管理 Conversation、Message、Run、ToolCall、Approval 和 Event 的权威状态；
- 模型 Provider、模型选择、上下文构建和 Agent Loop；
- 前台 Run 排队、执行、取消和超时；
- Tool Definition 校验、模型工具名映射和参数修复；
- Registration、租约、能力快照和 ToolCall 编排；
- 通用风险策略和 Approval 状态；
- HTTP API、SSE、PanelSession 事件重放和持久历史查询；
- 幂等、事务、审计链和可观测性；
- 服务端可信 Profile、Prompt Policy，以及通过组件身份读取的 TerminalConfig 模型配置。

Kael 不负责：

- 直接调用 Koko、Chen、Core 业务 API、Magnus 或 Lion；
- 理解 MCP 帧或具体 Session Component 协议；
- 保存 SSH、数据库、SFTP 或 ConnectToken 凭据；
- 根据工具名字猜测具体业务执行方式；
- 相信客户端提交的 user、org、risk 或 system prompt；
- 在本地执行任何由 Panel 注册的 Tool。

### 4.2 Luna

Luna 负责：

- 普通对话和 Luna 能力对话的 UI；
- 创建、恢复和关闭 PanelSession；
- 提供最小化、结构化、动态版本化 Context；
- 建立本地 Capability Registry；
- 将 Platform、Koko MCP、Chen MCP、本地 Script/UI 能力标准化后注册给 Kael；
- 按 Registration ID 精确执行 Kael ToolCall；
- 将 MCP 或本地执行结果转换为通用 ToolResult；
- 显示 Approval、能力状态、执行过程和结构化结果；
- 将 cancel 传播到真实执行器；
- 同时支持浏览器和 Electron 的既有身份通道。

Luna 不是 Conversation 或 Run 的权威存储，也不直接调用模型。

### 4.3 Koko 与 Chen

Koko 和 Chen 保持现有 Session Capability Provider 身份，继续负责：

- 当前会话上的真实工具执行；
- 连接凭据和资源连接生命周期；
- ACL、RBAC、命令复核与权限撤销复验；
- SFTP 路径约束、版本前置条件和文件安全；
- 数据库连接、字段脱敏和语句约束；
- 命令、文件和数据库操作的既有审计；
- MCP manifest、invoke、cancel 和结果协议。

这些行为不得迁入 Kael。

### 4.4 Platform Capability 边界

当前 Platform Capability 使用与 Runtime core 隔离的 Headless Gateway，Lina 已原生切换到 Kael，Capability Provider 边界如下：

- Kael 仓库内的隔离 Headless Gateway 提供 Platform Capability；
- Gateway 使用请求绑定的一次性 HMAC 委托传递当前 user/org，Core 执行最终 RBAC；
- 动态 OpenAPI 只通过受控 operation registry 暴露，并按 Profile、HTTP method、schema 和敏感路径限制；
- Kael Runtime core 只识别通用 Registration、ExecutionBinding 和 ToolResult；
- 后续如替换同名或新版语义能力的 Provider，不改变 Kael Runtime 协议。

Headless Gateway 已由 [ADR 0001](./adr/0001-headless-platform-gateway.md) 选定并实现。Core 旧 ChatAI Runtime/API/models/worker 已删除，只保留 Runtime Store 和所需的组件身份、配置、委托校验边界。Lina 只调用 `/kael/api/v1` 并直接消费 PanelDelivery dot 事件；iframe/embed、Legacy DTO/SSE adapter、background、Web Search、服务端 STT UI/请求和旧 stats 面板均已删除。浏览器支持时仍可使用不经过 Kael 的原生 `SpeechRecognition`。

## 5. 当前能力基线

迁移行为基线来自删除前冻结的 Koko agentd/Platform AI fixture 与当前 Luna/Lina/Kael 协议，不以已漂移的旧文档为准。旧服务只提供历史语义参考，不是当前可调用或可回退实现。

### 5.1 Koko agentd Runtime

Kael 已迁移并保留的核心语义：

- 串行 Run 队列、队列上限、Run 超时和关闭语义；
- 每 Run 的轮次与模型请求预算；
- 最终轮禁用工具并尽力生成部分回答；
- 输入和输出 JSON Schema 编译及校验；
- Provider 不支持原工具名时的安全别名与冲突检查；
- 非标准 Tool Arguments 的解析和一次模型修复；
- 写操作防重复，读操作最多一次安全刷新；
- 用户拒绝、Tool 失败、final result 和错误闭环；
- final result 返回用户决定后禁用工具并继续生成完整的结果说明，不能用固定占位文案提前结束；
- client message ID、idempotency key 和 ToolResult sequence；
- Run、Approval、ToolResult 的重复提交一致性；
- cancel 向未完成 ToolCall 传播；
- 事件游标、历史分页和重启恢复；
- token、时延和模型请求事件；
- Payload、History、Tool 数量、结果大小等边界限制。

当前默认行为基线包括：

| 项目 | 当前基线 |
|---|---|
| 最大 Agent 轮数 | 20 |
| 最大模型请求数 | 40 |
| 默认 Run 超时 | 30 分钟 |
| 最大排队 Run | 64 |
| 最大工具数 | 64 |
| 最大 Context | 4 MiB |

后续如需调整，应作为显式配置和兼容性变更记录，不能在迁移中无声改变。

### 5.2 模型 Provider

Kael 保留自身的 Model Provider port 作为 Runtime 测试替换点，但 Provider Adapter 必须直接使用官方 `github.com/openai/openai-go`，不得自行实现 OpenAI 请求结构、SSE 解码、工具调用结构或 API 错误协议。OpenAI 使用 SDK Responses API，OpenAI-compatible 和 DeepSeek 使用 SDK Chat Completions。

必须保留：

- OpenAI Responses；
- OpenAI Chat Completions 回退；
- OpenAI-compatible；
- DeepSeek thinking、结构化 action 和非 reasoning 回退；
- native tools、structured output 和纯文本模式；
- reasoning effort；
- store、previous response、local state 和 compaction；
- 模型窗口与输出 token 上限归一化；
- 显式代理、TLS 1.2、请求超时和错误分类；
- 请求预算、token 与 latency 统计；
- 日志和 tracing 中的敏感信息保护。

迁移验收不能只检查“模型能返回一段文本”。

### 5.3 Luna 能力域

首期 Luna 能力对话必须保持四个现有域：

| 域 | 能力与约束 |
|---|---|
| Terminal | SSH/Kubernetes/数据库终端上下文、PTY/受限后台执行、审批、取消、ACL 和审计 |
| File | 目录浏览、读取、保存、创建目录、重命名、删除、路径约束和版本前置条件 |
| SQL | read context、schema inspection、validate、proposal；应用只修改编辑器，不直接执行 SQL |
| Script | read/propose；draft-only、revision 校验；应用不保存、不执行 |

Koko/Chen 的执行协议和 Luna 当前安全检查保持不变。

### 5.4 Platform AI

普通对话当前承接的 Platform AI 产品能力：

- Conversation 列表、新建、详情、改名、Assistant 切换和删除；
- 分页消息历史、图片和文件附件；
- assistant/preset 与 starter prompts；
- 前台流式回答、取消和运行恢复；
- regenerate 和 branch；
- pending Approval 恢复；
- activity/result card 展示；
- API search、API call 和安全策略；
- 用户/全局限流和部署依赖；
- 服务端管理员 stats API 和脱敏 Conversation audit；
- Run/Token/API 审计、数据保留和僵尸状态清理；
- `general`、`management`、`asset`、`session_audit`、`ops` preset。

这些 preset 是模型策略和能力组合，不是新的 Conversation 类型。

Kael 的默认 `general` 恢复旧统一 JumpServer Assistant 语义：产品问答与迁移时旧源码默认 operation allowlist 等价的编译期固定范围内的授权 Core 搜索/调用。Kael 不读取旧 Chat AI operation IDs、allowed/blocked paths/tags 或 method policies 配置；功能尚未上线，旧开发分支的自定义配置不迁移，当前编译期策略是唯一生效配置。`management` 是管理员专用、范围更宽的静态权限能力；asset、session_audit、ops 继续使用更窄 scope。所有 operation 都必须具有可静态解析的 OpenAPI 权限元数据，创建 Run 的 Principal 必须拥有全部 required permissions；Core 执行 delegated request 时再按实时权限复核。Scheduled Report 当前没有代码实现，不纳入现状基线。

Luna 现有 Platform Panel 只使用上述能力的一个子集。旧 Lina 曾使用附件、联网、branch、regenerate、后台、STT、stats 和 audit；当前 Lina 只保留会话、消息、Artifact、前台 Run/SSE、cancel、Approval、branch、regenerate、result card 和 audit，并直接使用 `/kael/api/v1` 原生资源与 PanelDelivery dot 事件。background、服务端 STT、Web Search 的 UI/状态/请求及旧 stats 面板已经删除，不再以“请求后返回 unsupported”维持兼容；浏览器原生 `SpeechRecognition` 保留且不属于 Kael 服务端 STT。

## 6. 产品模型

### 6.1 普通对话

普通对话具有以下特征：

- 长生命周期，可在没有资源会话时创建、查看和继续；
- 不隐式绑定当前 Terminal、File、SQL 或 Script；
- 支持 assistant/preset；
- `general` 是 Lina 默认的旧统一 JumpServer Assistant，使用受控 Platform service capability；Platform Gateway 默认且必须启用，关闭或缺少与 Core 匹配的 delegation secret 时 Kael 启动失败；
- 其他 preset 可绑定明确授权的 Platform Capability；
- 当前 Luna 恰好打开某个 SSH Tab，不会使普通对话自动获得 SSH 工具；
- 页面焦点变化不能改变既有 Conversation 的类型。

### 6.2 Luna 能力对话

Luna 能力对话具有以下特征：

- Conversation 与当前资源连接分离；
- 每次执行绑定具体 PanelSession；
- Terminal、File、SQL、Script 只是 capability profile，不是四种 Conversation；
- 动态 Context 和 Registration 只在对应 PanelSession 内有效；
- 资源离线时仍可查看历史，但不能假装能力可用；
- Panel 断开时不能把 ToolCall 转给同一用户的另一个 Tab；
- 降级为普通模型回答必须由用户明确选择，不能静默发生。

### 6.3 独立维度

以下概念必须独立：

| 维度 | 示例 | 用途 |
|---|---|---|
| Conversation Kind | general、capability | 产品分类与历史展示 |
| Assistant/Preset | general、management、asset、session_audit、ops | 模型策略和能力组合 |
| Surface | general.chat、session.terminal、session.file | 当前交互环境与审计 |
| Profile | general、platform.management、terminal、file、sql、script | 服务端可信运行策略 |
| Capability Set | platform.read、platform.manage、luna.terminal 等 | 本次可见能力范围 |
| Registration | 某个 Panel 注册的一项具体能力 | 定义与精确路由 |

Conversation Kind、Assistant、Surface 或 Context 都不能单独授予工具权限。

## 7. 统一领域模型

### 7.1 Principal

Principal 表示经过可信入口确认的用户和组织身份。至少包含：

- subject ID；
- organization ID；
- authentication source；
- session/token fingerprint；
- 可选客户端和设备信息。

客户端提交的 user/org 只能作为请求提示，不能成为权威身份。

### 7.2 Conversation

Conversation 是长期对话容器，至少保存：

- ID；
- Principal 和组织范围；
- Kind；
- 默认 Assistant/Profile；
- 来源 Surface 的非授权元数据；
- title、status；
- created/updated/archived 时间；
- 产品元数据和版本。

Conversation 不保存活动 WebSocket、MCP executor 或当前 Registration。

### 7.3 Message

Message 保存：

- ID、Conversation ID；
- role；
- 结构化 parts；
- 文本内容；
- artifact/result 引用；
- idempotency key；
- created time。

消息格式应允许未来增加文件、图片、结构化结果和 UI action，而无需把所有内容塞入纯文本。

### 7.4 PanelSession

PanelSession 代表一次具体浏览器/Electron AI Panel 实例，至少保存：

- ID、Conversation ID、Principal；
- surface 与可信 profile；
- context version；
- registry revision；
- lease、last heartbeat；
- resume token hash；
- connection ownership 和状态。

普通对话也创建 PanelSession，以统一 SSE、重连、多 Tab 隔离和未来能力扩展；普通纯聊天的 Registration 可以为空。

### 7.5 ContextSnapshot

Context 是数据，不是指令，也不是权限。每个版本应包含：

- PanelSession ID；
- version 与 digest；
- domain/surface；
- 最小化语义数据；
- created time；
- 敏感级别和保留策略。

禁止上传完整 Vue store、密码、token、证书或无边界终端缓冲区。

### 7.6 Registration

Registration 属于一个 PanelSession，至少包含：

- server-generated Registration ID；
- PanelSession ID；
- Luna 本地稳定 client key；
- name、description；
- input/output schema；
- definition version 与 digest；
- risk、requires confirmation；
- read-only、destructive、open-world、idempotent 等注解；
- registry revision；
- lease、expires time 和状态。

Kael 只保存定义和路由信息，不保存浏览器 executor 句柄或资源凭据。

### 7.7 Run

每次用户请求对应一个 Run。Run 必须保存：

- ID、Conversation ID、输入 Message ID；
- 发起 Run 的 PanelSession ID；
- 实际 Profile 及版本；
- execution mode；
- capability mode；
- context version；
- registry revision；
- 可见 Registration 定义快照；
- 状态、取消原因和错误；
- 模型、token、时延和审计元数据。

建议状态：

- queued；
- running；
- waiting_capability；
- waiting_approval；
- cancelling；
- completed；
- failed；
- cancelled；
- interrupted。

普通纯聊天 Run 使用 disabled capability mode。需要 Luna 在线能力时使用 panel mode。Capability mode 必须显式保存，不能通过“当前工具列表是否为空”推断。

当前允许 foreground+disabled、foreground+panel 和 foreground+service；所有 background 组合返回 `background_requires_durable_store`。默认 Core-backed Journal 的 durable 只表示历史已持久提交，不代表具备后台 ownership 或安全续跑条件；未来补齐分布式 claim、状态同步与工具恢复后，background+disabled 和 background+service 可重新评估，background+panel 始终非法。service mode 已按 ADR 0001 绑定可信 Headless Provider，不能伪装成浏览器 Panel。

### 7.8 ToolCall 与 ToolResult

ToolCall 必须绑定：

- Conversation；
- Run；
- PanelSession；
- Registration；
- tool definition revision；
- arguments digest；
- risk 和 Approval；
- invocation sequence。

ToolResult 必须校验完整归属链，支持：

- running、success、error、cancelled、timeout；
- 递增 sequence；
- 重复 payload 幂等；
- 冲突 payload 拒绝；
- 有界结构化 result/error；
- executor 审计引用。

### 7.9 DomainEvent 与 PanelDelivery

领域状态变化生成全局唯一 DomainEvent；Event Projector 再为获准接收的 PanelSession 生成 PanelDelivery。二者不能混成同一对象：

- DomainEvent 包含 event ID、aggregate、type、timestamp、schema version 和有界 payload，并与状态同事务提交；
- PanelDelivery 包含 PanelSession ID、该 stream 内严格递增的 sequence、event ID、audience 和投影 payload；
- 同一 DomainEvent 可以投影到多个 Panel，各自拥有独立 sequence；
- DomainEvent 使用 Conversation 内递增 `seq` 写入 Runtime Journal；`RUNTIME_STORE=jsonl` 回退时另写 `data/events/<conversation-id>.jsonl`，PanelDelivery 仍有 PanelSession 内 sequence；
- PanelDelivery 在 SSE 发布前写入同一 Store 事务，并允许客户端忽略未知事件类型；旧 PanelSession 重启后失效，其 cursor 不能被新 Panel 继承。

## 8. Run 与能力规则

### 8.1 Run 快照

Run 开始时固定：

- Profile ID/version；
- Context version；
- Registry revision；
- 可见 Registration ID 和定义版本；
- model policy；
- approval policy。

Panel 后续切换 Tab、更新 Context 或刷新 manifest，只影响后续 Run。运行中不得静默切换环境。

### 8.2 工具可用条件

模型仅在全部条件成立时看到工具：

1. Run capability mode 允许 Panel capability；
2. Registration 属于 Run 的 PanelSession；
3. Registration 处于 active 状态；
4. Registration revision 在 Run 快照中；
5. Profile 允许该能力命名空间和风险级别；
6. Principal 和组织范围一致。

ToolCall 发出前还必须实时复验 Panel lease、Registration 状态、definition digest 和 Approval 状态。

### 8.3 不可用与断线

能力暂不可用时必须区分：

- capability_required；
- capability_expired；
- capability_revoked；
- capability_unavailable；
- provider_disconnected；
- invocation_timeout。

Luna 能力对话不能静默降级。Panel 重连后，应先通过 client key 与 definition digest 重新绑定 executor，再将 Registration 恢复为 active。

非幂等 ToolCall 不得因重连自动重放。必须通过原 invocation ID 查明结果，或者重新取得用户确认。

## 9. Capability Registry 与本地执行

### 9.1 原子注册

Luna 的 MCP manifest 是整组并带 revision 的，因此 Kael 除单项增删外，应支持整组原子替换。原子替换必须：

- 校验 base registry revision；
- 校验每项 schema 和注解；
- 为每项生成 Registration ID；
- 返回新的 registry revision、definition digest 和 lease；
- 全部成功后一次提交；
- 失败时保留上一完整 revision。

### 9.2 Luna 本地路由表

Luna 本地必须维护以下单向绑定：

```text
registration_id
  -> panel_session_id
  -> local client key
  -> resource_session_id / pane ID / revision
  -> executor adapter
```

禁止使用 user ID、组织、tool name 或“最近活跃 Tab”作为替代路由键。

### 9.3 MCP 适配

对于 Koko/Chen 能力：

1. Luna 接收 MCP manifest；
2. Luna 将工具定义与注解标准化为 Registration；
3. Kael 通过 SSE 发出通用 ToolCall；
4. Luna 查找准确本地 Registration；
5. Luna 构造既有 MCP `tools/call`；
6. Koko/Chen 执行并返回 MCP result；
7. Luna 将结果归一化为通用 ToolResult；
8. Kael 继续 Agent Loop。

Kael 不解析 MCP `content`、`structuredContent`、`isError` 或 `_meta`；这些转换属于 Luna Adapter。

### 9.4 Platform Capability

Platform Tool 必须是少量语义能力，例如资产搜索、审计查询或 UI 导航。每项能力都应明确：

- 用户意图；
- 输入/输出 schema；
- read/write/dangerous 风险；
- 是否需要确认；
- 使用的当前用户权限；
- 审计摘要；
- 失败与分页行为。

禁止生成“任意 API 请求”工具。

## 10. Context 设计

### 10.1 普通对话 Context

只包含必要信息，例如：

- locale、timezone；
- 当前产品入口；
- 当前 assistant/preset；
- 可选页面语义上下文；
- 不具授权意义的用户偏好。

### 10.2 Luna 能力 Context

按域提供最小化语义信息：

| 域 | Context 示例 |
|---|---|
| Terminal | 资产显示信息、协议、账号显示名、终端状态、有限输出快照 |
| File | 当前路径、选择项、连接状态、文件版本摘要 |
| SQL | dialect、database/schema、选区、当前 SQL、revision、最近错误 |
| Script | module、draft、revision、变量名称和是否有默认值 |

Context 变化应通过独立版本更新，不要求销毁 Conversation。

### 10.3 敏感信息

以下信息禁止进入 Kael Context、Conversation、模型请求和普通日志：

- 密码、私钥、数据库凭据；
- Cookie、Bearer、ConnectToken；
- 证书私密材料；
- 完整未经裁剪的终端/文件内容；
- Luna store 的无关状态；
- Koko/Chen executor 内部句柄。

## 11. API 与协议基线

### 11.1 公共路径决策

所有 Kael HTTP 与 SSE 路径统一使用 `/kael` 前缀。正式业务 API 的唯一根路径为：

```text
/kael/api/v1
```

不再提供按对话类型拆分的业务路径。普通对话和 Luna 能力对话全部使用 `/kael/api/v1`，共享资源模型，并通过 Conversation Kind、Profile、Run capability mode 和 Registration 表达差异。

这样可以保证：

- 同一 Conversation 可以在未来显式连接或断开能力；
- 普通对话和能力对话共用历史、Run、Event、Approval 和审计；
- 新增 Lina、Magnus 或 Lion 不需要再增加一套顶级路由；
- URL 不被误当成授权依据；
- API 版本只维护一套。

部署在 JumpServer site prefix 下时，浏览器实际请求为“site prefix + `/kael/api/v1`”。Luna 必须继续通过现有 `withWebSitePrefix` 生成 URL，不能假设部署在域名根目录，也不能用 Luna 自身的应用 base URL 拼 API：

- `/luna/...` 页面请求 `/kael/api/v1/...`；
- `/tenant-a/luna/...` 页面请求 `/tenant-a/kael/api/v1/...`；
- 禁止生成 `/luna/kael/...` 或 `/tenant-a/luna/kael/...`。

### 11.2 当前 API

| 方法 | 路由 | 作用 |
|---|---|---|
| GET | `/kael/api/v1/bootstrap` | 返回协议版本、feature、限制、CSRF 和稳定 cluster identity |
| GET | `/kael/api/v1/assistants` | 返回当前 Principal 可使用的 Assistant/Profile |
| GET | `/kael/api/v1/runtime-profiles` | Runtime Profile 与协议能力发现 |
| POST | `/kael/api/v1/conversations` | 创建 Conversation |
| GET | `/kael/api/v1/conversations?kind={general|capability}` | 按当前 Principal 和可选 Kind 分页查询 Conversation |
| GET | `/kael/api/v1/conversations/{id}` | 获取 Conversation |
| PATCH | `/kael/api/v1/conversations/{id}` | 改名、归档等元数据操作 |
| DELETE | `/kael/api/v1/conversations/{id}` | 软删除 Conversation；当前不会物理清除关联 Journal 数据，边界见 ADR 0006 |
| GET | `/kael/api/v1/conversations/{id}/messages` | 分页读取消息 |
| GET | `/kael/api/v1/conversations/{id}/runs` | 分页读取 Run，并发现 active、interrupted 等可恢复状态 |
| GET | `/kael/api/v1/conversations/{id}/approvals` | 查询当前 Principal 可见的 pending 或历史 Approval |
| POST | `/kael/api/v1/conversations/{id}/messages` | 创建输入 Message |
| POST | `/kael/api/v1/conversations/{id}/branches` | 从指定 Message 创建分支 Conversation |
| POST | `/kael/api/v1/messages/{id}/regenerations` | 基于指定用户问题创建新的回答版本 |
| POST | `/kael/api/v1/artifacts` | 上传经限制和校验的图片或文件 |
| GET | `/kael/api/v1/artifacts/{id}` | 所有权校验后读取 Artifact 元数据 |
| GET | `/kael/api/v1/artifacts/{id}/content` | 所有权校验后读取 Artifact |
| DELETE | `/kael/api/v1/artifacts/{id}` | 删除未引用或允许删除的 Artifact |
| POST | `/kael/api/v1/transcriptions` | 禁用占位端点；bootstrap `transcription=false` 且固定返回 unavailable，Lina 不调用 |
| POST | `/kael/api/v1/panel-sessions` | 创建 PanelSession |
| POST | `/kael/api/v1/panel-sessions/{id}/heartbeat` | 续租 |
| POST | `/kael/api/v1/panel-sessions/{id}/resume` | 使用 resume token 恢复 |
| DELETE | `/kael/api/v1/panel-sessions/{id}` | 关闭 PanelSession |
| PUT | `/kael/api/v1/panel-sessions/{id}/context` | 原子更新 Context version |
| PUT | `/kael/api/v1/panel-sessions/{id}/registrations` | 原子替换 Registry |
| DELETE | `/kael/api/v1/registrations/{id}` | 撤销 Registration |
| POST | `/kael/api/v1/runs` | 为已有输入 Message 创建 Run；当前只允许 foreground，background 被拒绝且 Lina 不提交 |
| GET | `/kael/api/v1/runs/{id}` | 获取 Run 状态与可用恢复动作 |
| POST | `/kael/api/v1/runs/{id}/cancel` | 幂等取消 Run |
| POST | `/kael/api/v1/runs/{id}/resume` | 显式恢复允许恢复的 Run |
| GET | `/kael/api/v1/panel-sessions/{id}/events` | 当前 Panel 的 SSE 事件流与游标恢复 |
| POST | `/kael/api/v1/tool-calls/{id}/results` | 回传增量或最终 ToolResult |
| GET | `/kael/api/v1/approvals/{id}` | 获取当前 Principal 可见的安全审批预览 |
| POST | `/kael/api/v1/approvals/{id}/decisions` | 提交 Approval 决定 |
| POST | `/kael/api/v1/admin/platform-registry/refresh` | 管理员刷新 Platform Registry |
| GET | `/kael/api/v1/admin/stats?days={1..365}` | 服务端管理员统计；默认 30 天，数值范围 clamp 到 1..365，非法文本回退默认值；Lina 无 stats 面板 |
| GET | `/kael/api/v1/admin/audit/conversations` | 管理员分页查询脱敏会话审计 |
| GET | `/kael/api/v1/admin/audit/conversations/{id}` | 管理员查看单个脱敏会话审计 |

Message 与 Run 为独立对象，以支持重试、重新运行、未来模型切换和评估。Lina 按原生 Message -> Run -> PanelSession SSE 顺序调用，不提供“发消息即运行”的旧 DTO 兼容层。

健康与运维路径不放入 API 版本，但仍统一在 `/kael` 下：

| 方法 | 路由 | 作用 |
|---|---|---|
| GET | `/kael/health/live` | 只检查进程和 HTTP Server 存活，不访问外部依赖 |
| GET | `/kael/health/ready` | 检查进程内 Store 与持久化 adapter；Core 模式在 2 秒超时内轻量探测 Runtime Store，不检查 Worker 或模型端点 |
| GET | `/kael/health/startup` | 初始化或恢复扫描较慢时用于启动探针 |
| GET | `/kael/internal/metrics` | 由网络策略保护的指标入口 |
| GET | `/kael/openapi.json` | 当前稳定 API 描述 |

### 11.3 路由与版本规则

Canonical 路径和版本规则：

- Canonical 路径不带尾斜杠。
- 调用方只使用 canonical 路径；不为尾斜杠变体或 Redirect 行为提供迁移兼容承诺。
- `/kael/api/v1` 的破坏性变更必须进入新 path version；新增可选字段和事件使用向前兼容规则。
- Event payload、Registration definition 和 Profile 各自保留独立 schema/version，不能只依赖 URL 版本。
- 不创建按产品或对话类型拆分的业务根路径；Conversation、Run 和 Event 只能从 `/kael/api/v1` 访问。
- 对话类型不能决定权限，也不能形成独立数据库表、状态机或协议分支。
- API 不接受客户端提交任意 system prompt、user ID、org ID 或最终授权工具集合。

### 11.4 旧路径退出

| 已退出路径 | 当前规则 |
|---|---|
| `/api/v1/chat-ai/...` | Core 业务路由已删除；仅保留组件签名的 `/api/v1/chat-ai/runtime-store/` |
| `/koko/agent/sessions/...` | Luna/Lina 不再调用，不作为客户端回退；Koko 仓库内物理清理另行处理 |

Kael 不新增 `/api/v1/chat-ai` 或 `/koko/agent` 顶级路由。Luna 与 Lina 都直接使用 `/kael/api/v1`。旧 Platform 数据不自动进入 Kael，也没有内置只读入口、read-through 或可写兼容 API；Lina 的旧 path、iframe/embed 和 DTO/SSE adapter 均已删除。Kael 只保留一套状态模型。

Luna Web 与 Electron 的 JSON、上传和 SSE 请求统一使用逻辑服务名 `kael`。Web 只保留不 rewrite 的 `/kael` 开发代理；Electron 的 `kael` 服务依次使用 `JMS_KAEL_DESKTOP_URL`、`JMS_KAEL_DEV_URL`，本地开发默认端口为 8083，生产环境无专用地址时使用当前 JumpServer origin。旧 `chat-ai`、`agent` 服务名及 `JMS_AI_*`、`JMS_AGENT_*` 环境变量不再支持，也不能作为别名指向新旧服务。

Koko 的 Terminal、MCP、WebSocket 等非 AI 路由不在本次迁移范围，保持原有调用关系；这不构成对旧 agentd 的保留或回退承诺。

生产网关必须把 site prefix 下完整的 `/kael/` 路径同源转发给 Kael，且不剥离 `/kael`；Kael 原生处理完整路径。开发代理与生产网关必须保持这一语义一致。

### 11.5 SSE

统一事件至少包括：

- conversation.updated；
- panel.ready、panel.resumed、panel.lease_expiring；
- registration.accepted、updated、expired、revoked；
- run.queued、started、waiting_capability、waiting_approval、completed、failed、cancelled、interrupted；
- message.created、delta、completed；
- model.requested、completed；
- tool.call、tool.progress、tool.completed、tool.failed、tool.cancelled；
- approval.required、resolved；
- stream.reset、error。

Lina 直接消费上述 dot 命名的 PanelDelivery 及其 `payload`，不映射为 `message_delta`、`message_done`、`approval_required` 等 Legacy event，也不维护旧字段别名 DTO。Luna 自身的产品适配边界与此分开。

Heartbeat 是无 ID 的 SSE comment，只属于传输保活，不是 DomainEvent 或 PanelDelivery，不分配 sequence，也不持久化。

要求：

- SSE `id` 与 PanelDelivery sequence 一致；
- sequence 在单个 PanelSession stream 内为正向严格递增安全整数；
- 客户端仅在成功消费后推进 cursor；
- 支持 query cursor 与 `Last-Event-ID`；
- 建连时 cursor 过期返回 `410 cursor_expired`；连接中要求重同步时发送 `stream.reset` 后关闭；
- DomainEvent 与状态同 Store 事务，PanelDelivery 先写入 Store 再发布；
- 当前协议客户端应忽略未知事件；
- ToolCall 与 Approval 只投递给 Run 的 PanelSession。

当前 Luna 单事件与缓冲区上限属于协议基线；调整必须作为协议版本变更。

## 12. Approval、安全与审计

### 12.1 权限原则

AI 永远没有额外权限：

- Platform Capability 使用当前用户的 Core 权限；
- Luna Session Capability 使用当前资源会话权限；
- Registration 只是能力提示；
- 执行端仍是最终安全权威；
- 权限撤销、Session revoke、资产授权变化必须在执行时生效。

### 12.2 Approval

风险至少分为 read、write、dangerous。

Approval 必须绑定：

- Conversation、Run、ToolCall、Registration；
- PanelSession；
- tool definition revision；
- arguments digest；
- risk、preview 和 expires time；
- Principal 和组织。

panel-scoped Approval 必须显示在发起 Run 的 Panel，不能在另一 Tab 代为确认；原 Panel 永久失效后必须过期或取消。service-scoped Approval 已按 ADR 0001 绑定原 Provider、ToolCall、definition 和 arguments digest，不能借此转移本地 ToolCall。

Approval 不能替代 Koko/Core 执行前的 ACL/RBAC 复验。

### 12.3 鉴权

Luna 当前存在两种入口：

- 浏览器：同源 Cookie、Org header、CSRF；
- Electron：主进程注入 Core OAuth Bearer、Org 和时区。

Kael 需要支持这两种产品形态，但 Runtime 核心只能接收已经验证的 Principal。当前只保留一个 Core identity adapter：它把调用方现有 Cookie 或 Bearer 与组织 Header 转发到 Core profile/permissions 接口实时校验，再把结果转换为 Principal。Kael 不相信 Luna 自报 user/org，也不维护未被当前客户端使用的第二套身份断言协议。

### 12.4 CSRF 与 Origin

- 浏览器写请求必须继续有 CSRF 防护；
- Origin/Referer 必须按可信站点检查；
- 不允许 wildcard Origin 配合 credentials；
- 浏览器标准部署通过 Lina 站点同源反向代理 `/kael/`；`ALLOWED_ORIGINS` 只参与服务端 Origin 校验，不发送 CORS 响应头，直接跨域时由外部代理负责完整的 credentialed CORS；
- HTTPS 在网关终止 TLS 时优先把精确外部 Lina origin 配入 `ALLOWED_ORIGINS`；无法固定外部 origin 时才在受信网关后设置 `TRUST_FORWARDED_HEADERS=true`，并由网关删除或覆盖客户端同名头、提供真实 forwarded host/proto；Kael 端口不得直接暴露到不可信网络；
- token 不放 URL query；
- Electron renderer 不能自行注入 Authorization/Cookie；
- 浏览器和 Electron 只发送当前协议明确支持的 CSRF header，不保留旧 header 别名。

### 12.5 审计链

每次 AI 操作至少能够关联：

```text
Principal
  -> Conversation
  -> Run
  -> Model Request
  -> ToolCall
  -> Approval
  -> Registration
  -> Executor audit reference
  -> ToolResult
```

审计记录只保留必要摘要；token、凭据、完整敏感内容和未脱敏模型请求不得进入普通审计日志。

## 13. Store 与实例生命周期

### 13.1 当前 Store

Runtime 只依赖 `ports.Store` 和 `ports.Tx`。默认 adapter 仍以单进程 Memory 执行事务，但在发布 next state 前，通过组件签名把与 JSONL 相同的 snapshot/delta Journal CAS 追加到 Core `/api/v1/chat-ai/runtime-store/`。启动时分页重放最新 snapshot 与后续 delta，达到 4096 条 delta 后提交新 snapshot。`RUNTIME_STORE=jsonl` 才使用 `data/store/runtime.jsonl` 和 `data/events/<conversation-id>.jsonl`。Kael 不包含 DSN、数据库 driver、ORM、schema 或 migration。

Artifact 元数据和有界提取文本进入 Runtime Journal，但原始文件内容目前仍保存在 Kael 私有 `data/artifacts`；组件 AccessKey 位于 `data/keys/.access_key`。这两个目录必须使用服务账号私有持久卷，节点替换或故障切换时重新挂载或迁移 Artifact 卷，否则只能恢复元数据和提取文本。旧 Koko `data/agent/events` 和旧 Platform ORM 数据不属于新 Journal，也不由 Kael 启动流程读取。

### 13.2 当前生命周期限制

- Kael 重启后恢复 Conversation、Message、终态 Run、DomainEvent、幂等索引和审计状态；
- 活动 Run 收敛为 `interrupted`，活动 ToolCall 收敛为 `cancelled`，未决 Approval 收敛为 `expired`，不得自动重复执行；
- PanelSession、Registration、Run claim、活动 cursor 和 executor owner 只在当前进程内有效；
- 不同 Kael 实例可以读取同一 Core Journal，但内存状态不实时同步，revision 冲突不自动重载；生产必须设置 `replicas=1`，使用 `Recreate` 或严格的先停旧实例、fencing 后再启动新实例流程，会话粘性不能代替单 writer；开发、预发和生产不得让不同 Kael 同时写同一 Core `default` store；
- 后台 Run 仍被禁用；Core-backed durable history 不等于可安全恢复后台执行；
- 浏览器 executor 连接只存在于当前拥有该 PanelSession 的 Kael 实例内。

### 13.3 后续分布式执行

Core-backed Journal 已解决权威持久化位置，但没有提供分布式 claim、事件唤醒或 Panel 路由。需要跨重启续跑、后台执行或多实例并发时，必须在 Store port 后增加相应协调语义；不得把产品、数据库 driver 或 ORM 细节泄漏到 Runtime、领域对象、HTTP API、Event schema 或 Luna 协议。启用前必须另外验证分布式 ownership、非幂等 ToolCall 恢复、Approval 绑定和 cursor 保留策略。

## 14. Luna 改造范围

### 14.1 首期 UI

Luna 显式呈现：

- 普通对话；
- Luna 能力对话。

新建时确定类型。当前页面焦点只用于推荐默认入口或可连接的 Context，不能更改既有 Conversation 类型。

历史列表未来应展示：

- 类型；
- assistant/preset；
- Luna 能力域和资源摘要；
- capability 在线/离线状态；
- 最近 Run 状态。

### 14.2 首期适配策略

本节只描述 Luna 的两个 Panel Controller。为控制 Luna 改动规模，可以暂时保留 Platform Panel 和 Workspace Panel 的内部 Controller，但二者都连接 Kael，并通过 Luna Adapter 映射到统一模型。Lina 不在该 Compatibility Adapter 内，已经直接使用 Kael DTO 和 PanelDelivery。

首期不强行合并：

- Platform `content/result_cards` 与 Workspace `UIMessage.parts`；
- 两套现有 SSE event names；
- Platform conversation cancel 与 Workspace run cancel；
- Platform assistant selector 与 Workspace domain adapter；
- 两种 Approval 展示细节。

这些差异由 Luna Compatibility Adapter 隔离，不能泄漏为 Kael 内两套领域模型。

### 14.3 保持不变

- Koko Terminal/SFTP WebSocket；
- Chen WebSocket；
- Script 本地执行器；
- MCP manifest 和 tools/call/cancel；
- SQL/Script proposal 的 apply/reject 与 revision 校验；
- 文件 path/version 安全；
- 敏感 metadata 过滤；
- 浏览器和 Electron 双运行时。

### 14.4 后续前端收敛

未来抽取通用 AI Client，统一负责：

- Conversation；
- PanelSession；
- Context；
- Registration；
- Run；
- SSE/reconnect；
- ToolCall/ToolResult；
- Approval；
- Event projection。

各产品域只保留 Context、Capability Adapter 和 Renderer。

## 15. Kael 推荐模块边界

Kael 当前按下列边界组织模块，不恢复旧聊天代理：

| 模块 | 职责 |
|---|---|
| bootstrap | 配置、依赖组装、生命周期和优雅退出 |
| api | `/kael/api/v1` HTTP/SSE、协议投影 Adapter、DTO 校验 |
| identity | 可信 Principal、组织和入口认证适配 |
| conversation | Conversation 与 Message |
| run | Run Supervisor、状态机、取消和恢复 |
| runtime | 通用 Agent Loop 与 Context Builder |
| model | Provider、模型路由、预算和错误分类 |
| capability | Registration、lease、invocation 和 result |
| policy | Profile、Prompt、风险与 Approval policy |
| store | Store/Tx port、Core Journal adapter、本地 JSONL 回退和未来分布式协调边界 |
| event | 持久 DomainEvent log、SSE 和 PanelSession replay |
| audit | 审计关联与脱敏 |
| observability | 日志、指标和 tracing |

模块命名和核心接口不得以 Luna、Koko 或 MCP 为中心。产品适配只允许出现在 API/adapter 边界。

## 16. 迁移实施记录

阶段 0 至阶段 3 的代码改造已完成。以下条目记录已落地范围和发布前仍需执行的环境验证，不表示继续保留旧接口或双栈回退。

### 阶段 0：冻结基线

- 固化本文档；
- 列出 Koko agentd 行为和 Provider 矩阵；
- 盘点现有 Platform AI 服务端的准确语义工具；
- 冻结 Luna HTTP/SSE/MCP 兼容输入；
- 从当前 Platform Handler、Serializer、Runner 生成兼容 fixture，不能直接使用已漂移的旧文档和 SSE Schema；
- 冻结选择为过渡 Headless Platform Gateway 承接前台 Platform Capability，同时保持 Runtime core 的长期业务无关边界；
- 确认组件注册、Core identity adapter、TerminalConfig 模型配置和 Store 方案；
- 确认新客户端只使用 `/kael/api/v1`，不建立旧 Runtime 或 agentd 回退开关。

### 阶段 1：Kael 基础与普通对话

- 恢复可构建、可部署的 Kael 服务；
- 建立组件注册、Identity、Config、Store、Event 和 Model 基础；
- 建立统一 Conversation/Message/Run；
- 迁移普通对话、持久历史、流式输出和取消；
- 通过 `/kael/api/v1` 和 Luna Compatibility Adapter 承接 Luna Platform Panel；Lina 直接消费 Kael 原生 DTO/PanelDelivery，不在 Kael 暴露旧顶级路由；
- 迁移附件、branch/regenerate 和 audit；Kael 保留服务端 stats API 但 Lina 删除旧 stats 面板；Lina 删除 background、Web Search、服务端 STT、通知请求，bootstrap 仍明示这些服务端能力不可用，浏览器原生语音识别不计入服务端能力；
- 按阶段 0 的决定承接前台 Platform 动态 OpenAPI、Core Tool 和进程内 Approval；旧 Koko Agent Session JSONL 不导入 Kael；
- 将 Platform 能力改造成受控 Registration，或在明确的过渡 Gateway 中隔离；
- Luna Platform Panel 切换到 Kael。

### 阶段 2：Luna Capability 与 agentd

- 建立 PanelSession、Context、Registration 和 lease；
- 迁移 Koko Agent Loop 与 Provider 能力；
- 去除 Runtime 中 JumpServer、resource session、MCP 和工具名硬编码；
- Luna 注册 Terminal、File、SQL、Script 能力；
- 完成 ToolCall、Approval、ToolResult、cancel 和恢复；
- 在 feature flag 后接入 Kael。

### 阶段 3：等价验证与切流

- 对普通对话和四个 Luna 能力域做行为级回归；
- 验证多 Tab、断线、重连、Approval、cancel 和非幂等安全；
- 在未上线环境完成用户、组织和权限组合验证；
- 观测成功率、首 token 延迟、Tool 失败率、恢复率和审计完整度；
- Luna 停止访问 Koko agentd；
- 发布失败时停止创建新 Kael Run，排空或取消在途 Run/Approval，再部署相互匹配的 Lina/Luna/Kael 构建并保留 Core Journal 与 Artifact 卷；不得恢复旧 Core ChatAI Runtime、旧客户端路径或 Koko agentd。

### 阶段 4：未来设计演进

- Lina 接管 Platform Capability；
- 普通对话显式连接/断开 Capability；
- Capability 对话跨环境重新绑定；历史持久化已由当前 Core-backed Journal 提供；
- 增加 Magnus、Lion、RDP 和 UI Action；
- 统一 Luna/Lina AI SDK；
- 在不改变 Conversation/Run 协议的前提下增加新 Profile 和 Capability Provider。
- 在现有 Core-backed adapter 之外补齐分布式 claim/ownership、状态同步、事件唤醒、准确 Panel 路由和安全工具恢复后，再评估后台 Run、跨重启续跑和多实例共享。

Koko 旧 agentd 的物理删除必须在允许修改 Koko 后另立阶段。

本期阶段 0 至阶段 3 已完成的代码状态与尚需环境执行的发布 Gate，统一记录在 [迁移实施状态](./MIGRATION_STATUS.md)。阶段 4 与 Koko 旧代码物理删除仍是后续工作，不能混入本期改动；旧 agentd 在此期间也不是客户端回退路径。

## 17. 精简测试策略

### 17.1 原则

本次迁移不把 Koko 和 Luna 的现有测试逐文件复制到 Kael，也不为简单 DTO、getter、框架路由或 UI 样式添加测试。

只为以下风险保留测试：

- 状态机错误会造成重复执行或状态丢失；
- 路由错误会造成跨用户、跨组织或跨 Tab 执行；
- 恢复错误会重复危险操作；
- Provider 差异会破坏 Agent Loop；
- 协议错误会使 Luna 与 Kael 无法联通；
- 权限、Approval 或敏感数据边界可能失守。

测试优先使用表驱动和少量端到端契约场景，在一个场景内覆盖完整状态转换，避免大量重复 setup、重 mock 和单字段断言。

### 17.2 最小必要测试主题

目标是八个跨层主题，而不是为每个路由、事件、Provider 和工具复制测试文件：

| ID | 主题 | 必须覆盖 |
|---|---|---|
| T1 | 架构守卫 | Kael Runtime 不依赖 Koko/MCP/Core 业务包；Luna 不再访问 Koko agentd |
| T2 | 普通对话完整链路 | Conversation、附件、stream、branch/regenerate、cancel、持久 history/进程内 Approval、audit，以及 Lina 不发 background/Web Search/服务端 STT/stats 请求 |
| T3 | Run 与 Provider | 状态机、预算、partial answer；OpenAI、compatible、DeepSeek 能力协商和 fallback |
| T4 | Panel 精确路由 | 同用户多 Panel、同名工具、lease/revision、跨 Tab/组织拒绝 |
| T5 | Tool 与 Approval | schema、argument repair、写防重、读刷新、digest、approve/reject、result/cancel 幂等 |
| T6 | Store 与 Event | Core Journal 分页/CAS 与 JSONL 回退恢复、先写 Store 后发布、Panel cursor replay/过期、重启安全收敛和非幂等不重放 |
| T7 | Luna Adapter | Terminal、File、SQL、Script 的最小 manifest fixture 与一条代表性完整执行链 |
| T8 | 身份与环境边界 | 浏览器 Cookie/CSRF、Electron Bearer/Org、Luna/Lina 零旧 Runtime/agentd 流量，以及旧 AI 表已清理或开发库已重建 |

同一不变量只在最低层测试一次；API 层只验证边界和映射，不重复 Runtime 内部全部分支。不测试纯展示 class、文案、第三方组件内部行为或重复的 domain adapter 样板。

### 17.3 测试代码控制

- 不原样迁移旧测试文件；先抽取行为矩阵，再选择最少场景覆盖不变量。
- 共享 fixture 只保留协议级最小对象，避免庞大通用 mock framework。
- 优先真实 Store adapter 和本地 HTTP handler，减少层层接口 mock；Core adapter 验证分页、snapshot 和 revision conflict，JSONL 回退恢复使用临时目录验证。
- Provider 使用小型确定性 fake，只覆盖能力协商和异常边界。
- UI 测试以 Controller/Adapter 为主，不重复验证 Vue 框架渲染。
- Bug 回归测试必须对应明确不变量；修复后不添加无关组合。
- 测试数量和代码行不是质量目标，关键安全与恢复路径覆盖才是目标。

## 18. 验收标准

### 18.1 普通对话

- Conversation 可创建、列表、继续、改名和删除；
- 消息可流式返回并在同一 PanelSession 内断线重连后恢复，历史消息可跨 Kael 重启读取；
- cancel、Run 和 Approval 状态可从权威快照恢复；Kael 重启时未完成 Run/ToolCall/Approval 收敛为安全终态，不续跑；
- panel-scoped Approval 只允许同一 PanelSession 在 lease 内 resume 后继续决定；原 Panel 永久失效后只能查看终态；service-scoped Approval 按 ADR 0001 绑定原 ToolCall、operation、definition 和参数 digest；
- assistant/preset 和 starter prompt 保留；
- 图片和文件按 feature 正常工作；Lina 不暴露或请求 Web Search、服务端 STT，且没有旧 stats 面板；bootstrap 仍明确标记相关服务端能力不可用，浏览器原生 `SpeechRecognition` 不经过 Kael；
- branch 和 regenerate 保留；后台 Run 在补齐分布式 ownership、状态同步和安全工具恢复前明确禁用；
- Platform activity/result 能正常展示；
- 服务端 stats、Lina 脱敏 audit、清理和 Runtime Journal 数据保留语义明确；
- 普通对话不会因当前 SSH/File/SQL/Script Tab 获得 Session Capability；
- Platform Tool 使用当前用户权限并产生审计。

### 18.2 Luna 能力对话

- Terminal、File、SQL、Script 均可完成核心任务；
- Context 动态更新并按 version 固定到 Run；
- manifest 原子注册并有 registry revision；
- ToolCall 包含 PanelSession 和 Registration 绑定；
- Approval 只在原 Panel 显示；
- cancel 传播到真实 executor；
- Panel 断开不向其他 Tab 转移；
- 非幂等工具不被自动重复执行；
- 资源凭据不离开 Koko/Chen/本地执行器。

### 18.3 Runtime 等价性

- Provider 与 fallback 矩阵保持；
- Agent Loop、预算、schema、argument repair 和去重保持；
- history、DomainEvent 和幂等索引可跨重启保持；Panel cursor 属于具体 PanelSession，重启后不恢复；
- Approval、安全限制和错误分类保持；
- 模型密钥由 Core TerminalConfig 下发，只存在于服务端内存和发往模型 Provider 的请求中；
- Kael 没有具体业务工具执行代码。

### 18.4 客户端与发布边界

- Luna 的普通 AI 和 Agent Runtime 请求都进入 Kael；
- Lina 的 AI Runtime 请求只进入 Kael；直接消费 PanelDelivery dot 事件，不存在 iframe/embed、旧 DTO/SSE adapter、background、服务端 STT、Web Search 请求或 stats 面板；浏览器原生语音识别不属于 Kael Runtime；
- Luna 到 Koko/Chen 的会话与 MCP 流量保持；
- Koko `/koko/agent/*` 不再收到 Luna 业务请求；
- 旧 Koko agentd 不接收 AI 客户端请求，也不作为失败回退；
- 发布 runbook 必须包含在途 Run/Approval 处置，以及 Core Journal 与 Artifact 卷保留要求；不得恢复旧 Core Runtime、旧客户端路径或服务名别名；
- Koko、Chen、Magnus 和 Lion 未被本期代码修改；Core 旧 ChatAI Runtime/API/models/worker 已删除，只保留 Runtime Store 及必要的组件支持，Lina 切流不改变 Runtime 协议。

## 19. 已冻结事项与发布 Gate

以下事项由 ADR 和实施状态共同约束：

1. 生产网关对 `/kael/` 的保留前缀转发和 SSE 参数；`/kael/internal/metrics` 无业务用户认证，必须由网络 ACL 只开放给监控网络；
2. 浏览器 Cookie 与 Electron Bearer 由 Kael Core identity adapter 实时校验并转换为可信 Principal；
3. 模型配置、API Key 和 Secret 由 Kael component 使用签名身份从 Core TerminalConfig 读取；
4. 当前默认使用 Core-backed Runtime Journal 且 Kael 不连接数据库；本地 JSONL 只作为显式回退；
5. Koko Agent Session JSONL 和旧 Platform 数据均不导入 Kael；使用过旧开发分支的环境必须清理旧 AI 表或重建开发库；
6. Platform AI 当前已选择过渡 Headless Gateway 承接前台能力；长期 Provider 替换不改变 Runtime 协议；
7. 当前 Platform AI 只承接前台动态 OpenAPI/Core Tool 和进程内 Approval；后台、站内信及相关跨重启能力不启用；
8. 当前 Platform AI 服务端语义工具的完整清单；
9. Luna 当前 Panel Adapter 的收敛版本；Lina 已收敛到原生 Kael DTO/PanelDelivery，旧 Core 路径、旧 DTO/iframe 和 Koko agentd 均不设客户端回退；Koko 非 AI 路由不在本期范围；
10. feature flag、启用维度和指标阈值必须由部署方结合容量与 SLO 在发布前定义，仓库不臆造统一阈值。

推荐默认值：

- 使用唯一权威业务根路径 `/kael/api/v1`，不提供其它业务根路径或入口别名；
- 使用调用方已有 Cookie/Bearer，通过 Core profile/permissions 实时解析 Principal；
- 非 superuser 继续要求 `chat_ai.use_chatai`；Run 固化创建时的最小授权快照用于异步 operation 可见性，Core 在实际调用时实时复核权限；
- Kael 以 `kael` Terminal component 注册，首次使用 BootstrapToken，后续使用私有 AccessKey；
- 模型 Secret 来自 Core TerminalConfig，不写入 Kael 部署配置，也不经 Luna；
- Runtime 当前使用 Core-backed Journal 保留 history/DomainEvent；需要后台续跑或多实例共享时另加分布式 ownership，不预设 Kael 直连数据库；
- Luna 当前 UI 通过单一 Kael Adapter 接入，再逐步统一 Client SDK 与 Event projection；不保留旧后端路由；
- 使用过旧开发分支的数据库应清理旧 AI 表或直接重建；当前 Runtime 不提供导入、只读入口、read-through 或双写。

## 20. 文档维护规则

每次 AI 相关变更至少检查本文以下部分是否需要同步更新：

- 产品类型与用户行为；
- 组件职责；
- 领域对象和状态；
- API、SSE、Registration、Approval；
- 鉴权、权限和审计；
- 迁移阶段和完成状态；
- 精简测试矩阵；
- 验收标准。

禁止只修改实现而不记录协议或行为变化。也禁止把尚未实现的设计在文档中标记为已完成。
