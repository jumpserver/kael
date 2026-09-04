# Kael AI Native Agent Runtime 迁移与演进方案

## 1. 文档状态

| 项目 | 内容 |
|---|---|
| 状态 | 设计基线 |
| 日期 | 2026-09-04 |
| 目标仓库 | Kael、Luna |
| 参考 | 《JumpServer AI Native Agent Runtime 完整方案》与 Koko/Luna/Kael 当前实现 |
| 本期代码范围 | 只修改 Kael 和 Luna |
| 实施状态 | ADR 0002/0004 范围内的代码级逻辑和 Kael JSONL Store 已完成；生产切流 Gate 见 `MIGRATION_STATUS.md` |

本文是后续 AI 改造的实现、评审和验收基线。若代码与本文冲突，应先明确并记录新的架构决策，再修改本文和代码。

[ADR 0002](./adr/0002-core-component-and-store-port.md) 与 [ADR 0004](./adr/0004-jsonl-store-and-event-protocol.md) 已冻结本期运行形态：Kael 与 Koko 一样注册为 Core Terminal component，模型配置来自 TerminalConfig；Kael 不连接数据库，当前使用本地 JSONL Store 保留由 Kael 创建的历史和 DomainEvent，但不读取或导入 Koko `data/agent/events/*.jsonl`。后台 Run、多实例共享、活动 Panel 能力和未决 Approval 的跨重启续接仍禁用。

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

### 2.3 本期只改 Luna 和 Kael

本期允许：

- 在 Kael 中建立新的通用 Agent Runtime；
- 将 Koko agentd 的通用 AI 能力迁入 Kael；
- 将 Platform AI 的对话能力迁入 Kael；
- 修改 Luna，使普通对话和 Luna 能力对话都连接 Kael；
- 在 Luna 中建立 Platform/Luna Capability Adapter；
- 保持 Luna 到 Koko、Chen 和本地工具的既有执行链。

本期不允许：

- 修改或删除 Koko 中现有 agentd 代码；
- 修改 Koko、Chen 的 MCP/会话工具协议；
- 修改 Lina、Core、Magnus 或 Lion；
- 让 Kael 直接执行终端、文件、数据库或页面操作；
- 让 Kael 持有资源连接凭据。

因此本期是“在 Kael 重建并由 Luna 切流”的逻辑迁移。Koko 中旧 agentd 的物理删除或启动开关调整属于后续独立变更。

## 3. 架构原则

目标架构为：

```text
AI Panel Host + Headless Agent Runtime + Local Capability Gateway
```

调用关系：

```text
User
  |
  v
Luna AI Panel
  |  Agent Runtime Protocol: HTTP + SSE
  v
Kael Agent Runtime
  |
  +-- panel binding --> 原 Luna PanelSession
  |                       +-- Luna MCP Adapter --> Koko / Chen
  |                       +-- Luna Local Adapter --> Script / UI
  |                       +------------- tool.result --> Kael
  |
  +-- service binding --> Headless Platform Gateway --> Core API（当前用户权限）
```

必须始终满足：

1. Kael 是决策与编排 Runtime，不是业务能力执行器。
2. Luna 是当前环境的 Agent Host、Context Provider 和 Capability Gateway。
3. Koko、Chen 和 Luna UI 是 panel binding 后面的能力来源；Core API 位于隔离的 service provider 后面，对 Runtime core 保持不可见。
4. MCP 只用于 Luna 与 Session Component 之间，不是 Kael 的 Runtime 协议。
5. AI 不获得当前用户之外的任何权限。
6. Registration 只描述能力；真正执行时仍由 Core、Koko 或 Chen 复验权限。
7. ToolCall 必须回到发起 Run 的准确 PanelSession，禁止广播或跨 Tab 自动转移。
8. Conversation 历史可跨同一数据目录的 Kael 重启存在；PanelSession 与 executor 不可跨进程恢复，正在执行的 Run 不能在环境之间漂移或自动重放。

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

### 4.4 Platform Capability 的过渡责任

目标方案中，Platform Capability 最终适合由 Lina AI Panel 承载。但本期只能修改 Luna 和 Kael，必须区分前台过渡和完整 Platform 兼容：

