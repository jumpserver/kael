# Kael 文档

本目录只描述当前代码已实现的设计与接口边界。

- [当前架构](./ARCHITECTURE.md)：组件职责、Codex Harness、领域对象、能力与审批、HTTP/SSE、存储及部署约束。
- [命令执行生命周期](./command-execution-lifecycle.md)：Panel 工具调用、命令状态、回执超时、取消与恢复边界。

配置项和默认值见 [config_example.yml](../config_example.yml)，启动装配见 [cmd/kael/main.go](../cmd/kael/main.go)。

修改组件职责、模型执行、领域对象、协议、审批或持久化行为时，同步更新对应设计说明。文档不维护迁移计划、历史契约或尚未实现的目标方案；接口字段和校验以链接的实现为准。
