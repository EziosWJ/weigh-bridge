# Agent 通信协议

[协议 v1](v1.md) 定义 Agent 与 Server 的 WSS 消息、HTTP 图片上传、身份配置、采集和打印交互。独立 agent-mock 与未来真实 Agent 使用同一契约。

契约及接入已实现并通过协议／SQLite／浏览器验证；真实设备驱动和现场验收尚未完成。需求与决策见 [Spec](../docs/requirements/agent-mock-issue-spec.md)、[ADR 0015](../docs/adr/0015-agent-owned-simulation.md)。