- Kael 仓库内的隔离 Headless Gateway 提供 Platform Capability；
- Gateway 使用请求绑定的一次性 HMAC 委托传递当前 user/org，Core 执行最终 RBAC；
- 动态 OpenAPI 只通过受控 operation registry 暴露，并按 Profile、HTTP method、schema 和敏感路径限制；
- Kael Runtime core 只识别通用 Registration、ExecutionBinding 和 ToolResult；
- 未来 Lina 接管同名或新版语义能力时，只替换 Capability Provider，不改变 Kael Runtime。

该过渡方案与 Legacy 只读策略已由 [ADR 0001](./adr/0001-headless-platform-gateway.md) 冻结。站内信、STT 和 Web Search 仍按 bootstrap feature 明确关闭，旧 Lina 对应能力继续由旧服务承接。

## 5. 当前能力基线

“完整迁移”以当前 Koko agentd、Platform AI 服务端代码以及 Luna/Lina 实际调用为准，不以旧 Kael 实现或已漂移文档为准。

### 5.1 Koko agentd Runtime

必须迁移并保持的核心语义：

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

普通对话必须承接当前 Platform AI 的产品能力：

- Conversation 列表、新建、详情、改名、Assistant 切换和删除；
- 分页消息历史、图片和文件附件；
- assistant/preset 与 starter prompts；
- 流式回答、后台运行、取消和运行恢复；
- regenerate 和 branch；
- pending Approval 恢复；
- activity/result card 展示；
- 当前已有的 API search、API call、Web search 和安全策略；
- STT、用户/全局限流和部署依赖；
- 管理员 stats 和脱敏 Conversation audit；
- Run/Token/API 审计、数据保留和僵尸状态清理；
- `general`、`management`、`asset`、`session_audit`、`ops` preset。

这些 preset 是模型策略和能力组合，不是新的 Conversation 类型。

当前 `general` 是 chat-only；`management` 才是管理员范围内的动态 Core 管理能力。旧 Platform 文档对此描述相反，迁移以实际代码为准。Scheduled Report 当前没有代码实现，不纳入现状基线。

Luna 现有 Platform Panel 只使用上述能力的一个子集；Lina 已使用附件、联网、branch、regenerate、后台、STT、stats 和 audit。本期不修改 Lina，生产网关继续把它的旧路径流量留在 Platform AI；Kael 仍必须按 Lina 已使用的完整能力设计 `/kael/api/v1`，供未来 Lina 直接切换，不能只迁移 Luna 子集。

## 6. 产品模型

### 6.1 普通对话

普通对话具有以下特征：

- 长生命周期，可在没有资源会话时创建、查看和继续；
- 不隐式绑定当前 Terminal、File、SQL 或 Script；
- 支持 assistant/preset；
- `general` 首期可采用纯模型模式；
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

当前允许 foreground+disabled、foreground+panel 和 foreground+service；所有 background 组合返回 `background_requires_durable_store`。当前本地 JSONL 的 durable 只表示历史落盘，不代表具备后台 ownership 或安全续跑条件；未来接入支持分布式 claim 与工具恢复的外部 Store 后，background+disabled 和 background+service 可重新评估，background+panel 始终非法。service mode 已按 ADR 0001 绑定可信 Headless Provider，不能伪装成浏览器 Panel。

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
- DomainEvent 使用 Conversation 内递增 `seq` 写入 `data/events/<conversation-id>.jsonl`；PanelDelivery 另有 PanelSession 内 sequence；
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

### 11.2 目标 API

