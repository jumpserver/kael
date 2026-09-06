# ADR 0007：Codex harness 替换自研 Agent Loop

## 状态

已接受，2026-09-06，`harness` 分支试用实现。用户明确选择直接替换，不保留旧引擎或双引擎开关。替代 ADR 0005 的模型执行决策，以及架构第 7 节的手写循环与预算语义。

## 职责

- Kael 仍是 Agent 服务：身份、Conversation、Run、审批、业务事件、审计、精确能力路由由 Kael 管理。
- Codex App Server 是唯一推理引擎，负责模型请求、持续工具循环及原生上下文压缩。通过 stdio 双向 JSON-RPC 接入，不暴露 Codex 网络监听。
- Luna 继续注册 Context/Registration，协调 Koko、Chen 和本地执行器并展示结果。已有 Headless Platform Gateway 继续执行 service binding；本次不迁移该业务边界。
- 删除手写 Loop、OpenAI Go SDK adapter 和 Chat Completions fallback。`internal/model` 仅保留配置、消息、usage 和错误值类型。

## 协议与执行

固定 `codex-cli 0.153.2`；启动时检查精确版本，Docker 同样固定 npm 包版本。依赖该版本的 experimental dynamicTools 和 environments 字段，升级需要重新生成/验证协议并执行集成测试。

Luna bootstrap 必须收到 `agent_engine=codex`、`agent_protocol_version=1`。现有 `/kael/api/v1` HTTP、PanelDelivery SSE、工具回执和审批端点保持。`model.requested/completed` 的 `scope=agent_turn` 表示一次完整 harness turn，包含工具和审批等待；不再声称是内部单次模型请求。现有 ModelCall/ModelRequestCount 存储字段在本分支记录 harness turn，值通常为 1；Luna 不将该总耗时分摊到某次工具调用。usage 从 Codex 累积计数扣除本 turn 起点，覆盖该 turn 内所有模型调用。

工具以 `kael_` 安全别名暴露，避免与内置工具重名。每次调用必须匹配当前 thread、turn 和 Registration；在 Kael 重新校验输入和输出 schema，再走原审批和执行通道。同一 callId 的相同重投复用回执，更改参数则失败；同一 turn 内相同写操作不重放。final-result 回执之后拒绝后续业务工具并要求模型总结。取消/超时使等待回调退出并终止对应 Codex 进程，组件仍通过原取消通道接收取消；取消不意味着撤销已执行操作。

## 模型与运行环境

Core TerminalConfig 仍是模型凭据唯一来源；启动及每次 Run 执行前读取配置，正在执行的 turn 不热切模型。`CHAT_AI_BASE_URL` 必须支持 Responses API。DeepSeek 旧 Chat Completions 路径明确拒绝，其它兼容端点由实际 Responses 请求验证，不提供隐式降级。模型密钥只进入子进程环境，不写入参数、配置文件或前端。

每个活动 Panel 的进程使用独立私有 HOME/CODEX_HOME 和空工作目录，不继承用户 Codex 登录、插件、MCP 配置或应用 Secret。线程 `environments=[]`，禁用 shell、unified exec、Code Mode host、浏览器、computer use、联网搜索和 subagents。真实业务操作只能经过注册能力；内置计划、问询、技能目录等辅助工具不等于远程资产执行能力。未集成的问询表单返回空答案，不替用户决定，要求 Agent 在普通聊天中提问；未知 host request 失败关闭。stderr 不直接进入业务错误或日志。

## 上下文与恢复边界

同一用户、组织、Conversation、Panel，工具/模型/策略未变且历史仍为追加关系时，复用同一进程内的 ephemeral Codex thread，只提交新增消息与当前 Context。历史编辑/分支、工具注册快照改变、模型配置变化或上次运行失败会使旧 thread 失效，新建线程从 Kael 业务历史构建上下文。取消或失败后不自动续跑或重放工具。

删除原来的 30 条历史裁剪、20 轮/40 请求循环限制；新建线程输入仍受 4 MiB 上限约束，超限明确报错，不静默丢弃历史。同一 turn 最多处理 128 个动态工具请求，完整 Run 仍受现有总超时约束。内部模型请求数不由 Kael 猜测。

本次试用不新增跨进程推理状态恢复。Core Journal 仍保存业务历史；Codex thread 是进程内执行缓存，不是第二份权威历史。最多保留 16 个 Panel 进程，空闲超过 5 分钟回收，容量满时优先回收空闲进程。回收后下一次提问重建历史，先前压缩状态与内部推理轨迹不会保留。Kael 正常关闭删除本实例私有目录；异常退出可能留下 `data/harness/instance-*`，停机后可清理这些执行缓存。后台任务、多节点续跑和旧审批恢复均不在本次范围内。

## 试用

1. 安装 `npm install -g @openai/codex@0.153.2`，用 `codex --version` 核对。
2. Core 开启 Chat AI，配置支持 Responses 的模型、base URL 和 API key。保留原组件注册和 Platform Gateway delegation 配置。
3. 使用 `CODEX_BINARY` 指定可执行路径，或使用默认 PATH 中的 `codex`。运行 `make run`；同时使用 Luna 的 `harness` 分支。
4. 验证终端只读排查、SQL/脚本 proposal 审批、拒绝审批、停止、多轮追问及断连。真实模型效果和真实 Core/Koko/Chen 联调需要试用环境验收。

不访问真实模型的验证命令：

```sh
go test ./...
KAEL_TEST_CODEX="$(command -v codex)" go test -race ./internal/runtime ./internal/service
```

第二条启动实际 Codex 二进制，但仅连接测试进程提供的本地 Responses fixture，不使用生产 API key。覆盖工具闭环、连续对话和 usage；单元测试覆盖重复写、回执幂等、参数修复、final-result、取消、越界事件和断流。

参考：[App Server](https://learn.chatgpt.com/docs/app-server)、[配置](https://learn.chatgpt.com/docs/config-file/config-reference)。
