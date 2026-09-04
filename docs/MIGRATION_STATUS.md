# AI Runtime 迁移实施状态

日期：2026-09-04

## 结论

Kael、Luna 与 Lina 的代码级逻辑迁移已经完成。两类前端都只使用 Kael 原生 `/kael/api/v1`，不把旧 `/api/v1/chat-ai` 作为运行时回退。Lina 按 Message -> Run -> PanelSession SSE 的原生资源流程工作，并直接消费 `message.delta`、`tool.call`、`approval.required`、`run.completed` 等 dot 命名的 PanelDelivery，不再保留旧 DTO/SSE 映射或 iframe/embed 入口。Kael 按 JumpServer Terminal component 注册为 `kael`，使用 BootstrapToken 获取并保存组件 AccessKey，通过组件身份读取 Core TerminalConfig 中的 `CHAT_AI_*` 模型配置并发送心跳。Kael 不连接 MySQL、PostgreSQL 或其它业务数据库。

Runtime 仍只依赖 `ports.Store`/`ports.Tx` 抽象。默认 adapter 通过组件签名调用 JumpServer Core Runtime Store API，以 CAS 追加与本地 JSONL 相同的 snapshot/delta Journal；Conversation、Message、Run 终态、DomainEvent、幂等索引和审计状态可跨 Kael 节点恢复。Artifact 原始字节不在 Journal 中，替换节点仍需挂载或迁移原 `data/artifacts` 卷。`RUNTIME_STORE=jsonl` 仅作为预先选定的本地回退，不是 Core 故障时的自动 failover。Core-backed Journal 不是分布式执行协调器，PanelSession 的浏览器连接、Registration、运行中的 ToolCall、未决 Approval 和 SSE 连接仍不会跨进程恢复。

## 实施矩阵

| 范围 | 状态 | 实现语义 |
|---|---|---|
| Component | 完成 | 与 Koko 相同的 Terminal registration、AccessKey 校验和心跳契约；component type 为 `kael` |
| Model | 完成 | 从 TerminalConfig 读取并定期刷新 `CHAT_AI_ENABLED/PROVIDER/BASE_URL/API_KEY/PROXY/MODEL`；仅 `CHAT_AI_ENABLED` 控制功能启停，本地配置不保存模型 Secret |
| Store port | 完成 | Runtime 只依赖通用 Store/Tx 接口；默认由 Core 保存 opaque Journal，Kael 不直接使用数据库；本地 JSONL 为可选回退 |
| 直接数据库依赖 | 已移除 | 无 DSN、GORM、MySQL/PostgreSQL driver、migration 或领域 ORM 标签 |
| Conversation/Message/Run | 完成 | 历史、终态和幂等索引持久化；重启时未完成 Run 收敛为 `interrupted`，不会自动重放模型或工具 |
| Event/SSE | 完成 | DomainEvent 随 Runtime Journal 持久化；`message.delta` 约 50ms/1KiB 合并并在模型/工具/Run 边界 flush；PanelDelivery 使用 PanelSession 私有 sequence，Lina 直接消费其 dot 事件名，不经过 Legacy event adapter；旧 Panel cursor 不跨重启恢复；本地 JSONL 回退另写 Conversation event archive |
| Background Run | 禁用 | bootstrap `background=false`；Lina 已删除入口、状态和请求；Kael 仍以历史错误码 `background_requires_durable_store` 拒绝手工提交的 background Run，实际尚缺分布式 claim/ownership、状态同步和安全工具恢复 |
| Panel capability | 完成（进程绑定） | Context/Registry version、lease、准确 PanelSession/Registration binding、Approval、ToolResult、cancel；重启后必须新建 PanelSession 并重新注册工具 |
| Luna 四域 | 完成 | Terminal、File、SQL、Script 保留现有 Koko/Chen/本地 executor；资源凭据不进入 Kael |
| Platform AI | 前台可用（启动必备） | Lina 默认 `general` 是旧统一 JumpServer Assistant：只搜索与当前源码中旧默认 operation allowlist 等价的固定范围、权限元数据可静态解析且当前用户拥有全部 required permissions 的 Core API；隔离 Headless Gateway、service binding、请求绑定 HMAC、Core 最终 RBAC、脱敏结果卡；Approval 不跨 Kael 重启 |
| Artifact | 部分迁移 | 元数据和有界提取文本进入 Core-backed Journal，元数据可由 `GET /kael/api/v1/artifacts/{id}` 按所有权读取；原始文件内容目前仍保存在 Kael 私有 Artifact 目录，尚未迁入 JumpServer 文件存储 |
| Web Search/服务端 STT/通知 | 已从 Lina 移除 | bootstrap 明确返回 `false`；Lina 已删除 Web Search、服务端 STT 和通知的 UI、状态与请求代码，不回退旧 Runtime；浏览器支持时仍可使用浏览器原生 `SpeechRecognition`，它不经过 Kael |

