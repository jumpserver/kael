# Kael 文档

本目录保存 Kael 的架构、协议、迁移和验收规范。

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

阅读顺序：先用 `ARCHITECTURE.md` 确认长期不变量，再用迁移方案安排实现，最后按 Platform AI 契约核对旧能力兼容。

路径约定：Kael 的唯一权威业务 API 根路径为 `/kael/api/v1`，不提供按对话类型拆分的业务别名路径。

文档描述目标架构和迁移约束，不代表未完成能力已经上线。实际完成状态以主文档的迁移清单及代码为准。

本地启动时，Kael 会按顺序读取当前目录的 `config.yml`、`config.yaml`、`.config.yml` 或 `.config.yaml`；也可以通过 `-f` 或 `KAEL_CONFIG_FILE` 指定配置文件。YAML 与环境变量都使用 Koko 风格的平铺大写键，例如 `CORE_HOST`、`BOOTSTRAP_TOKEN`、`NAME`、`BIND_HOST` 和 `HTTPD_PORT`；不支持嵌套配置。模型配置从 Core TerminalConfig 读取。Runtime 通过 Store port 写入工作目录下的 `data/store/runtime.jsonl`，并把每个 Conversation 的 DomainEvent 写入 `data/events/*.jsonl`；不要求或读取数据库 DSN，也不读取 Koko 的历史事件文件，详见 ADR 0004。
