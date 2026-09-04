# Kael AI Runtime Architecture

## 1. 文档定位

| 项目 | 内容 |
|---|---|
| 状态 | 目标架构基线 |
| 日期 | 2026-09-04 |
| 适用范围 | Kael AI Runtime、Luna/Lina AI Panel 及其能力适配边界 |
| 当前实现状态 | Kael/Luna/Lina 逻辑迁移已实现；Kael 使用 Core component + Core-backed Runtime Journal，边界见 ADR 0002/0004/0006 |
| 本期代码范围 | Kael、Luna、Lina 与 JumpServer Core 的 Runtime Store 接口 |

本文将《JumpServer AI Native Agent Runtime 完整方案》的核心思想，与现有 Koko agentd、Platform AI、Luna/Lina 客户端的真实能力合并为 Kael 的长期架构约束。

三类文档的职责不同：

- 本文定义稳定架构、所有权和不可破坏的不变量；
- [AI Native Agent Runtime 迁移与演进方案](./ai-native-agent-runtime-migration.md) 定义迁移阶段、实施范围、验收标准和精简测试策略；
- [旧版 Platform AI 契约与当前迁移边界](./platform-ai-compatibility.md) 将已删除实现作为历史参考，并记录当前迁移边界。

初稿中未加 Kael 前缀的路径、示例代码、示例 TTL、目录树和技术选型建议不是最终协议。

文档和事实的权威边界是：

- 目标架构由本文和已经批准的 ADR 定义；
- 当前行为由实际代码和可执行契约定义；
- 迁移顺序与兼容方式由专题迁移文档定义；
- 三者冲突时不得自行选择其一，必须先记录 ADR，并同步更新受影响的文档、代码和契约。

## 2. 架构目标

Kael 采用：

> AI Panel Host + Headless Agent Runtime + Local Capability Gateway

Kael 是注册到 Core 的 Terminal component，也是通用推理与编排 Runtime；AI Panel 是 Agent Host。模型配置来自 Core TerminalConfig，只有 `CHAT_AI_ENABLED` 控制启停，不再存在 method/embed 模式。Koko、Chen 和本地 UI/Script 通过 Luna 的 panel binding 提供；Core API 通过 ADR 0001 的 service binding 提供，并由通用 Capability Broker 与 Runtime core 隔离。

核心不变量：

1. Kael Runtime 核心对象包括 Principal、Conversation、Message、Artifact、PanelSession、ContextSnapshot、Registration、ExecutionBinding、Run、Step/ModelCall、ToolCall、ToolResult、Approval、DomainEvent 和 PanelDelivery。
2. Kael Runtime 不理解 Lina、Luna、Koko、Chen、MCP、SSH、数据库或具体业务 API。
3. Luna 持有当前环境、能力执行器和结果渲染；Kael 不持有资源连接凭据。
4. 普通对话和 Luna 能力对话共用一套 Conversation、Run、Event 和模型调用链。
5. 所有 AI 业务接口只使用 `/kael/api/v1`，不提供其它业务根路径或按对话类型拆分的入口；Lina 不保留 iframe/embed、旧 DTO 或旧 SSE 事件映射。
6. Conversation 与 Message 历史默认通过 Core-backed Journal 跨 Kael 节点恢复；PanelSession、Registration 和 executor 连接不跨进程恢复。每次 Run 固定本次执行环境和能力快照。
7. ToolCall 必须路由到 Registration 绑定的准确 ExecutionBinding；当前启用的 panel binding 必须返回原 PanelSession，禁止广播、按用户猜测或自动转移到其它 Tab。
8. Context 是不可信数据，不是指令或权限。
9. Registration 只是能力声明；真正执行时仍由 Core、Koko、Chen 或本地执行器复验权限。
10. 状态与 Event 先提交到 Store 事务，再通知订阅者；DomainEvent 按 Conversation 持久化，PanelDelivery cursor 只属于具体 PanelSession。
11. 非幂等操作结果未知时不得自动重放。
12. 产品差异只存在于 Profile、Context、Capability Adapter 和 Renderer 边界，不能进入 Runtime 核心。

## 3. 系统上下文

### 3.1 当前逻辑架构

```text
Browser / Electron
        |
        v
+------------------------------------------------------+
| Luna / Lina AI Panel                                 |
|                                                      |
| Chat Surface | Context Adapter | Local Registry      |
| Tool Dispatcher | Approval Surface | Result Renderer |
+-------------------------+----------------------------+
                          |
                 HTTP commands + SSE events
                          |
                          v
+------------------------------------------------------+
| Kael                                                 |
|                                                      |
| Identity Adapter | API | Conversation | Run          |
| Context Builder | Agent Loop | Model Router          |
| Capability Broker | Approval | Event log | Audit    |
+--------------+---------------------------+-----------+
               |                           |
               v                           v
        Model Providers             Store Port
                                      |
                         Core Journal adapter (default)
                              / JSONL fallback

Panel-scoped tool execution remains behind Luna:

Luna MCP Adapter ------------------> Koko / Chen
Luna Local Adapter ----------------> Script / UI

Service-scoped Platform execution:

Kael Capability Broker ------------> Headless Platform Gateway ------------> Core API
```

普通对话命令由 Luna 或 Lina 通过 HTTP 提交给 Kael；Kael 按 PanelDelivery audience 通过 SSE 投递状态。Lina 直接读取 PanelDelivery DTO 及其 dot 事件名（如 `message.delta`、`tool.call`、`approval.required`、`run.completed`），不经过 Legacy DTO/SSE adapter。Luna 的 panel-scoped 文本增量、ToolCall 和 Approval 回到原 Panel；可共享的脱敏终态可以投影给同一 Conversation 的其它授权 Panel。panel-scoped ToolResult 由 Luna 通过 HTTP 回传。

service binding 已按 ADR 0001 启用：Capability Broker 把调用路由给 Headless Provider，该执行路径不使用 Panel SSE，状态和 Approval 仍通过统一 Event/PanelDelivery 投影。按 ADR 0004/0006，历史和 DomainEvent 可从 Core Journal 恢复，但未决 Approval、活动 ToolCall 和 Panel capability 不跨 Kael 重启续接。

### 3.2 当前迁移形态与长期形态

当前迁移形态由 Luna、Lina、Kael 和 Core Runtime Store 接口共同组成：

- Luna 同时承载普通对话 UI 和 Luna 能力对话 UI；
- Lina 的普通对话 API 层只使用 Kael 原生资源和 PanelDelivery dot 事件，不保留 iframe/embed、旧 DTO/SSE adapter 或旧 Platform Runtime 回退；
- Kael 的隔离 Headless Platform Gateway 承载 Platform Capability，Luna 只负责 UI、事件和 Approval 交互；
- Koko、Chen 和本地执行器继续提供现有 Session Capability；
- Core 的旧 ChatAI Runtime/API/models/worker 已删除；`/api/v1/chat-ai/` 下只保留持久化并返回组件签名 opaque Runtime Journal 的 `runtime-store/`。Core 不执行 Kael Runtime，Magnus、Lion 和 Koko 的执行边界不变；
- Koko agentd 的能力和业务流量迁入 Kael，不代表本期删除 Koko 中的旧代码。

长期允许 Lina 成为 Platform Agent Host，Magnus、Lion 或其它组件成为新的能力来源。替换 Capability Provider 不得改变 Kael 的 Conversation、Run 或 Event 协议。

### 3.3 信任边界

系统至少包含以下信任域：

| 信任域 | 信任程度 | 处理原则 |
|---|---|---|
| Browser renderer | 不可信客户端 | 请求、Context、Registration 和 ToolResult 全部校验 |
| Electron renderer | 不可信客户端 | 不允许直接提供 Authorization 或 Cookie |
| Electron main process | 受控身份代理 | 注入最新 Bearer、组织和时区，隔离窗口流 |
| 同源入口/身份适配器 | 可信入口 | 验证身份并生成 Kael 可消费的 Principal |
| Kael Runtime | 可信编排层 | 不保存资源凭据，不越过执行端权限 |
| Core/Koko/Chen/本地执行器 | 最终执行权威 | 每次执行重新校验 RBAC、ACL 和资源状态 |
| Model Provider | 外部不可信处理方 | 只发送允许的数据，输出和 Tool Arguments 必须校验 |

