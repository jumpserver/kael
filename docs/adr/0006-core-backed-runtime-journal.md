# ADR 0006：由 JumpServer Core 持久化 Runtime Journal

## 状态

已接受，2026-09-04。本文替代 ADR 0004 中“本地 JSONL 是默认 Runtime Store”的实现选择；ADR 0004 继续定义 JSONL 记录格式、状态恢复和 Event/PanelDelivery 语义。

## 背景

Kael 的 Runtime 与领域层已经只依赖 `ports.Store`/`ports.Tx`，`Memory` Store 在提交内存状态前通过 `statePersistence` 持久化事务 delta。Core 只需要持有用户可见的 AI 问答历史；Run、Panel、Context、Registration、Event/Delivery 等执行状态属于当前进程，不应随完整历史快照长期进入业务数据库。Terminal AI 因会话执行恢复需要，使用独立本地 JSONL 保存完整运行态。

Kael 仍不得直接连接 JumpServer 数据库，也不得把 Django ORM、数据库 driver 或业务模型带入 Runtime。Core 已经是组件身份、模型配置和业务数据的信任边界，因此 Runtime Journal 通过组件签名 API 写回 Core。

## 决策

- `RUNTIME_STORE` 默认值为 `core`。`jsonl` 只用于本地开发或预先规划的隔离环境，不是旧 Runtime 兼容入口或 Core 故障回退。
- Kael 使用现有 Terminal component AccessKey 调用 `/api/v1/chat-ai/runtime-store/`，不新增数据库凭据或存储 Secret。
- Core 保存的 `record` 是 ADR 0004 的单行 `journalRecord`：包含版本、创建时间、base64 编码 payload 和 SHA-256 checksum。payload 只包含已有用户问题的 Conversation、用户 Message、非流式 Assistant Message（含结果卡片）及这些 Message 引用的 Artifact；Core 验证传输外壳，不解释其中的 Go `gob` snapshot/delta。
- 加载使用带一次性 UUID `nonce` 的 `GET /api/v1/chat-ai/runtime-store/?after=<revision>&limit=<1..1000>&nonce=<uuid>` 分页重放。响应为 `{nonce,revision,results:[{revision,commit_id,snapshot,record}],has_more,receipt}`；Core 使用当前请求 AccessKey 对 nonce、查询游标、head、分页标记和所有有序 record digest 签发整页 HMAC receipt，Kael 验签后才解码。这样 AccessKey 轮转不要求改写历史记录，也能拒绝页面截断、重排或旧响应重放。当 cursor 早于最近 snapshot 时，Core 从该 snapshot 开始返回。
- 提交使用 `POST /api/v1/chat-ai/runtime-store/`，请求为 `{commit_id,expected_revision,snapshot,record,integrity}`。`integrity` 是当前 AccessKey secret 对 store key、commit ID、revision、snapshot bit 和精确 record SHA-256 的 HMAC。Core 以 commit ID 幂等、以 expected revision CAS 原子追加，成功或同一提交重试均返回 `201 {revision,commit_id,receipt}`；Kael 必须验证回执 HMAC、commit ID 和准确的下一 revision。revision 冲突返回 `409 runtime_store_revision_conflict`。
- Kael 继续在 `Memory.Transaction` 内生成完整 next state，但 Core adapter 只比较并提交用户可见历史投影。纯运行态事务不会产生 Core record；历史投影提交失败时该事务仍 fail closed，不发布内存状态或 SSE 通知。
- 每次事务只生成一个 commit ID；网络错误和 5xx 使用完全相同的请求做有限短重试。最终结果仍不确定或发生 revision conflict 时，本地 adapter 进入 poisoned 状态，readiness 和后续写立即失败，必须重启并从签名 Journal 重放后恢复，不能猜测提交结果。
- Core 正常运行只追加历史 delta，不再周期性生成 snapshot 或删除早期 delta；Kael 重启时分页加载并顺序重放全部保留记录。记录数和启动重放时间因此随问答历史持续增长，容量与启动耗时必须单独监控。
- Kael 在发送前把单条 Core history record 限制为 8 MiB，避免超过常见 MariaDB `max_allowed_packet` 配置；超限事务直接失败且不 poison adapter，Core 不会收到该记录。
- 模型流式 `message.delta`、Run、Tool、Event 和 Delivery 变化只在进程内 Store 中提交；Assistant Message 进入终态后，其最终正文、失败摘要和结果卡片一次性进入 Core 历史投影。
- Core 重启恢复只恢复用户可见历史，不恢复或重放未完成 Run、ToolCall、Approval、PanelSession/Registration。Terminal AI 继续按 ADR 0004 从独立本地 JSONL 恢复完整运行态并执行安全收敛。
- 载入旧版 Core Journal 时，Kael 先迁移其中的 Terminal AI 数据，再立即提交一条精简历史 snapshot，替换此前包含完整运行态的记录。

