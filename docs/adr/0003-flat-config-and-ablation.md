# ADR 0003：平铺配置与抽象消融

## 状态

已接受，2026-09-03。

## 决策

- Kael 配置与 Koko 一致，YAML 和环境变量只使用平铺大写键。删除嵌套 section、字段兼容映射和 `KAEL_*` 同义别名。
- AccessKey 与 Artifact 目录固定从进程工作目录的 `data` 派生；model refresh、worker、lease、retention 和 payload limit 使用代码中的安全默认值，不形成额外部署配置面。
- 入站用户身份只使用 Core identity adapter。Browser Cookie 和 Electron Bearer 每次由 Core profile/permissions 校验；删除未被当前客户端使用的 assertion 策略、factory 和相关配置。
- `cluster_id` 固定为 `kael`，`instance_id` 直接使用组件 `NAME`，不再单独配置。

## 消融判定

以下边界不能删除：

- Store/Tx port 是未来接入外部存储的明确扩展点，也是本次需求要求保留的接口；
- Model Provider 是 Agent Loop 的测试替换点，并把官方 OpenAI Go SDK 类型隔离在 Adapter 内；
- Capability Provider 隔离 Luna panel capability 与 Platform service capability；Luna 当前确实会为非 general assistant 创建 service Run；
- Platform Gateway 的委托校验、OpenAPI allowlist 和结果脱敏是安全边界，不是可省略的通用抽象。

本次删除的内容均不改变 `/kael/api/v1`、Luna panel/service 调用、Terminal component 注册、TerminalConfig 模型读取或 Store 接口。
