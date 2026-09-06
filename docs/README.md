# Kael 文档

本目录保存 Kael 的架构、协议、迁移和验收规范。harness 分支只使用 Codex App Server；请先阅读 ADR 0007 安装固定版本运行时并配置 Responses 模型。

AI 相关实现必须先与这些文档保持一致。任何会改变以下内容的代码修改，都必须在同一次变更中更新对应文档：

- 组件职责或依赖方向；
- Conversation、PanelSession、Run、Registration 等领域对象；
- HTTP、SSE、能力调用或审批协议；
- 普通对话或 Luna 能力对话的用户行为；
- 鉴权、权限、审计、持久化或恢复语义；
- 迁移范围、兼容策略或验收标准。

当前主文档：

- [Kael AI Runtime Architecture](./ARCHITECTURE.md)
- [AI Native Agent Runtime 迁移与演进方案](./ai-native-agent-runtime-migration.md)
- [Platform AI 现状契约与迁移边界](./platform-ai-compatibility.md)
- [迁移实施状态与生产切流 Gate](./MIGRATION_STATUS.md)
- [ADR 0001：过渡 Headless Platform Gateway](./adr/0001-headless-platform-gateway.md)
- [ADR 0002：Core Component 与 Store Port](./adr/0002-core-component-and-store-port.md)
- [ADR 0003：平铺配置与抽象消融](./adr/0003-flat-config-and-ablation.md)
- [ADR 0004：JSONL Store 与 Event 协议](./adr/0004-jsonl-store-and-event-protocol.md)
- [ADR 0005：官方 OpenAI Go SDK](./adr/0005-official-openai-go-sdk.md)
- [ADR 0006：由 JumpServer Core 持久化 Runtime Journal](./adr/0006-core-backed-runtime-journal.md)
- [ADR 0007：Codex Harness 与试用步骤](./adr/0007-codex-harness.md)

阅读顺序：先用 `ARCHITECTURE.md` 确认长期不变量，再用迁移方案安排实现，最后按 Platform AI 契约核对旧能力兼容。

路径约定：Kael 的唯一权威业务 API 根路径为 `/kael/api/v1`，不提供按对话类型拆分的业务别名路径。

文档描述目标架构和迁移约束，不代表未完成能力已经上线。实际完成状态以主文档的迁移清单及代码为准。

本地启动时，Kael 会按顺序读取当前目录的 `config.yml`、`config.yaml`、`.config.yml` 或 `.config.yaml`；也可以通过 `-f` 或 `KAEL_CONFIG_FILE` 指定配置文件。YAML 与环境变量都使用 Koko 风格的平铺大写键，例如 `CORE_HOST`、`BOOTSTRAP_TOKEN`、`NAME`、`BIND_HOST` 和 `HTTPD_PORT`；不支持嵌套配置。模型配置从 Core TerminalConfig 读取。Lina 默认 `general` 依赖 Platform Gateway，因此它默认且必须启用；复制 `config_example.yml` 后必须填写与 Core 匹配、去除首尾空白后至少 32 字符的 `PLATFORM_DELEGATION_KEY`，仓库不提供默认共享密钥，关闭或留空会在监听端口前失败。Runtime 默认通过组件签名调用 Core `/api/v1/chat-ai/runtime-store/` 保存并加载 Journal；设置 `RUNTIME_STORE=jsonl` 才会回退到工作目录下的本地 JSONL。浏览器标准入口是 Lina 站点同源代理的 `/kael/`；HTTPS 在网关终止 TLS 时优先配置精确 `ALLOWED_ORIGINS`，无法固定时才在能覆盖客户端 forwarded headers 的可信网关后启用 `TRUST_FORWARDED_HEADERS`；Kael 端口不得直接暴露，且 `ALLOWED_ORIGINS` 不会启用 CORS。Kael 不要求数据库 DSN，也不读取 Koko 的历史事件文件。Artifact 文件内容目前仍位于 `data/artifacts`，详见 ADR 0006。