## 数据边界

- Core Runtime Store 保存的是 Kael 权威问答历史 Journal，不投影到已经删除的 `chat_ai_conversation`、`chat_ai_message`、`chat_ai_agent_run` 等旧 Django 模型。历史查询通过 Kael API 读取；仅在当前进程存在的运行详情不会跨重启查询。
- 旧 Platform Conversation 和 Koko `data/agent/events/*.jsonl` 不导入新 Journal。旧 Platform Runtime/API/models/worker 和对应 migration 已删除；本功能尚未上线，不提供旧历史迁移、只读入口或兼容写入口，使用过旧开发分支的环境应清理旧 AI 表或重建开发数据库。
- 被持久 Message 引用的 Artifact 元数据和有界提取文本属于历史 Journal；原始 Artifact 文件内容仍由 Kael 的私有 `data/artifacts` 目录管理。组件 AccessKey 仍位于 `data/keys/.access_key`。
- Core-backed Journal 只解决问答历史的持久化位置问题，不提供分布式 Run ownership。当前所有 Kael component account 都使用同一个 `default` store，必须只有一个受控活动写入者；CAS 冲突会拒绝并行写入。
- opaque global Journal 当前不会因 Core 业务 user/org 被删除而级联，也没有按 user/org 分片的物理 purge 或 retention。Conversation `DELETE` 仍是软删；其 Message/Artifact 历史会保留到后续明确实现可验证 purge，不能把 UI 不可见等同于数据已删除。
- 当前 readiness 检查进程内 Store 状态及持久化 adapter；Core 模式会在 2 秒超时内对当前 revision 之后执行带签名的单条轻量查询，校验整页 receipt 并确认 Core head 未与本地分叉，JSONL 模式检查 journal 仍可用。它不检查 Worker 或模型端点；提交路径仍 fail closed，运维侧还需监控 Kael 写入错误。
- `/kael/internal/metrics` 暴露 `kael_runtime_store_snapshot_disabled`、`kael_runtime_store_revision` 和 `kael_runtime_store_records_since_snapshot`。Core 模式固定报告 snapshot disabled，并以 records counter 反映需要重放的历史 delta 数；该端点没有业务用户认证，必须由反向代理或网络 ACL 只开放给监控网络。

## 后果

Conversation、用户问题、终态回答、结果卡片、Message 幂等信息和关联 Artifact 元数据默认保存在 JumpServer Core，可在 Kael 更换节点后重新加载。Run、Context、Registration、Model/ToolCall、ToolResult、Approval、DomainEvent、PanelDelivery 和运行审计仅存在于当前进程；Kael 重启会中断而不是恢复进行中的非 Terminal 任务。Artifact 原始字节仍要求复用 `data/artifacts` 私有卷。

本地 JSONL 仍可用于本地开发或预先规划的隔离环境，但两个 adapter 不是双写关系。禁止在 Core 故障时自动切到 JSONL；切换 `RUNTIME_STORE` 前必须停止写入并确认目标存储已有所需历史，避免数据回退或形成两个并行写权威。