## 4. 组件职责与所有权

### 4.1 组件职责

| 组件 | 必须负责 | 禁止负责 |
|---|---|---|
| Kael API | 身份边界、DTO 校验、`/kael/api/v1`、SSE、幂等入口 | 具体业务工具执行 |
| Kael Runtime | Context Builder、模型路由、Agent Loop、Run 状态和工具决策 | 解析 MCP、调用 Koko/Chen/Core 业务 API |
| Kael Capability Broker | Registration 校验、精确路由、等待结果、取消传播 | 根据工具名或用户 ID 猜 executor |
| Kael Store/Event | 权威状态、事务、Outbox、事件恢复和审计关联 | 保存浏览器 executor 句柄 |
| Luna AI Panel | UI、Context、Registry、Tool Dispatcher、Approval 和 Renderer | 成为 Conversation/Run 权威存储 |
| Luna Adapter | Platform、MCP、本地能力与通用协议互转 | 把产品协议泄漏进 Kael Runtime |
| Lina AI Panel | Kael 原生 Conversation/Message/Artifact/前台 Run、Approval、audit UI；直接消费 PanelDelivery | iframe/embed、旧 DTO/SSE 映射、background/Web Search/服务端 STT 请求或旧 stats 面板 |
| Koko/Chen | 真实 Session 执行、连接凭据、ACL 和既有审计 | 承担新的通用 Agent Loop |
| Core | Platform 业务数据、最终 RBAC、Serializer 和后台业务 Job | 相信 AI Registration 代替授权 |
| Model Provider | 生成文本或结构化 ToolCall | 获得用户凭据或直接访问业务组件 |

### 4.2 权威所有权

| 对象或责任 | 权威所有者 | 说明 |
|---|---|---|
| 用户与组织身份事实 | 可信入口，Kael 保存验证后的 Principal | 客户端 user/org 字段不能自证身份 |
| Conversation、Message、Artifact 元数据 | Kael Store port；默认 Journal 由 JumpServer Core 保存 | Panel 本地状态只是视图缓存；Core 只保存 Kael opaque Journal，不导入遗留数据库表或数据 |
| Run、Step、ModelCall、DomainEvent、PanelDelivery | Kael | 旧 agentd 或旧 Platform runner 不能同时推进同一 Run |
| PanelSession、lease、连接 ownership | Kael；活动连接由具体 Kael 实例持有 | 不能退化为“最近活跃 Tab” |
| Context 原始采集 | Luna | Kael 保存经校验的版本化快照 |
| Registration 定义、版本和租约 | Kael | executor 函数不上传到 Kael |
| 本地 Registration 路由表 | Luna | 绑定本地 client key、资源会话和 adapter |
| ToolCall 与 Approval 编排 | Kael | Luna 只执行已校验的调用和展示决定 |
| Terminal/File 真实执行 | Koko、Chen 或既有会话层 | Kael 不持有 SSH、SFTP 凭据 |
| DB context/schema/validate | Chen 或既有数据库会话层 | 不代表授权执行任意 SQL |
| SQL proposal apply | Luna 编辑器本地动作 | 只修改编辑器，不直接执行 SQL |
| Script proposal apply | Luna 编辑器本地动作 | draft-only，不保存、不执行 |
| UI action | Luna 本地执行器 | 必须保留目标、revision 和安全校验 |
| Platform 最终业务授权 | Core | Kael Approval 不能替代 Core RBAC |
| 模型凭据和 Provider 配置 | Core TerminalConfig；Kael 组件身份按需读取 | 不写入 Kael 配置，不下发给 Luna |
| 旧开发分支遗留的 Platform 表/数据 | 不属于当前 Runtime | 功能尚未上线且旧 Core models/API 已删除；部署前清理旧 AI 表或重建开发库，不提供迁移或兼容入口 |

### 4.3 禁止依赖

Kael Runtime 核心包不得依赖：

- Luna、Lina、Koko、Chen、Magnus 或 Lion 的业务包；
- MCP SDK 或具体 Session Component 帧格式；
- JumpServer Core 的业务 URL、Serializer 或数据模型；
- SSH、RDP、数据库、SFTP 或浏览器 UI executor；
- 某个产品固定的工具名。

本期已按 ADR 0001 选择 Headless Platform Gateway 承接前台 Platform Capability。它必须是独立 Adapter 或独立部署单元，不能反向污染 Runtime 核心；Lina 默认 `general` 依赖它，因此 Kael 启动时必须装配成功，见 12.3 节和第 19 节。Gateway 启动加载 Core `/api/swagger.json` 时复用 Kael component AccessKey 签名身份，Core 不为此开放匿名 schema；实际 operation 请求仍使用绑定最终用户和组织的 delegation token。

## 5. 产品交互模式

### 5.1 两类对话

| 类型 | 用户语义 | Capability |
|---|---|---|
| 普通对话 | 不绑定当前 Luna 资源会话的长期对话 | 纯模型，或由可信 Assistant/Profile 明确启用的 Platform Capability |
| Luna 能力对话 | 在当前 Luna Panel 中使用动态环境能力 | 只使用本次 PanelSession 注册并被 Profile 允许的 Terminal、File、SQL、Script 等能力 |

普通对话不会因为用户当前打开 SSH、File、SQL 或 Script 页面而自动获得对应能力。Luna 能力对话失去能力时必须显示不可用、等待或失败，不能静默降级为普通回答。

### 5.2 相互独立的维度

| 维度 | 示例 | 作用 |
|---|---|---|
| Conversation Kind | general、capability | 产品分类和历史展示 |
| Assistant/Preset | general、management、asset、session_audit、ops | 模型策略与 Platform 能力组合 |
| Surface | general.chat、session.terminal、session.file | 当前交互环境和审计 |
| Runtime Profile | general、platform.management、terminal、file、sql、script | 服务端可信运行策略 |
| Execution Mode | foreground、background | 决定 Run 的调度、离线、配额、通知和清理策略 |
| Capability Mode | disabled、panel、service | 决定本次 Run 不使用能力、依赖原 Panel，或使用可信 Headless Provider |
| Registration | 某个 Panel 注册的一项具体能力 | 工具定义和精确路由 |

这些维度不能互相替代。URL、页面焦点、Context、Assistant 名称或客户端提供的工具集合都不能单独授予权限。

ADR 0001 后的合法组合：

| Execution Mode | Capability Mode | 语义 |
|---|---|---|
| foreground | disabled | 交互优先调度的普通纯模型 Run，客户端通常实时订阅 |
| foreground | panel | 交互优先调度并使用原 Panel 注册能力 |
| foreground | service | 使用可信 Headless Provider 的交互 Run；执行不依赖 Panel 在线 |
| background | 任意 | 当前非法；虽保留历史错误码 `background_requires_durable_store`，实际需先补齐分布式 claim/ownership、状态同步与安全工具恢复 |

`service` capability mode 已由 [ADR 0001](./adr/0001-headless-platform-gateway.md) 冻结。它只允许服务端可信 Profile 和已装配的 CapabilityProvider 创建，不能由客户端注册或伪装成 Panel 能力。

所有组合的 `POST /runs` 都立即返回 Run 资源；foreground 不表示 HTTP 请求阻塞到 Run 结束，流式结果统一由独立 SSE 订阅获得。

### 5.3 Profile

Profile 由 Kael 服务端配置并版本化，至少固定：

- system policy；
- 允许的模型与 Provider 能力；
- capability namespace 和风险上限；
- Agent Loop 预算；
- Approval policy；
- Context 和 History 限制；
- 输出与 result card 能力。