Kael 的管理员 stats API 仍接受 `days=1..365`（默认 30），保留 flat usage 计数并返回 `model_calls`、按 model call 计算的 `average_model_duration_ms` 及最多 10 项 `top_operations`；它是服务端管理/观测接口，Lina 已删除旧统计面板和对应请求。Lina 的 Conversation audit 页面直接读取 Kael audit 资源；会话保存创建者的姓名/用户名快照用于责任人展示，消息内容和执行结果仍执行审计脱敏。

## 配置与生命周期

- 配置文件和环境变量统一使用 Koko 风格的平铺大写键；核心键为 `CORE_HOST`、`BOOTSTRAP_TOKEN`、`NAME`、`BIND_HOST`、`HTTPD_PORT`、`HTTP_REQUEST_TIMEOUT`、`IGNORE_VERIFY_CERTS` 和 `RUNTIME_STORE`，不再维护嵌套配置及同义别名。
- AccessKey 默认保存在 `data/keys/.access_key`，权限为 `0600`；已有 Key 会先通过 Core profile 校验，未注册或已失效时使用 BootstrapToken 注册。
- 模型配置只来自 TerminalConfig，默认每分钟刷新；仅 `CHAT_AI_ENABLED` 控制功能启停，`CHAT_AI_METHOD`、`CHAT_AI_EMBED_URL` 及 Lina iframe/embed 分支已删除。Core 不可用、Chat AI 被禁用或模型端点不完整时 fail closed。
- 所有 `/kael/api/v1` 用户请求继续要求旧 `chat_ai.use_chatai` 权限（superuser 除外）。Platform Gateway 默认且必须启用，因为 Lina 默认 `general` 依赖它；显式关闭或缺少有效 `PLATFORM_DELEGATION_KEY` 时 Kael 在监听端口前失败，不能出现 ready 但默认会话固定返回 503 的部署。Kael 的 delegation key/ID/issuer/audience 必须分别匹配 Core 对应 `CHAT_AI_*` 配置，其中 secret 去除首尾空白后至少 32 字符；仓库和镜像不提供可工作的默认共享密钥。
- Platform Gateway 默认允许 `GET/POST/PUT/PATCH`，不默认允许 `DELETE`。`general` 使用与当前 JumpServer 源码默认 `CHAT_AI_ALLOWED_OPERATION_IDS` 等价的编译期固定范围；它不会读取生产环境对 operation IDs、allowed/blocked paths/tags 或 method policies 的自定义配置。切流前必须比较现网策略，任何差异都要显式评审并收窄。asset/session_audit/ops 继续叠加各自更窄范围，management 仅管理员可用。所有范围再叠加 method allowlist、敏感路径拒绝、OpenAPI 静态 required-permissions 全量检查；动态权限或缺少权限元数据的 operation 一律不可搜索、不可调用。
- Run 创建时固化 admin flags 与 permission 列表供异步搜索和选择保持同一授权可见性；Core 在实际 delegated request 上仍按当前用户状态和权限实时复核，权限撤销后不能凭旧 Run 快照执行。
- Runtime journal 默认通过 `/api/v1/chat-ai/runtime-store/` 保存在 Core；提交使用 commit ID 幂等、expected revision CAS、请求 HMAC integrity 和签名 receipt，读取使用一次性 nonce 与整页签名 receipt。网络/5xx 以同一 commit ID 有限重试；最终结果不确定或 CAS 冲突会 poison 本地 Store，必须重启恢复。
- 达到 4096 条 delta 后 Kael 尝试提交新 snapshot；全量 snapshot 超过单条记录限制时，本进程降级为继续追加 delta，单个 delta 超限仍拒绝事务。`RUNTIME_STORE=jsonl` 时才写 `data/store/runtime.jsonl` 和 `data/events/*.jsonl`。
- Kael 不读取或导入 Koko `data/agent/events/*.jsonl`，也不导入或投影旧 Platform `chat_ai_*` 数据。Core 中旧 ChatAI Runtime/API/models/worker 已删除，`/api/v1/chat-ai/` 下只保留供 Kael 组件签名访问的 `runtime-store/`；当前没有旧 Runtime 写入口或历史只读兼容入口。本功能尚未上线，因此不提供旧 AI 数据迁移；使用过旧开发分支的环境应在部署前删除旧 AI 表或重建开发数据库。
- 一个进程内可以使用多个 worker；Core Journal 的 CAS 防止并发覆盖，但当前没有冲突重载、分布式 claim 或跨实例 Panel 路由。所有 Kael component account 当前共享 `default` store。生产必须使用 `replicas=1`，采用 `Recreate` 或严格的先停旧实例、fencing 后再启动新实例流程；会话粘性不能代替单 writer。开发、预发和生产不得让不同 Kael 同时指向同一 Core `default` store。
- `/kael/health/ready` 检查进程内 Store 已初始化、未关闭且未 poisoned，并检查当前持久化 adapter；Core 模式会在 2 秒超时内对 `runtime-store` 发起带签名的轻量探测并校验 receipt 与本地 revision 未分叉，JSONL 模式会检查 journal 仍可用。它不检查 Worker 或模型端点。`/kael/internal/metrics` 没有业务用户认证，必须由反向代理或网络 ACL 限制到监控网络。
- 浏览器标准入口是 Lina 站点同源反向代理的 `/kael/`。HTTPS 在网关终止 TLS 时，优先把精确外部 Lina origin 加入 `ALLOWED_ORIGINS`；若外部 origin 无法固定，才在受信网关后设置 `TRUST_FORWARDED_HEADERS=true`，并由网关覆盖客户端提交的 forwarded host/proto。两者都不配置时，外部 HTTPS Origin 会与 Kael 所见内部 HTTP 不匹配并返回 403。Kael 端口不得直接暴露到不可信网络。`ALLOWED_ORIGINS` 不产生 CORS 响应头；直接跨域浏览器部署仍须由外部代理正确处理带凭据 CORS。
- Artifact 元数据和有界提取文本随 Journal 写入 Core；只有原始 Artifact 文件内容继续使用 `data/artifacts`，组件 AccessKey 继续使用 `data/keys/.access_key`，这两个目录均需要服务账号私有持久卷。节点替换或故障切换必须重新挂载或迁移同一 Artifact 卷，否则只能恢复元数据而无法读取原始内容。