| 方法 | 路由 | 作用 |
|---|---|---|
| GET | `/kael/api/v1/bootstrap` | 返回协议版本、feature、限制、CSRF 和稳定 cluster identity |
| GET | `/kael/api/v1/assistants` | 返回当前 Principal 可使用的 Assistant/Profile |
| GET | `/kael/api/v1/runtime-profiles` | Runtime Profile 与协议能力发现 |
| POST | `/kael/api/v1/conversations` | 创建 Conversation |
| GET | `/kael/api/v1/conversations?kind={general|capability}` | 按当前 Principal 和可选 Kind 分页查询 Conversation |
| GET | `/kael/api/v1/conversations/{id}` | 获取 Conversation |
| PATCH | `/kael/api/v1/conversations/{id}` | 改名、归档等元数据操作 |
| DELETE | `/kael/api/v1/conversations/{id}` | 删除或软删除 Conversation |
| GET | `/kael/api/v1/conversations/{id}/messages` | 分页读取消息 |
| GET | `/kael/api/v1/conversations/{id}/runs` | 分页读取 Run，并发现 active、interrupted 等可恢复状态 |
| GET | `/kael/api/v1/conversations/{id}/approvals` | 查询当前 Principal 可见的 pending 或历史 Approval |
| POST | `/kael/api/v1/conversations/{id}/messages` | 创建输入 Message |
| POST | `/kael/api/v1/conversations/{id}/branches` | 从指定 Message 创建分支 Conversation |
| POST | `/kael/api/v1/messages/{id}/regenerations` | 基于指定用户问题创建新的回答版本 |
| POST | `/kael/api/v1/artifacts` | 上传经限制和校验的图片或文件 |
| GET | `/kael/api/v1/artifacts/{id}/content` | 所有权校验后读取 Artifact |
| DELETE | `/kael/api/v1/artifacts/{id}` | 删除未引用或允许删除的 Artifact |
| POST | `/kael/api/v1/transcriptions` | 语音转写 |
| POST | `/kael/api/v1/panel-sessions` | 创建 PanelSession |
| POST | `/kael/api/v1/panel-sessions/{id}/heartbeat` | 续租 |
| POST | `/kael/api/v1/panel-sessions/{id}/resume` | 使用 resume token 恢复 |
| DELETE | `/kael/api/v1/panel-sessions/{id}` | 关闭 PanelSession |
| PUT | `/kael/api/v1/panel-sessions/{id}/context` | 原子更新 Context version |
| PUT | `/kael/api/v1/panel-sessions/{id}/registrations` | 原子替换 Registry |
| DELETE | `/kael/api/v1/registrations/{id}` | 撤销 Registration |
| POST | `/kael/api/v1/runs` | 为已有输入 Message 创建前台或后台 Run |
| GET | `/kael/api/v1/runs/{id}` | 获取 Run 状态与可用恢复动作 |
| POST | `/kael/api/v1/runs/{id}/cancel` | 幂等取消 Run |
| POST | `/kael/api/v1/runs/{id}/resume` | 显式恢复允许恢复的 Run |
| GET | `/kael/api/v1/panel-sessions/{id}/events` | 当前 Panel 的 SSE 事件流与游标恢复 |
| POST | `/kael/api/v1/tool-calls/{id}/results` | 回传增量或最终 ToolResult |
| GET | `/kael/api/v1/approvals/{id}` | 获取当前 Principal 可见的安全审批预览 |
| POST | `/kael/api/v1/approvals/{id}/decisions` | 提交 Approval 决定 |
| POST | `/kael/api/v1/admin/platform-registry/refresh` | 管理员刷新 Platform Registry |
| GET | `/kael/api/v1/admin/stats` | 管理员统计 |
| GET | `/kael/api/v1/admin/audit/conversations` | 管理员分页查询脱敏会话审计 |
| GET | `/kael/api/v1/admin/audit/conversations/{id}` | 管理员查看单个脱敏会话审计 |

Message 与 Run 为独立对象，以支持重试、重新运行、未来模型切换和评估。兼容层可以保留“发消息即运行”的旧行为，但内部必须拆分。

健康与运维路径不放入 API 版本，但仍统一在 `/kael` 下：

| 方法 | 路由 | 作用 |
|---|---|---|
| GET | `/kael/health/live` | 只检查进程和 HTTP Server 存活，不访问外部依赖 |
| GET | `/kael/health/ready` | 检查必要配置、Store 和 Worker 是否可服务 |
| GET | `/kael/health/startup` | 初始化或恢复扫描较慢时用于启动探针 |
| GET | `/kael/internal/metrics` | 由网络策略保护的指标入口 |
| GET | `/kael/openapi.json` | 当前稳定 API 描述 |

### 11.3 路由与版本规则

Canonical 路径和版本规则：

