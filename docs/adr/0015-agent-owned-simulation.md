---
status: accepted
---

# 模拟行为归属 Agent，系统统一处理采集数据

> **2026-10-22 效力：业务规则继续有效，部署及阶段描述按当前基线。** 人工确认、证据及后续重量修订继续适用；旧“断网暂停”指远程依赖背景，现区分互联网与本地链路，本地正常时断互联网可办理。文中未实现描述为当时状态。当前依据见 [ADR 0023](0023-standalone-site-baseline.md)及[基线索引](../development-baseline.md)；下文历史正文保留。

2026-10-19，用户决定将设备模拟迁至带独立 Web 控制台的 agent-mock，与未来真实现场 Agent 共用通信契约。系统不再按模拟／正式来源分成两套处理流程，是否产生模拟数据由 Agent 决定，不引入 Server 测试身份登记；原因是当前用途为开发与上线前测试，需要保持简单。

这调整了此前测试阶段在系统内维护 testMode、simulated 来源及测试单据隔离的设计；历史交付描述仍保留。用户随后授权发布 [Issue #3](https://github.com/EziosWJ/weigh-bridge/issues/3) 后实施，已实现并通过可执行验证。操作员确认、重量与对应抓拍依据、服务端业务校验和断网暂停确认规则继续适用。此决策不代表真实设备已接入或验收；范围见 [Spec](../requirements/agent-mock-issue-spec.md)，准确报文见 [协议 v1](../../protocol/v1.md)。