客户端只能请求自己有权使用的 Profile，不能提交任意 system prompt 或最终工具集合。

## 6. 领域模型与生命周期

### 6.1 对象关系

```text
Principal
  |
  +-- Conversation
        |
        +-- Message -------- Artifact references
        |
        +-- Run
              |
              +-- Step / ModelCall
              +-- ToolCall -- Approval
                     |
                     +------ ToolResult

Conversation
  |
  +-- PanelSession
        |
        +-- ContextSnapshot
        +-- Registration
        +-- PanelDelivery stream

Conversation / PanelSession / Registration / Run / Artifact / Approval
        |
        +-- DomainEvent / OutboxRecord
                    |
                    v
              Event Projector
                    |
                    +-- PanelDelivery --> PanelSession stream
```

### 6.2 一级对象

| 对象 | 语义 | 关键约束 |
|---|---|---|
| Principal | 经可信入口验证的用户和组织身份 | 不能由请求体自报 |
| Conversation | 长生命周期对话容器 | 不保存活动连接或 executor |
| Message | 结构化用户、助手或工具消息 | Message 与 Run 分离 |
| Artifact | 图片、文件和派生文本的受控引用 | 大对象不进入 Event |
| PanelSession | 一个具体 Browser/Electron Panel 实例 | 多 Tab 各自独立 |
| ContextSnapshot | 当前环境的最小化语义数据 | 有 version、digest 和保留策略 |
| Registration | Panel 能力定义及路由声明 | 属于一个 PanelSession，有 lease/revision |
| Run | 一次用户请求的执行实例 | 固定 execution mode、Profile、Context 和 Registry 快照 |
| Step/ModelCall | Agent Loop 内部步骤和模型请求 | 受预算、超时和审计约束 |
| ToolCall | 模型提出、Kael 校验后的能力调用 | 绑定 Run、Panel、Registration 和 arguments digest |
| ToolResult | executor 回传的有界结构化结果 | sequence 递增，重复提交幂等 |
| Approval | 风险操作的受控决定 | 绑定主体、调用、参数和过期时间；重启时未决项统一过期，不续接执行 |
| DomainEvent | 权威状态变化及待发布事实 | 与领域状态同 Store 事务提交 |
| PanelDelivery | DomainEvent 对某个 Panel 的有序投影 | 按 PanelSession 分配 sequence 和 audience |

ExecutionBinding 是 Run 中“本次能力由谁执行”的精确路由概念。panel binding 使用 PanelSession + Registration；ADR 0001 增加 service binding，由服务端可信 CapabilityProvider 提供。两者都不是用户级全局工具集合，也不能相互接管。

### 6.3 Conversation、PanelSession 与 Run

Conversation 的生命周期长于 Tab 和资源连接。用户可以在新的 Panel 中重新打开历史 Conversation，但新的 Panel 不会继承旧 Panel 的 Registration。

每个由 Panel 发起的 Run 必须记录原始 PanelSession，用于：

- 固定 Context 与 Registry 快照；
- 投递本次交互事件；
- 展示 Approval；
- 形成多 Tab 审计边界。

PanelSession ID 与运行依赖必须区分：

- `capability_mode=disabled` 的纯模型 Run 不依赖 Panel executor，SSE 断开后可以继续；
- `capability_mode=panel` 的 Run 只能调用快照中的 Panel Registration，派发 ToolCall 时要求有效 lease；
- 当前所有 `execution_mode=background` 组合都非法；Core-backed Journal 之外还需具备分布式 ownership、状态同步与安全工具恢复后才能重新启用；
- Headless Platform Capability 使用 `capability_mode=service` 和服务端快照，不能伪装成 Panel Run，也不能接受客户端创建 service-scoped Registration。

### 6.4 Run 快照

Run 从 queued 进入执行前，必须原子固定：

- Profile ID 与版本；
- execution mode；
- capability mode；
- Context version 与 digest；
- Registry revision；
- 可见 Registration ID、definition version 和 digest；
- model routing 与 fallback policy；
- Approval policy；
- 输入 Message 与 idempotency identity。

Panel 后续切换页面、更新选区、刷新 manifest 或改变 Profile，只影响后续 Run。

### 6.5 状态机

核心状态：

| 对象 | 状态 |
|---|---|
| Conversation | active、archived、deleted |
| PanelSession | creating、active、disconnected、expired、closed |
| Registration | pending、active、expired、revoked、superseded |
| Message | pending、streaming、completed、failed、cancelled |
| Run | queued、running、waiting_capability、waiting_approval、cancelling、completed、failed、cancelled、interrupted |
| ToolCall | created、waiting_approval、dispatched、running、succeeded、failed、cancelled、timeout、unknown |
| Approval | pending、approved、rejected、expired、cancelled、consumed |
| Artifact | uploaded、validated、attached、quarantined、deleted |

状态约束：

- Run 的 completed、failed、cancelled 是终态；interrupted 是显式可恢复状态；
- queued 只能由获得执行权的 Worker 进入 running；
- interrupted 只能由显式 resume 在重新校验身份、Profile 和执行绑定后回到 queued；
- waiting_capability 只能在原执行绑定恢复后继续，超过配置期限后以 capability timeout 失败；
- waiting_approval 只能由有效且未过期的决定继续；
- cancelling 必须向未完成 ToolCall 传播取消并最终收敛；
- PanelSession 的 expired、closed 不可恢复；disconnected 只能在 lease 窗口内经 resume 回到 active；
- Registration 的 expired、revoked、superseded 对该 revision 不可逆；
- Approval 只能消费一次；approved 后由执行 claim 原子进入 consumed，执行失败记录在 ToolCall/Run，不回退 Approval；
- Run 在 consume 前取消时，未消费 Approval 同步进入 cancelled；
- ToolCall 的 unknown 只能通过原 invocation 查询或人工处置解析，不能被当作 failed 后重新调用；
- 状态变化和对应 DomainEvent/OutboxRecord 必须在同一事务边界提交。

## 7. Agent Loop

### 7.1 标准流程

```text
User Message
    |
    v
Create Run -> Claim -> Freeze Snapshot
    |
    v
Build Model Context
    |
    v
Call Model
    |
    +---- Final output --------------------------+
    |                                            |
    +---- Tool decision                          |
             |                                   |
             v                                   |
      Validate Registration                      |
             |                                   |
      Apply Policy / Approval                    |
             |                                   |
      Dispatch to exact binding                  |
             |                                   |
      Wait for ToolResult                        |
             |                                   |
      Append bounded result ---------------------+
             |
             v
        Next model turn or terminal state
```

### 7.2 Runtime 不变量

- 每个 Run 有最大轮数、模型请求数、工具数、Context、History、Payload、结果大小和总超时。
- 达到预算时进入不允许新工具的最终生成轮，尽力给出安全的部分回答。
- Tool input/output schema 在进入模型和接收结果时都要校验。
- Provider 不支持原工具名时只能使用可逆、无冲突的安全别名。
- 非标准 Tool Arguments 只允许有限解析和一次受控修复。
- final-result ToolResult 只终止后续工具调用；Runtime 必须再进行一轮禁用工具的模型生成，向用户解释 proposal 及已应用或已拒绝的结果。
- 写操作不自动重试；只读操作只有在请求身份明确时才允许一次安全刷新。
- cancel 必须同时影响 Run、等待中的 ToolCall、Provider 请求和真实 executor。
- Run 的模型、预算与 fallback policy 一旦开始不能因配置热更新而静默改变。

### 7.3 模型抽象

Kael 只保留领域无关的 Model Provider port，不自研 OpenAI 协议 SDK，也不引入决定领域模型的 Agent Framework。模型 Adapter 统一使用官方 `github.com/openai/openai-go`：OpenAI 走 Responses API，OpenAI-compatible 与 DeepSeek 走 Chat Completions；HTTP、鉴权、SSE、工具调用类型和 API 错误解析由 SDK 负责。迁移基线至少需要承接：

