# ADR 0006：由 JumpServer Core 持久化 Runtime Journal

## 状态

已接受，2026-09-04。本文替代 ADR 0004 中“本地 JSONL 是默认 Runtime Store”的实现选择；ADR 0004 继续定义 JSONL 记录格式、状态恢复和 Event/PanelDelivery 语义。

## 背景

Kael 的 Runtime 与领域层已经只依赖 `ports.Store`/`ports.Tx`，`Memory` Store 在提交内存状态前通过 `statePersistence` 持久化事务 delta。此前默认 adapter 把同一记录写到 Kael 本地 `data/store/runtime.jsonl`，导致 Conversation、Message、Run、Approval、DomainEvent 和审计状态依赖单机数据卷，不能满足 AI 问答状态由 JumpServer 持有的部署要求。

Kael 仍不得直接连接 JumpServer 数据库，也不得把 Django ORM、数据库 driver 或业务模型带入 Runtime。Core 已经是组件身份、模型配置和业务数据的信任边界，因此 Runtime Journal 通过组件签名 API 写回 Core。

## 决策

- `RUNTIME_STORE` 默认值为 `core`。`jsonl` 只用于本地开发或预先规划的隔离环境，不是旧 Runtime 兼容入口或 Core 故障回退。
- Kael 使用现有 Terminal component AccessKey 调用 `/api/v1/chat-ai/runtime-store/`，不新增数据库凭据或存储 Secret。
- Core 保存的 `record` 是 ADR 0004 的完整单行 `journalRecord`：包含版本、创建时间、base64 编码 payload 和 SHA-256 checksum。Core 验证传输外壳，不解释其中的 Go `gob` snapshot/delta。
- 加载使用带一次性 UUID `nonce` 的 `GET /api/v1/chat-ai/runtime-store/?after=<revision>&limit=<1..1000>&nonce=<uuid>` 分页重放。响应为 `{nonce,revision,results:[{revision,commit_id,snapshot,record}],has_more,receipt}`；Core 使用当前请求 AccessKey 对 nonce、查询游标、head、分页标记和所有有序 record digest 签发整页 HMAC receipt，Kael 验签后才解码。这样 AccessKey 轮转不要求改写历史记录，也能拒绝页面截断、重排或旧响应重放。当 cursor 早于最近 snapshot 时，Core 从该 snapshot 开始返回。
- 提交使用 `POST /api/v1/chat-ai/runtime-store/`，请求为 `{commit_id,expected_revision,snapshot,record,integrity}`。`integrity` 是当前 AccessKey secret 对 store key、commit ID、revision、snapshot bit 和精确 record SHA-256 的 HMAC。Core 以 commit ID 幂等、以 expected revision CAS 原子追加，成功或同一提交重试均返回 `201 {revision,commit_id,receipt}`；Kael 必须验证回执 HMAC、commit ID 和准确的下一 revision。revision 冲突返回 `409 runtime_store_revision_conflict`。
- Kael 继续在 `Memory.Transaction` 内先生成 next state、成功提交远端 record 后才发布内存状态。远端失败不会产生本地成功事务或 SSE 通知。
- 每次事务只生成一个 commit ID；网络错误和 5xx 使用完全相同的请求做有限短重试。最终结果仍不确定或发生 revision conflict 时，本地 adapter 进入 poisoned 状态，readiness 和后续写立即失败，必须重启并从签名 Journal 重放后恢复，不能猜测提交结果。
- 远端 delta 达到 4096 条时，下一次尝试提交完整 snapshot；后续加载从最新 snapshot 重放。如果全量 snapshot 超过单条记录上限，本进程禁用后续 snapshot 尝试并继续追加可编码的 delta，避免因确定性 snapshot 过大停止所有写入。此降级会增加 Core Journal 长度，需通过容量监控和后续分片/外部快照方案处理；单个 delta 自身超限仍会拒绝事务。
- 模型流式 `message.delta` 在 Service callback 边界按约 50ms 或 1KiB 合并，且在模型轮结束、工具调用前和 Run 终态提交前强制 flush；Store/Event 仍按合并后的原始文本顺序提交，减少逐 token 远端事务。
- 启动恢复规则不变：未完成 Run 收敛为 `interrupted`，活动 ToolCall 取消，未决 Approval 和旧 PanelSession/Registration 过期，不自动重放模型或非幂等工具。

