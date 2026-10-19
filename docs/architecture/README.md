# 架构边界

当前已存在 Go Server 与 React Web；已增加 Agent WSS 接入、独立 Web mock 及 [v1 契约](../../protocol/v1.md)，已完成协议／SQLite／浏览器验证。V1.0 为 Linux 现场 Agent → WSS → 云端 Server/PostgreSQL，Web 经业务 HTTP API 访问 Server。真实 Agent 的设备驱动及 SQLite 现场存储仍为计划。

Phase 2A 前端界面与流程模拟已实现，采用独立的浏览器测试数据和演示状态，不新增正式业务 API 或设备通信；实际验证见 [交付记录](../requirements/phase-2a-delivery.md)。范围见 [Spec](../requirements/business-v1-ui-scope.md)，任务顺序见 [阶段计划](../requirements/phase-2a-plan.md)；上述现场架构仍为后续目标。

领域与人工确认、断网约束见 [CONTEXT.md](../../CONTEXT.md)，项目运行与发布入口见 [README.md](../../README.md)。现有 host-collector 是脚手架的宿主机监控采集器，与设备 Agent 无关。

下一阶段已确认计划为服务端测试业务闭环及现有前端接入，真实设备另行安排。当前 B01–B07 服务端测试闭环及前端接入已实现，SQLite／真实浏览器完成验证，PostgreSQL 集成受 Docker 环境限制尚未验证，真实设备与正式业务仍未实现，实际完成范围及验证见 [实施记录](../requirements/backend-business-loop-delivery.md)；设计依据见 [确认范围](../requirements/backend-business-loop-spec.md)、[数据库与接口设计](backend-business-loop-design.md)、[分批实施计划](../requirements/backend-business-loop-plan.md)。

保留 docs/adr/ 的历史决策用于溯源；历史目录名和 Issue 引用不适用于当前可执行路径，当前目录以根 README 和 Taskfile 为准。