- OpenAI Responses；
- OpenAI Chat Completions fallback；
- OpenAI-compatible Provider；
- DeepSeek reasoning、结构化 action 和非 reasoning fallback；
- native tools、structured output 和纯文本模式；
- reasoning effort、previous response、local state 与 compaction；
- token/window 归一化、代理、TLS、超时和安全错误分类。

官方 SDK 必须隔离在 Adapter 内，不能进入 domain/application 接口。代理、TLS 和超时通过注入 SDK 的 HTTP client 配置；SDK 内建重试关闭，由 Runtime 的请求预算和只读重试策略统一控制。

### 7.4 关键调用时序

普通对话：

```text
Luna -> bootstrap -> Conversation -> PanelSession
     -> Message -> Run(disabled) -> Model
     <- PanelDelivery/SSE <- DomainEvent
```

Luna 能力对话：

```text
Luna discovers Koko/Chen manifest
  -> normalize Context + Registration
  -> create Run(panel) and freeze snapshot
  <- tool.call SSE to the same PanelSession
  -> Luna local lookup -> MCP/local executor
  -> ToolResult HTTP -> Kael -> next model turn
```

Approval 与取消：

```text
Kael persists Approval -> original Panel renders decision
  -> Kael validates binding/digest/expiry
  -> executor revalidates ACL/RBAC -> ToolResult

Run cancel -> tool.cancel to original Panel
  -> Luna MCP/local cancel -> terminal ToolResult
  -> Kael converges Run to cancelled/failed
```

断线、后台与新 Panel：

```text
SSE disconnect -> subscription closes; Run is not implicitly cancelled
Panel reconnect -> verify Principal/resume token -> replay same PanelDelivery stream
New Panel -> read Conversation/Message/Run/Approval snapshots -> start a new stream
New Panel never takes over an old panel-local invocation
```

## 8. Context 与 Capability

### 8.1 Context

Context 由 Luna 的语义 Adapter 从当前页面和资源会话生成，而不是上传整个前端 Store。Context 必须：

- 最小化、结构化、版本化；
- 有大小、字段、敏感级别和保留限制；
- 明确 surface/domain；
- 只包含模型完成任务所需的显示信息和有限快照；
- 在 Run 开始时冻结；
- 被当作不可信数据引用，不能覆盖 system policy。

禁止进入 Context、Conversation、模型请求或普通日志的内容包括：

- 密码、私钥、数据库凭据；
- Cookie、Bearer、ConnectToken；
- 证书私密材料；
- 无边界终端、文件或数据库内容；
- Luna Store 的无关状态；
- Koko/Chen executor 句柄。

### 8.2 Registration

Registration 至少包含：

- server-generated ID 和 PanelSession ID；
- Luna 本地稳定 client key；
- name、description、input/output schema；
- definition version 与 digest；
- risk、requires confirmation；
- read-only、destructive、open-world、idempotent 等注解；
- registry revision；
- lease、expires time 和状态。

Renderer 提交的风险与重试注解不可信。Kael 必须维护由 Profile 和受控 capability catalog 决定的最低安全策略：

- 客户端只能提高 risk、增加 confirmation 或收紧能力，不能降低服务端策略；
- destructive、idempotent、open-world、read-only 等影响执行和重试的属性以服务端策略为下限；
- 未知工具默认拒绝；仅当 Profile 显式允许某个动态 namespace 时，才按 dangerous、requires confirmation、non-idempotent、open-world 的保守默认值注册；
- 不能因为 schema 合法就自动接受动态 namespace；
- 将来若使用 manifest 签名或 attestation，只能增强来源可信度，不能取消执行端二次权限校验。

Luna 的 manifest 更新采用原子 Registry 替换：

1. 提交 base registry revision；
2. Kael 校验全部 schema、命名空间和风险注解；
3. 全部成功后生成新 revision、Registration ID、digest 和 lease；
4. 任一项失败则保留上一完整 revision。

### 8.3 精确路由

Luna 本地路由必须形成：

```text
registration_id
  -> panel_session_id
  -> local client key
  -> resource session / pane / revision
  -> executor adapter
```

禁止使用 user ID、organization ID、tool name、页面焦点或最近活跃 Tab 代替这条绑定。

ToolCall 发出前，Kael 必须实时复验：

1. Run 的 capability mode 允许工具；
2. Registration 属于 Run 的 PanelSession；
3. Registration active 且 lease 未过期；
4. definition digest 与 Run 快照一致；
5. Profile 允许该 namespace 和风险；
6. Principal 与组织范围一致；
7. Approval 状态满足策略。

### 8.4 Adapter 边界

对于 Koko/Chen，Luna 负责 MCP manifest、`tools/call`、cancel 和 result 与通用 Registration/ToolResult 的转换。Kael 不解析 MCP `content`、`structuredContent`、`isError` 或 `_meta`。

Platform Tool 必须面向用户意图，例如资产查询、审计汇总或 UI 导航。禁止向模型暴露“任意 Method + 任意 URL”的开放工具。

### 8.5 长任务

浏览器不是后台 Worker。Panel 可以通过语义工具启动 Core 等权威后台 Job，并返回 job ID；Job 被服务端接管后可以在 Panel 关闭后继续。

依赖 Panel 本地 executor 的长 ToolCall 仍受 PanelSession lease 约束。需要浏览器关闭后继续执行的 Platform Agent Loop，必须使用可信 Headless Provider 并先补齐分布式执行语义，不能假装 Luna 仍在线，也不能回退到已删除的 Core ChatAI Runtime。

## 9. API 与事件协议

### 9.1 路径与部署规则

唯一权威业务根路径：

```text
/kael/api/v1
```

规则：

- 不提供其它业务根路径、入口别名或按对话类型拆分的 API；
- Kael 原生处理完整 `/kael` 前缀，开发代理和生产网关都不 rewrite；
- canonical 路径不带尾斜杠；
- 迁移期可以直接兼容尾斜杠，但 POST、上传和 SSE 不能依赖 Redirect；
- 浏览器始终通过对应前端的 site-prefix helper 构造地址；
- `/luna/...` 页面请求 `/kael/api/v1/...`；
- `/tenant-a/luna/...` 页面请求 `/tenant-a/kael/api/v1/...`；
- Lina 页面同样只请求 site-prefix + `/kael/api/v1/...`，禁止旧 `/api/v1/chat-ai/...`、iframe/embed 或任意外部 AI URL；
- 禁止拼成 `/luna/kael/...` 或 `/lina/kael/...`；
- Luna Web/Electron 对 Kael 使用唯一逻辑服务名 `kael`；
- `JMS_KAEL_DESKTOP_URL`、`JMS_KAEL_DEV_URL` 只允许配置不含凭据和业务 path 的 origin，客户端始终追加完整 `/kael/api/v1/...`；
- 生产环境没有专用 Kael origin 时使用当前 JumpServer `session.origin`；
- Electron 不得回退到旧 Agent 或 Platform AI 服务端口；
- 旧业务路径不由 Kael 复制接管，其退出规则属于迁移方案。

运维端点不属于业务 API version：

- `/kael/health/live`；
- `/kael/health/ready`；
- `/kael/health/startup`；
- `/kael/internal/metrics`；
- `/kael/openapi.json`。

### 9.2 资源分组