- Canonical 路径不带尾斜杠。
- 迁移期可以同时接受带尾斜杠形式，但必须直接处理，不能依赖 HTTP Redirect，尤其是 POST、上传和 SSE。
- `/kael/api/v1` 的破坏性变更必须进入新 path version；新增可选字段和事件使用向前兼容规则。
- Event payload、Registration definition 和 Profile 各自保留独立 schema/version，不能只依赖 URL 版本。
- 不创建按产品或对话类型拆分的业务根路径；Conversation、Run 和 Event 只能从 `/kael/api/v1` 访问。
- 对话类型不能决定权限，也不能形成独立数据库表、状态机或协议分支。
- API 不接受客户端提交任意 system prompt、user ID、org ID 或最终授权工具集合。

### 11.4 旧路径退出

| 现有 Luna 调用 | 目标 |
|---|---|
| `/api/v1/chat-ai/...` | `/kael/api/v1/...`，由 Luna 普通对话 Adapter 映射 |
| `/koko/agent/sessions/...` | `/kael/api/v1/...`，由 Luna 能力对话 Adapter 映射 |

Kael 不新增 `/api/v1/chat-ai` 或 `/koko/agent` 顶级路由。迁移期内，未修改的 Lina 继续访问旧 Platform AI；未来修改 Lina 时再切换到 `/kael/api/v1`。Koko 的旧 agentd 路由可以暂时保留供旧客户端回滚，但新 Luna 对它的业务流量必须归零。兼容层不得让 Kael 保留第二套状态模型。

Luna Web 与 Electron 的 JSON、上传和 SSE 请求统一使用逻辑服务名 `kael`。Web 只保留不 rewrite 的 `/kael` 开发代理；Electron 的 `kael` 服务依次使用 `JMS_KAEL_DESKTOP_URL`、`JMS_KAEL_DEV_URL`，本地开发默认端口为 8083，生产环境无专用地址时使用当前 JumpServer origin。旧 `chat-ai`、`agent` 服务名及 `JMS_AI_*`、`JMS_AGENT_*` 环境变量只能作为有期限的弃用别名，且必须指向同一 Kael 服务，禁止回退到旧 8088 或 5050 服务。

切换完成后，Luna 删除 `/api/v1/chat-ai/` 和 `/koko/agent/` 的开发代理与客户端常量；`/koko/` 下 Terminal、MCP 和 WebSocket 等非 agentd 路由继续保留。

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

Heartbeat 是无 ID 的 SSE comment，只属于传输保活，不是 DomainEvent 或 PanelDelivery，不分配 sequence，也不持久化。

要求：

- SSE `id` 与 PanelDelivery sequence 一致；
- sequence 在单个 PanelSession stream 内为正向严格递增安全整数；
- 客户端仅在成功消费后推进 cursor；
- 支持 query cursor 与 `Last-Event-ID`；
- 建连时 cursor 过期返回 `410 cursor_expired`；连接中要求重同步时发送 `stream.reset` 后关闭；
- DomainEvent 与状态同 Store 事务，PanelDelivery 先写入 Store 再发布；
- 未知事件对旧客户端可忽略；
- ToolCall 与 Approval 只投递给 Run 的 PanelSession。

现有 Luna 单事件与缓冲区上限在兼容阶段保持不变；调整必须作为协议版本变更。

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
- token 不放 URL query；
- Electron renderer 不能自行注入 Authorization/Cookie；
- 兼容旧 CSRF header 时，应记录废弃计划。

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

Runtime 只依赖 `ports.Store` 和 `ports.Tx`。当前 adapter 是单进程本地 JSONL Store，为 Conversation、Message、Run、ToolCall、ToolResult、Approval、DomainEvent、PanelDelivery、Profile 和审计索引提供事务与查询语义。`data/store/runtime.jsonl` 保存带校验的状态 journal；`data/events/<conversation-id>.jsonl` 保存可读的 Conversation DomainEvent 归档。Kael 不包含 DSN、数据库 driver、ORM、schema 或 migration。

Artifact 内容和组件 AccessKey 是独立的私有文件，不属于 Runtime Store。旧 Koko `data/agent/events` 不属于 Kael Runtime Store，也不由 Kael 启动流程读取。

### 13.2 当前生命周期限制

