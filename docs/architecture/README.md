# 架构边界

当前已存在 Go Server 与 React Web；agent/、protocol/ 只有预留说明。V1.0 规划为 Linux 现场 Agent → WSS → 云端 Server/PostgreSQL，Web 经既有 HTTP API 访问 Server。Agent 计划使用 SQLite。

领域与人工确认、断网约束见 [CONTEXT.md](../../CONTEXT.md)，项目运行与发布入口见 [README.md](../../README.md)。现有 host-collector 是脚手架的宿主机监控采集器，与设备 Agent 无关。

保留 docs/adr/ 的历史决策用于溯源；历史目录名和 Issue 引用不适用于当前可执行路径，当前目录以根 README 和 Taskfile 为准。