| 资源组 | Canonical 路径 |
|---|---|
| 协议发现 | `/kael/api/v1/bootstrap` |
| Assistant/Profile | `/kael/api/v1/assistants`、`/kael/api/v1/runtime-profiles` |
| Conversation/Message/Recovery | `/kael/api/v1/conversations`、`/kael/api/v1/conversations/{id}/messages`、`/kael/api/v1/conversations/{id}/runs`、`/kael/api/v1/conversations/{id}/approvals`、`/kael/api/v1/messages/{id}/regenerations` |
| Artifact | `/kael/api/v1/artifacts`、`/kael/api/v1/artifacts/{id}` |
| 服务端 STT（禁用占位） | `/kael/api/v1/transcriptions`；bootstrap `transcription=false`，固定返回 unavailable，Lina 不调用 |
| Panel/Context/Registry | `/kael/api/v1/panel-sessions`、`/kael/api/v1/panel-sessions/{id}/context`、`/kael/api/v1/panel-sessions/{id}/registrations` |
| Run | `/kael/api/v1/runs`、`/kael/api/v1/runs/{id}` |
| Event | `/kael/api/v1/panel-sessions/{id}/events` |
| ToolResult | `/kael/api/v1/tool-calls/{id}/results` |
| Approval | `/kael/api/v1/approvals/{id}`、`/kael/api/v1/approvals/{id}/decisions` |
| Platform 管理 | `/kael/api/v1/admin/platform-registry/refresh`、`/kael/api/v1/admin/stats?days={1..365}`、`/kael/api/v1/admin/audit/conversations`；stats 仅为服务端管理/观测接口，Lina 无 stats 面板 |

完整方法和资源映射见迁移方案的“目标 API”章节。

### 9.3 HTTP 与 SSE

Panel 到 Kael 的命令使用 HTTP：

- 创建或修改 Conversation、Message、Artifact；
- 创建、取消或恢复 Run；
- 创建、续租、恢复或关闭 PanelSession；
- 更新 Context 和 Registration；
- 回传 ToolResult；
- 提交 Approval 决定。

Kael 到 Panel 的事件使用 SSE：

- Conversation、Message 和 Run 状态；
- 文本 delta；
- Registration 和 lease 状态；
- ToolCall、Tool progress 和 Tool terminal result；
- Approval required/resolved；
- stream reset 和安全错误。

SSE `data` 是原生 PanelDelivery。Lina 直接按 `delivery.type` 的 dot 事件名消费该对象，不把它重写为 `message_delta`、`message_done`、`approval_required` 等旧事件，也不维护旧字段别名 DTO。

不需要为了双向通信强制 WebSocket。SSE 连接关闭只取消订阅，不等于取消 Run。

### 9.4 Event 与恢复语义

领域事实与 SSE 投递是两个对象：

- DomainEvent 有全局唯一 event ID、aggregate、type、timestamp、schema version 和有界 payload，并与领域状态在同一 Store 事务中提交；
- PanelDelivery 有 PanelSession ID、该 stream 内的 sequence、event ID、audience 和投影 payload；
- Event Projector 按 PanelSession 原子分配 sequence，并在向 SSE 发布前把 PanelDelivery 提交到 Store；
- 同一个 DomainEvent 可以产生多个 PanelDelivery，各 Panel 的 sequence 相互独立；
- PanelDelivery 是具体 PanelSession 的重放真值；Runtime Journal 可保留它用于审计，但 PanelSession 重启后失效，新的 Panel 不能沿用旧 cursor。

PanelSession 与 Conversation 的绑定规则：

- 一个 PanelSession 只绑定一个 Conversation；
- 打开 Conversation 时创建新的 PanelSession，或用仍有效且属于同一 Principal/org 的 resume token 恢复原 PanelSession；
- 切换 Conversation 时关闭原 PanelSession，再为目标 Conversation 创建或恢复 PanelSession；
- 同一 Conversation 可以被同一 Principal/org 的多个 PanelSession 显式打开；
- 组织切换必须关闭或失效原组织的 PanelSession，不能原地修改其 Principal；
- 新 Panel 不继承旧 Panel 的 Registration、cursor 或未完成本地 invocation。

事件受众：

| 事件 | PanelDelivery audience |
|---|---|
| panel、registration、lease | 对象所属 Panel |
| run.queued/started/waiting、message.delta、model progress | 发起 Run 的 Panel |
| ToolCall、tool progress、完整 ToolResult/terminal detail | 原 execution Panel，禁止向其它 Panel 泄漏终端、文件或数据库结果 |
| panel-scoped approval.required/resolved | 原 execution Panel，禁止转移 |
| message.completed、脱敏 Run terminal、Conversation metadata | 当前仍获授权且显式打开该 Conversation 的 Panel |
| service-scoped Approval | 已按 ADR 0001 启用；可由同一 Principal/org 决定，但只能继续原 service ToolCall 和参数 digest |

交付规则：

- SSE `id` 是 stream sequence 的十进制字符串，sequence 必须保持 JavaScript safe integer；
- 传输语义为 at-least-once，客户端按 `PanelSession ID + sequence` 去重；
- 客户端成功投影后才推进 cursor；
- 重连支持 `Last-Event-ID` 和受控 cursor；
- 建连时 cursor 超出保留期返回 `410` 和机器码 `cursor_expired`；
- 已连接 stream 需要重同步时发送 `stream.reset` 后关闭连接；
- heartbeat 使用无 ID SSE comment，不进入 DomainEvent 或 PanelDelivery sequence；
- 未知事件类型和未知可选字段必须可忽略；
- 代理关闭响应缓冲和缓存，并允许长连接。

新 Panel 打开已有 Conversation 时，先读取 Conversation、Message、Run 和 Approval 的权威快照，再从自己的 stream cursor 接收后续事件。快照必须明确 Approval scope 和当前允许动作；旧 Panel 的 panel-scoped Approval 在新 Panel 中不可执行。新 Panel 不能尝试接管旧 Panel 的本地 ToolCall。

### 9.5 版本策略

版本分为四层：

1. HTTP API major version：`/kael/api/v1`；
2. Event schema version；
3. Runtime Profile version；
4. Registration/Capability definition version。

新增可选字段、新事件类型、新 Profile 或新 Capability 属于兼容扩展。删除字段、改变既有语义、修改状态机或游标行为属于破坏性变更。

## 10. 身份、安全、Approval 与审计

### 10.1 Principal

Kael 只消费经过验证的 Principal，至少包括 subject、organization、authentication source 和 session/token fingerprint。

- Browser 使用同源 Cookie、组织选择 Header 和 CSRF；
- Electron 由 main process 移除 renderer 提供的敏感 Header，再注入当前 Bearer、组织、时区和 Referer；
- Electron renderer 只能提交逻辑服务 `kael` 和 `/kael/api/v1/...` 相对路径，不能指定任意绝对 URL；
- Electron IPC stream handle、chunk 和 subscription abort 必须绑定 `webContentsId`，不同窗口不能互相读取或关闭订阅；
- `POST /kael/api/v1/runs/{id}/cancel` 是独立服务端命令，由 Kael 按 Principal/org/Conversation/Run 和 ExecutionBinding 授权；panel capability Run 的取消仍必须路由到原 execution Panel；
- 组织 Header 只能选择已经获准的组织，不能自证权限；
- 组织切换不修改既有 Principal 或 Run，必须关闭原 PanelSession，并在新组织下重新创建；
- 每次普通请求和 SSE 重连都重新验证 Principal 对 Conversation 和 PanelSession 的所有权；
- Kael 的 Core identity adapter 在每次请求和 SSE 重连时，使用调用方现有 Cookie 或 Bearer 查询 Core profile/permissions；不接受客户端自报 Principal，也不维护第二套身份断言协议。

Panel resume token 必须绑定 Principal、organization、PanelSession 和客户端实例，短期有效、只保存 hash、不放入 URL query，并在成功恢复后轮换。

身份验证方案是 Kael 业务 HTTP/SSE 开始实现前的阻塞 Gate，不是上线后再补的增强项。

### 10.2 权限

AI 永远不获得当前用户之外的权限：

- Platform Capability 使用当前用户的 Core 权限；
- Luna Session Capability 使用当前资源会话权限；
- Profile 决定最大允许能力范围；
- Registration 表示当前可发现能力；
- Kael 在派发前复验绑定与策略；
- Core/Koko/Chen/本地执行器在执行时做最终复验。

