# Agent 通信协议

[协议 v1](v1.md) 定义 Agent 与 Server 的 WSS 消息、HTTP 图片上传、身份配置、采集和打印交互。独立 agent-mock 与真实 Agent 使用同一契约；当前目标连接本厂业务服务，见 [ADR 0023](../docs/adr/0023-standalone-site-baseline.md)。

契约及接入已有软件验证；D12 与打印已有部分现场结果，完整设备闭环尚待验收，见[交付索引](../docs/history/README.md)。历史 SQLite 验证不代表当前数据库支持。需求与决策见 [Spec](../docs/requirements/agent-mock-issue-spec.md)、[ADR 0015](../docs/adr/0015-agent-owned-simulation.md)。