## 数据边界

- Core Runtime Store 保存的是 Kael 权威 Runtime Journal，不投影到已经删除的 `chat_ai_conversation`、`chat_ai_message`、`chat_ai_agent_run` 等旧 Django 模型。管理查询通过 Kael API 读取新状态。
- 旧 Platform Conversation 和 Koko `data/agent/events/*.jsonl` 不导入新 Journal。旧 Platform Runtime/API/models/worker 和对应 migration 已删除；本功能尚未上线，不提供旧历史迁移、只读入口或兼容写入口，使用过旧开发分支的环境应清理旧 AI 表或重建开发数据库。
- Artifact 元数据和有界提取文本属于 Runtime Journal；当前只有原始 Artifact 文件内容仍由 Kael 的私有 `data/artifacts` 目录管理。组件 AccessKey 仍位于 `data/keys/.access_key`。将附件原始内容迁入 JumpServer 文件存储是后续独立工作。
- Core-backed Journal 解决 Runtime 状态的持久化位置问题，不等于分布式 Run ownership，也不迁移 Artifact 原始字节。当前所有 Kael component account 都使用同一个 `default` store，必须只有一个受控活动写入者：生产设置 `replicas=1`，使用 `Recreate` 或严格的先停旧实例、fencing 后再启动新实例流程；会话粘性不能代替单 writer，开发/预发/生产也不得让不同 Kael 同时写同一 Core `default` store。CAS 冲突会拒绝写入，后台 Run、跨实例 Panel 恢复和未决 Approval 续接仍保持禁用。
- opaque global Journal 当前不会因 Core 业务 user/org 被删除而级联，也没有按 user/org 分片的物理 purge 或 retention。当前 Conversation `DELETE` 只是软删 delta，关联 Message/Run/Event/Artifact 元数据仍留在现行状态和后续 snapshot；更早 record 也会保留到 Core 以一次成功 snapshot compaction 替换历史为止。它不是最终合规归档，生产落地前必须明确新 Journal 的保留策略，并后续实现按 store 分片、retention 和可验证 purge；不能把 UI 不可见等同于数据已删除。
- 当前 readiness 检查进程内 Store 状态及持久化 adapter；Core 模式会在 2 秒超时内对当前 revision 之后执行带签名的单条轻量查询，校验整页 receipt 并确认 Core head 未与本地分叉，JSONL 模式检查 journal 仍可用。它不检查 Worker 或模型端点；提交路径仍 fail closed，运维侧还需监控 Kael 写入错误。
- `/kael/internal/metrics` 暴露 `kael_runtime_store_snapshot_disabled`（0/1）、`kael_runtime_store_revision` 和 `kael_runtime_store_records_since_snapshot`。memory/JSONL 模式返回 0；Core 模式反映当前已验签/提交的本地 head、最新 snapshot 后的 delta 数，以及因 snapshot 记录超限而进入的进程内降级状态。该端点没有业务用户认证，必须由反向代理或网络 ACL 只开放给监控网络；告警阈值和回滚条件由部署方结合容量与 SLO 明确定义。

## 后果

Conversation、Message、Run 终态、DomainEvent、幂等索引和审计状态默认保存在 JumpServer Core，可在 Kael 更换节点或本地目录后重新加载。Artifact 原始字节除外：节点替换或故障切换必须重新挂载或迁移原 `data/artifacts` 持久卷，否则只能恢复元数据和有界提取文本。Kael 继续不连接业务数据库，Runtime/HTTP/Event 协议保持不变。

本地 JSONL 仍可用于本地开发或预先规划的隔离环境，但两个 adapter 不是双写关系。禁止在 Core 故障时自动切到 JSONL；切换 `RUNTIME_STORE` 前必须停止写入并确认目标存储已有所需历史，避免数据回退或形成两个并行写权威。