权限撤销、Session revoke、资产授权变化或组织切换必须在执行时生效。

### 10.3 Approval

风险至少分为 read、write、dangerous。Approval 必须绑定：

- Principal 和组织；
- Conversation、Run 和 ToolCall；
- 原 ExecutionBinding；panel 为 PanelSession 和 Registration，service 为可信 Provider binding；
- tool definition revision；
- arguments digest；
- risk、preview、policy version 和 expires time。

panel-scoped Approval：

- 只在原 execution Panel 展示和决定；
- 原 Panel 永久失效后必须过期或取消，不能转移本地 ToolCall；
- approve、reject、expire 和 consume 均需幂等；
- 确认不能修改参数；参数变化必须产生新的 ToolCall 和 Approval。

service-scoped Approval 已按 ADR 0001 启用。它可以由同一 Principal/org、同一 Conversation 的授权 Panel 恢复展示，但决定只能继续原 service invocation，不能接管或重放旧 Panel 的本地调用。

Kael Approval 只批准编排层继续调用，不能替代执行端的 ACL/RBAC、命令复核或业务审批。

### 10.4 数据与凭据

Kael 不得把以下内容写入领域数据、Event、日志、审计或客户端输出：

- SSH、数据库、SFTP 和 ConnectToken 凭据；
- 用户 Cookie、Bearer 或 Access Key；
- 模型 API Key；
- 未裁剪的敏感终端、文件、数据库内容；
- system prompt、内部堆栈或模型思维链。

Kael 使用组件签名身份从 Core TerminalConfig 读取模型凭据，只能在 Provider Adapter 的受控内存中使用，不得写入 Kael 配置、Store 或下发给 Luna。日志、Event、审计和错误响应必须按字段分类、裁剪和脱敏。

### 10.5 审计

每次 AI 操作必须能够关联：

```text
Principal
  -> Conversation
  -> Run
  -> ModelCall
  -> ToolCall
  -> Approval
  -> Registration
  -> Executor audit reference
  -> ToolResult
```

Kael 保存编排审计和执行摘要；Koko、Chen、Core 保存其既有执行审计。两者通过稳定 correlation ID 关联，不能重复保存完整敏感结果。

## 11. Store、实例与恢复

### 11.1 状态分类

| 类型 | 内容 | 当前要求 |
|---|---|---|
| Runtime state | Conversation、Message、Run、ToolCall、ToolResult、Approval、DomainEvent、PanelDelivery、审计索引 | 仅通过 Store port；默认以 snapshot/delta Journal 写入 Core |
| Artifact state | 图片、文件和派生内容 | 元数据和有界提取文本进入 Journal；原始文件内容当前仍在 Kael 私有目录 |
| Component identity | Core 签发的 AccessKey | 私有文件、`0600`、不进入 Store/Event/日志 |
| Process-local state | 活动 lease/connection ownership、锁、事件唤醒、SSE connection、Provider 请求、Panel executor channel | Kael 退出后清空；持久实体在启动时收敛为安全终态 |

Kael 当前不连接数据库，也不包含 ORM 或 schema migration。默认 adapter 使用组件 AccessKey 调用 Core `/api/v1/chat-ai/runtime-store/`：读取请求携带一次性 nonce 并验证覆盖 head 与全部有序结果的整页 HMAC receipt；写入使用 commit ID 幂等、请求 integrity、签名 receipt 和 `expected_revision` CAS 追加与本地 JSONL 相同的单行记录。网络/5xx 只以同一 commit ID 有限重试，最终结果不确定或 revision conflict 会 poison 本地 adapter，要求重启重放。`RUNTIME_STORE=jsonl` 是显式回退，此时才写 `data/store/runtime.jsonl` 和 `data/events/<conversation-id>.jsonl`。Runtime、领域对象、HTTP 和 Event 契约不感知具体持久化位置。

### 11.2 事务与 Outbox

- 每次领域状态转换与对应 DomainEvent/PanelDelivery 在同一个 Store 事务中提交；
- 事务失败必须回滚该次内存快照与本次事件归档追加；通知失败不改变已经持久提交的业务事实；
- Event Projector 按 event identity 幂等，并保存每个受众 Panel 的 Delivery；
- PanelDelivery sequence 必须在同一 PanelSession stream 内原子分配；
- Artifact 上传先进入隔离状态，校验成功后才能被 Message 引用；
- Artifact 元数据可通过 `GET /kael/api/v1/artifacts/{id}` 按主体/组织所有权读取；正文读取继续使用 `/content`；
- 删除 Conversation 时必须定义 Artifact 引用、审计和保留策略。

### 11.3 幂等

幂等键至少覆盖：

- Message 创建；
- Run 创建与 cancel；
- ToolResult sequence；
- Approval decision；
- Registry 原子替换；
- 外部非幂等 invocation ID。

相同键和相同 payload 返回同一结果；相同键但 payload 冲突必须拒绝。幂等记录的 scope、TTL 和保留期需在 API 契约中明确。

### 11.4 故障恢复

| 故障 | 收敛行为 |
|---|---|
| SSE 断开 | Run 保持自身语义；客户端按 cursor 重连 |
| Panel 暂时断开 | 纯模型 Run 可继续；Panel capability Run 进入等待 |
| Panel lease 过期 | Registration 不可用；不得转移到其它 Tab |
| Model 请求中断 | 按 Provider 能力恢复，否则 Run 标记 interrupted/failed |
| ToolResult 丢失且调用幂等 | 使用原 invocation ID 查询或安全恢复 |
| ToolResult 丢失且调用非幂等 | 标记 unknown，禁止自动重放 |
| Kael 实例退出 | Conversation 历史保留；活动 Run 标为 interrupted，Panel/Registration/Approval 失效，工具不自动重放 |
| Event 投影或发布失败 | 已提交 Delivery 可在原 PanelSession 有效期内重发，客户端去重 |
| Store 不可用 | 不产生未提交到 Store 的成功 Event |

### 11.5 实例约束

- 单进程内 Worker 通过 claim/lease 保证同一 Run 只有一个推进者；
- PanelSession connection 有唯一 owner，ToolCall 只路由到该 owner；
- 多个 Kael 实例可以读取同一 Core Journal，但当前内存状态不会在实例间实时同步；CAS 只拒绝覆盖，不负责冲突重载或 Run 调度；
- 全量 snapshot 超过记录上限时，当前进程停止继续尝试 snapshot 并追加 delta；这保证小事务继续写入，但 Journal 可能增长，且单个 delta 超限仍会失败；
- 当前生产入口必须设置 `replicas=1`，使用 `Recreate` 或严格的先停旧实例、fencing 后再启动新实例流程；实例粘性不能代替单 writer，开发、预发和生产不得让不同 Kael 同时写同一 Core `default` store；
- 滚动或故障切换会中断活动执行，新实例可从 Core Journal 恢复历史并安全收敛活动状态；Artifact 原始字节仍需重新挂载或迁移原 `data/artifacts` 持久卷；
- 具备分布式 claim、事件唤醒和准确 Panel 路由前，禁止宣称无状态横向扩容、跨实例 Panel 恢复或后台执行。

## 12. Platform AI

### 12.1 目标范围

Platform AI 纳入 Kael，而不是只迁移 Luna 的文本聊天子集。当前 Lina/Kael 产品基线包括：

- Conversation、Message、Artifact 和历史；
- 前台 stream、cancel 和恢复；
- branch、regenerate；
- Approval、result card/activity；
- quota、cleanup、服务端 stats API 和脱敏 audit；
- general、management、asset、session_audit、ops Assistant/Profile。