## 消融结果

- 删除嵌套配置映射、`KAEL_*` 同义环境变量、可配置 cluster/instance、数据路径和运行时默认参数；数据目录固定从工作目录派生，运行限制由 Runtime 自身默认值管理。
- 删除没有客户端使用的 assertion 身份实现与 Authenticator factory；Browser Cookie 和 Electron Bearer 只通过 Core profile/permissions 校验。
- 保留 Store/Tx port，因为未来外部存储是明确需求；保留 Model Provider 和 Capability Provider，因为它们分别承担模型测试替换与 Luna/Platform 执行边界。
- Model Provider 的领域 port 保留，底层协议实现已消融为官方 `github.com/openai/openai-go` Adapter；删除手写 OpenAI JSON、SSE、工具调用和 HTTP 错误解析，SDK 重试关闭并继续受 Runtime 请求预算约束。
- 保留 Platform Gateway 的隔离边界和安全参数，因为 Lina 默认 `general` 与 Luna 的 Platform assistant 会创建 `service` capability Run，删除它会造成现有功能回退。
- Lina 删除旧 iframe/embed、background、Web Search、服务端 STT、旧 stats 面板、Legacy DTO 和 Legacy SSE adapter；保留附件、branch/regenerate、Approval、audit、结果卡以及浏览器原生 `SpeechRecognition`。
- Core 删除旧 ChatAI Runtime、ORM API、worker 和模型/搜索/转写实现，仅保留 Runtime Store 及 Kael 所需的组件身份、TerminalConfig 和委托校验边界。

