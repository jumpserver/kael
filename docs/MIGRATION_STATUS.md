# AI Runtime 迁移实施状态

日期：2026-09-04

## 结论

Kael 与 Luna 的代码级逻辑迁移已经完成。Kael 按 JumpServer Terminal component 注册为 `kael`，使用 BootstrapToken 获取并保存组件 AccessKey，通过组件身份读取 Core TerminalConfig 中的 `CHAT_AI_*` 模型配置并发送心跳。Kael 不连接 MySQL、PostgreSQL 或其它业务数据库。

Runtime 仍只依赖 `ports.Store`/`ports.Tx` 抽象。当前 adapter 是本地 JSONL Store：Conversation、Message、Run 终态、DomainEvent、幂等索引和审计状态可跨重启恢复；每个 Conversation 另有可读的 `data/events/*.jsonl` 事件归档。它不是共享数据库，PanelSession 的浏览器连接、Registration、运行中的 ToolCall、未决 Approval 和 SSE 连接不会跨进程恢复。未来需要后台执行或多实例共享时仍须新增外部 Store adapter，不得让 Runtime 或领域对象直接依赖数据库驱动。

## 实施矩阵

| 范围 | 状态 | 实现语义 |
|---|---|---|
| Component | 完成 | 与 Koko 相同的 Terminal registration、AccessKey 校验和心跳契约；component type 为 `kael` |
| Model | 完成 | 从 TerminalConfig 读取并定期刷新 `CHAT_AI_ENABLED/METHOD/PROVIDER/BASE_URL/API_KEY/PROXY/MODEL`；本地配置不保存模型 Secret |
| Store port | 完成 | Runtime 只依赖通用 Store/Tx 接口；当前 JSONL adapter 保存事务 journal 与 Conversation event archive，不直接使用数据库 |
| 直接数据库依赖 | 已移除 | 无 DSN、GORM、MySQL/PostgreSQL driver、migration 或领域 ORM 标签 |
| Conversation/Message/Run | 完成 | 历史、终态和幂等索引持久化；重启时未完成 Run 收敛为 `interrupted`，不会自动重放模型或工具 |
| Event/SSE | 完成 | DomainEvent 按 Conversation 持久归档；PanelDelivery 使用 PanelSession 私有 sequence，旧 Panel cursor 不跨重启恢复 |
| Background Run | 禁用 | bootstrap `background=false`；请求 background 返回 `background_requires_durable_store` |
| Panel capability | 完成（进程绑定） | Context/Registry version、lease、准确 PanelSession/Registration binding、Approval、ToolResult、cancel；重启后必须新建 PanelSession 并重新注册工具 |
| Luna 四域 | 完成 | Terminal、File、SQL、Script 保留现有 Koko/Chen/本地 executor；资源凭据不进入 Kael |
| Platform AI | 前台可用 | 隔离 Headless Gateway、动态 OpenAPI、service binding、请求绑定 HMAC、Core 最终 RBAC、脱敏结果卡；Approval 不跨 Kael 重启 |
| Artifact | 完成 | 元数据在 Store，内容保存在私有 Artifact 目录；图片校验、多模态输入和有界文本提取 |
| Web Search/STT/通知 | 未启用 | bootstrap 明确返回 `false`；旧 Lina 能力继续由旧服务承接 |

## 配置与生命周期

- 配置文件和环境变量统一使用 Koko 风格的平铺大写键；核心键为 `CORE_HOST`、`BOOTSTRAP_TOKEN`、`NAME`、`BIND_HOST`、`HTTPD_PORT`、`HTTP_REQUEST_TIMEOUT` 和 `IGNORE_VERIFY_CERTS`，不再维护嵌套配置及同义别名。
- AccessKey 默认保存在 `data/keys/.access_key`，权限为 `0600`；已有 Key 会先通过 Core profile 校验，未注册或已失效时使用 BootstrapToken 注册。
- 模型配置只来自 TerminalConfig，默认每分钟刷新；Core 不可用、Chat AI 被禁用或模型端点不完整时 fail closed。
- Runtime journal 默认位于 `data/store/runtime.jsonl`，Conversation 事件归档位于 `data/events/*.jsonl`；Artifact 与组件 AccessKey 继续使用各自的私有目录。
- Kael 不读取或导入 Koko `data/agent/events/*.jsonl`；本地 JSONL Store 只保存切流后由 Kael 创建的 Runtime 状态和 DomainEvent。
- 一个进程内可以使用多个 worker；不同 Kael 实例不共享本地 JSONL 状态，生产入口仍必须保持实例粘性，且不能宣称跨实例恢复。

## 消融结果

- 删除嵌套配置映射、`KAEL_*` 同义环境变量、可配置 cluster/instance、数据路径和运行时默认参数；数据目录固定从工作目录派生，运行限制由 Runtime 自身默认值管理。
- 删除没有客户端使用的 assertion 身份实现与 Authenticator factory；Browser Cookie 和 Electron Bearer 只通过 Core profile/permissions 校验。
- 保留 Store/Tx port，因为未来外部存储是明确需求；保留 Model Provider 和 Capability Provider，因为它们分别承担模型测试替换与 Luna/Platform 执行边界。
- Model Provider 的领域 port 保留，底层协议实现已消融为官方 `github.com/openai/openai-go` Adapter；删除手写 OpenAI JSON、SSE、工具调用和 HTTP 错误解析，SDK 重试关闭并继续受 Runtime 请求预算约束。
- 保留 Platform Gateway 的隔离边界和安全参数，因为 Luna 的非 general assistant 当前会创建 `service` capability Run，删除它会造成现有功能回退。

## 生产切流 Gate

- Core 已包含 `kael` Terminal component type，并允许组件账户读取 `/api/v1/terminal/terminals/config/` 和提交 component heartbeat。
- 为首次注册配置有效 BootstrapToken，并为 `data/keys` 与 Artifact 目录提供服务账号私有卷。
- TerminalConfig 中 Chat AI 必须启用、method 为 `api`，并提供可访问的 provider、base URL、API key 和 model。
- `/kael/` 保留前缀转发，SSE 禁用 buffering/cache；本地 JSONL Store 部署保持单写实例和实例粘性。
- 普通对话和四个 Luna 能力域各完成一条真实链路；同用户多 Tab、Approval、cancel 和进程内断线重连通过。
- 验证 Kael 重启后历史仍可读取、未完成 Run 被标为 `interrupted`、旧 PanelSession/Registration/Approval 已失效且工具没有自动重放；后台 Run 或多实例共享仍须先实现并验证外部 Store adapter。