Lina 未指定 Profile 时使用 `general`，其语义是旧统一 JumpServer Assistant：产品问答加与旧源码默认 allowlist 等价的编译期固定范围内的授权 Core 搜索/调用。Kael 不读取旧 Chat AI operation IDs、paths/tags 或 method policies 的部署自定义配置，生产切流必须先比较并显式处置差异。`management` 仍是管理员专用的更宽静态权限范围；asset、session_audit、ops 保持各自更窄范围。Platform Gateway 默认且必须启用，并配置与 Core 匹配的 delegation secret；关闭 Gateway 或缺少密钥时 Kael 启动失败，而不是带着不可用的默认 Assistant 进入 ready。

旧 Lina 曾包含 iframe/embed、background、Web Search、服务端 STT 和 stats 面板；这些兼容面已从当前 UI、状态和请求层删除，不是本期必须恢复的产品能力。Kael bootstrap 继续把 background、Web Search、服务端 STT 和通知标记为不可用，`/transcriptions` 仅为固定 unavailable 的占位端点。浏览器原生 `SpeechRecognition` 保留在 Lina，它既不上传音频也不经过 Kael。旧文档中的 Scheduled Report 没有对应实现，同样不属于迁移基线。

### 12.2 Platform Tool

Platform Capability 应被设计成少量语义工具，而不是把全部 REST endpoint 机械转换为模型工具。每项能力必须明确：

- 用户意图和适用 Profile；
- input/output schema；
- read/write/dangerous 风险；
- 是否需要 Approval；
- 分页、失败和 result card 语义；
- 当前用户权限和 Core 二次 RBAC；
- 审计摘要与敏感字段策略。

现有 `management` 的动态 OpenAPI 是必须明确处置的迁移能力。当前只由隔离的 Headless Platform Gateway 承接；Gateway 仍必须以 operation ID、可信 Method/URL 构造、allowlist、schema、敏感路径和二次 RBAC 限制模型，不能退化为任意 HTTP 工具。已删除的 Core ChatAI Runtime 不是回退 Provider。

### 12.3 完整迁移的硬约束

以下三项不能同时无条件成立：

1. Kael Runtime core 不连接 Core 业务 API；
2. 原始迁移阶段只修改 Luna 和 Kael；
3. 当前 Platform AI 的前台、后台、持久 Approval、动态 OpenAPI、旧数据和通知一次性全部等价迁移。

仅 Luna 在线 Adapter 无法在浏览器关闭后继续 Core Tool、处理持久 Approval 或执行后台 Agent Loop。

长期推荐边界是：Runtime core 保持业务无关，Platform Capability 由 Lina 或独立可信 Gateway 提供。本期已经在 Kael 仓库增加隔离的 Headless Platform Gateway；该过渡实现必须：

- 使用独立进程或清晰的 Adapter composition root；
- 不被 Runtime domain/application 包导入；
- 只通过通用 Capability Broker 与 Runtime 交互；
- 使用短期、请求绑定、防重放的委托和 mTLS；
- 长期可承担后台 Tool、持久 Approval、动态 Registry 和执行审计；当前按 ADR 0004/0006 只启用前台执行，历史状态写入 Core-backed Journal，活动能力仍为进程绑定；
- 在未来 Lina/Core Gateway 就绪后可独立移除。

ADR 0001 已将领域模型扩展为可承载 Headless Gateway 的通用 CapabilityProvider/ExecutionBinding：

- binding kind 至少区分 panel 与 service；
- Registration 绑定具体 execution binding，而不是把 service 伪装成 PanelSession；
- Run 固定 binding、definition 和 policy 快照；
- service capability 使用独立 broker/worker transport，不通过 Panel SSE 派发；
- service Registration 有自己的身份、权限范围、lease、connection ownership 和审计；
- `capability_mode=service` 与 service-scoped Approval 同时启用，不能只增加一个枚举绕过安全模型。

本期已通过 [ADR 0001](./adr/0001-headless-platform-gateway.md) 选择过渡 Gateway，并引入通用 CapabilityProvider/ExecutionBinding。Core 的旧 ChatAI Runtime/API/models/worker 已删除，只保留 Runtime Store 及组件身份、配置和委托校验边界；具体状态以 [迁移实施状态](./MIGRATION_STATUS.md) 为准。

### 12.4 旧开发数据

Chat AI 尚未上线，不保留旧开发分支的 Conversation、Message、附件、Run、Approval 和 Audit 数据。当前 Core 应用已删除读取或推进这些对象的旧 Runtime、models、API、worker 和对应 migration，也没有迁移、只读或兼容入口。用过旧开发分支的环境必须在部署前删除旧 AI 表或重建开发数据库；Runtime 的唯一历史权威是新的 Core-backed Journal。

## 13. Koko agentd 迁移边界

迁入 Kael 的是通用 Runtime 能力：

- Agent Loop、Run queue、预算和超时；
- Model Provider、fallback 和结构化输出；
- schema、argument repair 和工具名归一化；
- ToolResult、Approval、cancel 和幂等；
- Event cursor、历史、恢复和限制；
- token、时延、错误和审计事件。

不迁入 Kael 的是：

- Koko/Chen 的 MCP executor；
- Terminal、File 以及 DB context/schema/validate 的具体会话协议；
- SQL proposal 和 Script proposal 的 Luna 编辑器 apply 逻辑；
- SSH、数据库、SFTP 连接和凭据；
- Koko/Chen 的 ACL、命令复核和执行审计。

切流后，新 Luna 的 Agent Runtime 请求只能进入 `/kael/api/v1`，同一 Message/Run 不得同时由旧 agentd 与 Kael 执行。旧 agentd 的物理删除和启动开关调整属于允许修改 Koko 后的独立阶段。

## 14. Kael 内部模块

推荐的逻辑模块：

| 模块 | 职责 |
|---|---|
| bootstrap | 配置、依赖装配、生命周期和优雅退出 |
| api | `/kael/api/v1`、SSE、DTO、错误和协议投影 |
| identity | Principal、组织和入口认证适配 |
| conversation | Conversation、Message、Artifact 引用 |
| run | Run Supervisor、状态机、claim、cancel 和恢复 |
| runtime | Context Builder、Agent Loop 和预算 |
| model | Provider interface、能力协商、路由和错误分类 |
| capability | Registration、lease、ToolCall 和 ToolResult |
| policy | Profile、Prompt、risk 和 Approval policy |
| store | 通用 Store/Tx port、事务、idempotency 和可替换 adapter |
| event | 持久 DomainEvent log、SSE projection 和 PanelSession replay |
| audit | 审计关联、摘要和脱敏 |
| observability | structured log、metric 和 tracing |

依赖方向：

```text
transport / provider / storage adapters
                    |
                    v
             application services
                    |
                    v
               domain core

domain core -> ports only
adapters implement ports
```

domain 和 application 不得导入 Gin、具体模型 SDK、数据库驱动、MCP 或 JumpServer 产品包。Web framework、模型 SDK、数据库产品和日志库属于 Adapter/部署选择，不是领域架构。

Kael 当前将 API、Worker 和 Core-backed Store adapter 部署在同一二进制中。Core 只保存 Journal，不提供分布式调度；需要独立扩缩容前，仍必须实现跨实例 ownership、事件唤醒和路由。

## 15. 部署视图

```text
Same-origin Gateway
        |
        +-- /kael/api/v1 --------> Kael component instance
        |                               |
        |                               +--> Run workers
        |                               +--> Core TerminalConfig --> Model providers
        |                               +--> Core Runtime Store API
        |                               +--> Local Artifact storage
        |                               +--> Process-local lease / event wake-up
        |
        +-- /kael/health/* ------> Kael probes
        +-- /kael/internal/* ----> Restricted operations network

Optional transitional deployment:

Kael Capability Broker <----> Headless Platform Gateway <----> Core

Kael component -------- registration / AccessKey / TerminalConfig / heartbeat / Runtime Journal --------> Core
```

部署要求：