## 生产切流 Gate

- Core 已包含 `kael` Terminal component type，并允许组件账户读取 `/api/v1/terminal/terminals/config/` 和提交 component heartbeat。
- 为首次注册配置有效 BootstrapToken，确认 Core 已应用 Runtime Store 数据库 migration，并提供仅允许 Kael 组件访问的 `/api/v1/chat-ai/runtime-store/`；为 `data/keys` 与 `data/artifacts` 提供服务账号私有持久卷。
- TerminalConfig 中 `CHAT_AI_ENABLED` 必须启用，并提供可访问的 provider、base URL、API key 和 model；不再存在 method 或 embed URL 配置。
- 按 `config_example.yml` 为必启用的 Platform Gateway 配置与 Core 完全匹配的 delegation key/ID/issuer/audience，确认 Redis nonce 防重放可用、Kael component AccessKey 可签名读取 `/api/swagger.json` 且 schema 含静态权限元数据，并验证普通用户具有 `chat_ai.use_chatai`；Core schema 不应改成匿名公开。同时对比现网自定义 Chat AI allowlist/policy 与 Kael 编译期范围。只启动模型而未完成这些检查不算旧统一 Assistant 完整切流。
- `/kael/` 保留前缀转发，SSE 禁用 buffering/cache；HTTPS 终止代理必须配置精确 `ALLOWED_ORIGINS`，或在保证覆盖客户端 forwarded headers 后启用 `TRUST_FORWARDED_HEADERS`；`/kael/internal/metrics` 只允许监控网络访问。Core-backed Store 必须以 `replicas=1` 和 `Recreate`/先停后启 fencing 维持单活动写入者，revision 冲突必须触发失败与运维处置。部署方需自行定义指标阈值和回滚条件，当前仓库不臆造统一阈值。
- 发布前确认 Core 路由清单中 `/api/v1/chat-ai/` 只剩 `runtime-store/`，并清理旧开发分支留下的 AI 表/数据。回滚时先停止创建新 Kael Run，排空或取消在途 Run/Approval，再成对回滚相互匹配的 Lina/Luna/Kael 构建；不得回退到已删除的 Core ChatAI Runtime。Core Journal 与 Artifact 卷必须保留，也不得把切换 `RUNTIME_STORE=jsonl` 当作数据回滚。
- 普通对话和四个 Luna 能力域各完成一条真实链路；同用户多 Tab、Approval、cancel 和进程内断线重连通过。
- 资产写入验收至少覆盖一次完整链路：中文创建意图命中 `assets_hosts_create`，先只读解析 Linux 平台 ID，以最小可写 body 生成单次 Approval，确认后 Core 返回 `201` 且资产可查询；不得把 Schema 编译失败或本地参数拒绝包装成“已创建”。
- 验证 Kael 从 Core snapshot/delta 重放后历史仍可读取、未完成 Run 被标为 `interrupted`、旧 PanelSession/Registration/Approval 已失效且工具没有自动重放；后台 Run 或多实例共享仍须先实现并验证分布式 ownership。