- Kael 重启后恢复 Conversation、Message、终态 Run、DomainEvent、幂等索引和审计状态；
- 活动 Run 收敛为 `interrupted`，活动 ToolCall 收敛为 `cancelled`，未决 Approval 收敛为 `expired`，不得自动重复执行；
- PanelSession、Registration、Run claim、活动 cursor 和 executor owner 只在当前进程内有效；
- 不同 Kael 实例不共享本地 JSONL 状态，入口必须保持单写实例和会话粘性；
- 后台 Run 仍被禁用；本地 durable history 不等于可安全恢复后台执行；
- 浏览器 executor 连接只存在于当前拥有该 PanelSession 的 Kael 实例内。

### 13.3 未来外部 adapter

需要跨重启续跑、后台执行或多实例共享时，通过同一 Store port 新增外部 adapter。该 adapter 可以使用任意合适的外部存储，但不得把产品、数据库 driver 或 ORM 细节泄漏到 Runtime、领域对象、HTTP API、Event schema 或 Luna 协议。启用相关能力前必须另外验证分布式 ownership、非幂等 ToolCall 恢复、Approval 绑定和 cursor 保留策略。

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

为控制改动规模，可以暂时保留当前 Platform Panel 和 Workspace Panel 的内部 Controller，但二者都连接 Kael，并通过 Adapter 映射到统一模型。

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

Kael 当前是清空后的服务骨架，应建立清晰的内部模块，而不是恢复旧聊天代理：

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
| store | Store/Tx port、本地 JSONL adapter 和未来外部 adapter 边界 |
| event | 持久 DomainEvent log、SSE 和 PanelSession replay |
| audit | 审计关联与脱敏 |
| observability | 日志、指标和 tracing |

模块命名和核心接口不得以 Luna、Koko 或 MCP 为中心。产品适配只允许出现在 API/adapter 边界。

## 16. 迁移阶段

### 阶段 0：冻结基线

- 固化本文档；
- 列出 Koko agentd 行为和 Provider 矩阵；
- 盘点现有 Platform AI 服务端的准确语义工具；
- 冻结 Luna HTTP/SSE/MCP 兼容输入；
- 从当前 Platform Handler、Serializer、Runner 生成兼容 fixture，不能直接使用已漂移的旧文档和 SSE Schema；
- 在严格目标架构、过渡 Headless Platform Gateway、保留旧 Platform 服务三条路径中明确选择；
- 确认组件注册、Core identity adapter、TerminalConfig 模型配置和 Store 方案；
- 定义灰度和回滚开关。

### 阶段 1：Kael 基础与普通对话

- 恢复可构建、可部署的 Kael 服务；
- 建立组件注册、Identity、Config、Store、Event 和 Model 基础；
- 建立统一 Conversation/Message/Run；
- 迁移普通对话、持久历史、流式输出和取消；
- 通过 `/kael/api/v1` 和 Luna Compatibility Adapter 无损承接当前 Platform Panel 协议，不在 Kael 暴露旧顶级路由；
- 迁移附件、branch/regenerate、stats/audit 等已确认范围；后台、Web Search、STT 和通知保持禁用并由 bootstrap 明示；
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
- 按用户或组织灰度；
- 观测成功率、首 token 延迟、Tool 失败率、恢复率和审计完整度；
- Luna 停止访问 Koko agentd；
- 保留快速回滚开关。

### 阶段 4：未来设计演进

- Lina 接管 Platform Capability；
- 普通对话显式连接/断开 Capability；
- Capability 对话跨环境重新绑定；历史持久化已由当前 JSONL Store 提供；
- 增加 Magnus、Lion、RDP 和 UI Action；
- 统一 Luna/Lina AI SDK；
- 在不改变 Conversation/Run 协议的前提下增加新 Profile 和 Capability Provider。
- 通过 Store port 增加外部 adapter 后，再评估后台 Run、跨重启续跑和多实例共享。

Koko 旧 agentd 的物理删除必须在允许修改 Koko 后另立阶段。