- Gateway 保留完整 `/kael` 前缀；
- SSE 关闭 buffering/cache，并设置足够的 read timeout；
- readiness 检查已初始化、未关闭且未 poisoned 的进程内 Runtime Store 及其持久化 adapter；Core 模式在 2 秒超时内执行带签名的轻量 Runtime Store 探测并校验 receipt 与 revision，JSONL 模式检查 journal 可用性；它不检查 Worker 或模型端点；
- liveness 不依赖模型、Core 或其它外部系统，避免重启风暴；
- 首次 `SIGINT`/`SIGTERM` 必须取消 HTTP request context 和 SSE、停止 heartbeat/worker 后再执行有界 Shutdown；信号订阅随即释放，第二次信号保留系统默认的强制退出语义；
- `/kael/internal/metrics` 没有业务用户认证，必须由反向代理或网络 ACL 只开放给监控网络；部署方根据自身容量和 SLO 定义指标阈值及回滚条件；
- Core-backed Store 使用 `replicas=1` 与 `Recreate`/先停后启 fencing；禁止自动切换到没有同一历史的 JSONL，也禁止不同环境共同写全局 `default` store；
- `data/keys` 与 `data/artifacts` 使用服务账号私有持久卷；节点替换时必须重新挂载或迁移 Artifact 卷；
- API、Worker 和可选 Gateway 使用独立最小权限 Secret；
- 外部连接使用 TLS；过渡 Gateway 到 Core 使用明确的 mTLS/委托策略。

## 16. 可观测性与运行限制

每条请求和后台执行至少携带：

- trace ID；
- Principal/organization 的不可逆审计标识；
- Conversation、PanelSession、Run、ToolCall、Approval ID；
- Profile、model、provider 和版本；
- registry/context revision；
- latency、token、retry/fallback 和 terminal status。

核心指标至少包括：

- Run queue、状态、成功率和端到端延迟；
- 首 token 延迟、模型延迟、token 和 Provider 错误；
- ToolCall 等待、失败、取消、unknown 和 Approval 转化；
- Panel lease、重连、cursor replay 和过期；
- Outbox backlog、Worker claim、僵尸 Run 和恢复；
- 用户/组织配额和限流。

日志和 trace 不记录凭据、完整 Context、完整 ToolResult、模型思维链或未脱敏业务响应。

## 17. 兼容与演进

- 新 Provider 通过 Model Adapter 接入，不改变 Run。
- 新产品能力通过 Profile、Context Adapter 和 Registration 接入，不增加新的业务 API 根路径。
- Lina 未来接管 Platform Capability 时，只替换 Provider，不迁移 Runtime 状态机。
- 普通对话未来显式连接能力时，只影响新 Run 快照，不能给既有 Run 隐式扩权。
- Profile、Event 和 Registration definition 分别版本化。
- 客户端忽略未知可选字段和未知事件，不根据中文文案判断状态。
- 破坏资源语义、状态机或 cursor 行为时才升级 HTTP major version。
- Core 的旧 `/api/v1/chat-ai/*` Runtime 路径已经下线，唯一例外是仅供 Kael 组件使用的 `/api/v1/chat-ai/runtime-store/`。
- Koko agentd 删除、Lina/Kael 原生链路验证、Core Gateway 和遗留数据清理分别设置发布 Gate。
- 回滚前先停止创建新 Kael Run，排空或取消在途 Run/Approval，再成对恢复相互匹配的 Lina/Luna/Kael 构建；不能恢复到已删除的 Core ChatAI Runtime，Core Journal 与 Artifact 持久卷必须保留，切换到 JSONL 不等于数据回滚。

未来可以抽取 Luna/Lina 共用的 AI Client SDK，统一 PanelSession、Conversation、SSE、Context、Registration、ToolCall、Approval 和 Event projection；各产品仍保留自己的 Context Adapter、Capability Adapter 和 Renderer。

## 18. 非目标

本文不要求：

- 在 Kael 复制 Koko agentd 的产品耦合代码；
- 在 Kael 执行 Terminal、File、SQL、Script 或 UI 操作；
- 把 MCP 作为 Kael Runtime 协议；
- 将任意 REST Method/Path 暴露给模型；
- 本期修改或删除 Koko、Chen、Magnus 或 Lion，或让 Lina/Core 承担 Runtime 状态机；Core 的 AI 运行时边界只保留 Runtime Store 与既有组件配置/身份/委托支持；
- 首期合并 Luna 现有两套 Panel Controller 和全部 UI 数据结构；
- 为每个 DTO、路由、事件或 Provider 复制大量测试；
- 把目标架构描述成当前已经完成的实现。

## 19. 决策状态

以下边界已由 [ADR 0001](./adr/0001-headless-platform-gateway.md)、[ADR 0002](./adr/0002-core-component-and-store-port.md)、[ADR 0003](./adr/0003-flat-config-and-ablation.md)、[ADR 0004](./adr/0004-jsonl-store-and-event-protocol.md)、[ADR 0006](./adr/0006-core-backed-runtime-journal.md) 和 [迁移实施状态](./MIGRATION_STATUS.md) 冻结；其中标为发布 Gate 的部署选择必须在切流前完成：

1. Platform AI 当前使用过渡 Headless Platform Gateway 承接前台能力；长期仍回到业务无关 Runtime 与正式 Platform Provider 边界；
2. 动态 Core OpenAPI 的前台承接方；后台 Core Tool 和跨重启 Approval 当前禁用；
3. 旧 Platform models/API/worker 已删除且不提供兼容；用过旧开发分支的环境须在发布前清理旧 AI 表/数据或重建开发库；
4. Browser Cookie 和 Electron Bearer 统一由 Kael 的 Core identity adapter 实时校验并转换为 Principal；
5. Model 配置和 API Key 来自 Core TerminalConfig，组件 AccessKey 来自 Terminal registration；
6. 当前默认使用 Core-backed Runtime Journal，不读取 Koko 历史 event，也不自动导入或投影旧 Platform ORM 数据；JSONL 是显式回退，Artifact 原始字节继续使用私有目录；
7. 当前使用单 writer 与 session stickiness；历史可由新实例从 Core Journal 重放，但不支持活动执行、Panel 能力或 Artifact 原始字节的自动跨实例恢复；
8. Event/cursor、Artifact、审计、幂等键和历史的保留期限；
9. worker、lease、retention、Run timeout 和 payload 限制使用 Runtime 安全默认值，不扩大部署配置面；
10. 生产网关、CSRF、Origin、SSE、探针和 metrics 的安全边界；具体网络 ACL 由部署方落实；
11. 灰度维度、观测阈值和构建级回滚条件必须由部署方在切流前定义，仓库不提供臆造的统一阈值；已删除的 Core ChatAI Runtime 不属于回滚组件。

已冻结、不再作为开放项的路径决策是：所有 Kael AI 业务接口统一使用 `/kael/api/v1`。

## 20. 架构守卫

实现和评审必须持续检查：

- Runtime core 没有产品或 MCP 依赖；
- Kael 中不存在具体 Session Tool executor；
- 所有业务 Handler 都位于 `/kael/api/v1`；
- Conversation 和 Run 只有一个权威状态模型；DomainEvent 与 PanelDelivery 只有一条统一投影链；
- Run 快照不可被后续 Panel 状态静默修改；
- ToolCall 只能到原 Registration/ExecutionBinding；panel-scoped 调用只能到原 Panel；
- Context、客户端 risk、user/org 和工具集合都不被当作可信授权；
- 状态与 Outbox 原子提交；
- 非幂等 unknown 不自动重放；
- Browser/Electron 身份和跨窗口流严格隔离；
- Luna/Lina 对旧 Runtime 的业务流量为零；Lina 不存在旧 DTO/SSE adapter、iframe/embed、background、Web Search 或服务端 STT 请求；
- AI 相关架构或协议变化同步更新 `docs`。

测试只覆盖会破坏以上不变量的高风险路径。具体八类跨层测试主题见迁移方案，不按文件或字段机械扩充测试代码。