本期阶段 0 至阶段 3 的代码实现状态与尚需环境执行的发布 Gate，统一记录在 [迁移实施状态](./MIGRATION_STATUS.md)。阶段 4 与 Koko 旧代码物理删除仍是后续工作，不能混入本期改动。

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
| T2 | 普通对话完整链路 | Conversation、附件、stream、branch/regenerate、cancel、持久 history/进程内 Approval、stats/audit，以及 background 的明确拒绝 |
| T3 | Run 与 Provider | 状态机、预算、partial answer；OpenAI、compatible、DeepSeek 能力协商和 fallback |
| T4 | Panel 精确路由 | 同用户多 Panel、同名工具、lease/revision、跨 Tab/组织拒绝 |
| T5 | Tool 与 Approval | schema、argument repair、写防重、读刷新、digest、approve/reject、result/cancel 幂等 |
| T6 | Store 与 Event | JSONL 事务恢复、先写 Store 后发布、Panel cursor replay/过期、重启安全收敛和非幂等不重放 |
| T7 | Luna Adapter | Terminal、File、SQL、Script 的最小 manifest fixture 与一条代表性完整执行链 |
| T8 | 身份与切流 | 浏览器 Cookie/CSRF、Electron Bearer/Org、旧 Platform 数据与零旧 runtime 流量 |

同一不变量只在最低层测试一次；API 层只验证边界和映射，不重复 Runtime 内部全部分支。不测试纯展示 class、文案、第三方组件内部行为或重复的 domain adapter 样板。

### 17.3 测试代码控制

- 不原样迁移旧测试文件；先抽取行为矩阵，再选择最少场景覆盖不变量。
- 共享 fixture 只保留协议级最小对象，避免庞大通用 mock framework。
- 优先真实 Store adapter 和本地 HTTP handler，减少层层接口 mock；JSONL 恢复使用临时目录验证。
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
- 图片和文件按 feature 正常工作；Web Search 与 STT 当前由 bootstrap 明确标记为不可用；
- branch 和 regenerate 保留；后台 Run 在具备安全续跑条件的外部 Store adapter 完成前明确禁用；
- Platform activity/result 能正常展示；
- stats、脱敏 audit、清理和 JSONL 数据保留语义明确；
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

### 18.4 切流

- Luna 的普通 AI 和 Agent Runtime 请求都进入 Kael；
- Luna 到 Koko/Chen 的会话与 MCP 流量保持；
- Koko `/koko/agent/*` 不再收到 Luna 业务请求；
- 同一用户消息不会被旧 Koko agentd 与 Kael 双重执行；
- 灰度期间可以按明确开关回滚；
- Koko、Chen、Core、Lina 未被本期代码修改。

## 19. 已冻结事项与发布 Gate

以下事项由 ADR 和实施状态共同约束：

1. 生产网关对 `/kael/` 的保留前缀转发、SSE 参数以及探针和 metrics 访问策略；
2. 浏览器 Cookie 与 Electron Bearer 由 Kael Core identity adapter 实时校验并转换为可信 Principal；
3. 模型配置、API Key 和 Secret 由 Kael component 使用签名身份从 Core TerminalConfig 读取；
4. 当前使用本地 JSONL Store 且不连接数据库；未来外部存储只能通过 Store adapter 接入；
5. Koko Agent Session JSONL 不导入 Kael；旧 Platform Conversation 是否迁移历史数据仍需单独决定；
6. Platform AI 采用严格目标架构、过渡 Headless Gateway，还是阶段性保留旧服务；
7. 当前 Platform AI 只承接前台动态 OpenAPI/Core Tool 和进程内 Approval；后台、站内信及相关跨重启能力不启用；
8. 当前 Platform AI 服务端语义工具的完整清单；
9. Luna Compatibility Adapter 的收敛版本，以及旧 Platform/Koko 路由的回滚窗口和停止服务条件；
10. feature flag、灰度维度、指标阈值和回滚条件。

推荐默认值：

- 使用唯一权威业务根路径 `/kael/api/v1`，不提供其它业务根路径或入口别名；
- 使用调用方已有 Cookie/Bearer，通过 Core profile/permissions 实时解析 Principal；
- Kael 以 `kael` Terminal component 注册，首次使用 BootstrapToken，后续使用私有 AccessKey；
- 模型 Secret 来自 Core TerminalConfig，不写入 Kael 部署配置，也不经 Luna；
- Runtime 当前使用本地 JSONL Store 保留 history/DomainEvent；需要后台续跑或多实例共享时新增外部 Store adapter，不预设数据库产品；
- 先兼容现有 Luna UI，再逐步统一 Client SDK 与 Event projection；
- 旧 Platform 历史若无法可靠迁移，应明确只读保留，而不是隐式丢失。

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
