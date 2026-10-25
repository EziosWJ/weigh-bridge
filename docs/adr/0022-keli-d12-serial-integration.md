---
status: accepted (consolidated summary)
---

# D12 仪表由现场 Agent 独占采集

2026-10-24 有效决定摘要，合并已确认修订，不构成新的业务决策。[原始决定与演进记录](../history/adr/0022-keli-d12-serial-integration.md)保留来源与历史事实；实现及验收统一见[当前状态](../next.md)。

## 有效决定

D12 串口驱动与打印共用现场 Agent，保留原始帧、小数位、校验、稳定及故障依据。当前核验设备固定 tF=05、8N1、无流控、吨制及 1 秒无有效帧阈值，只开放端口和波特率；单位结论不推广到未核验的型号／固件。每帧及故障变化参与测量有效性核实。

## 取舍

沿用单 Agent 避免串口争用及第二条设备链路；按现场样本确定单位和校验，避免文档标志与实机差异造成千倍误差。零值、原始带载报文和完整业务闭环分别验证；固定参数减少维护选择，同时收窄了支持设备范围。

## 当前细则

[设备规格第 4 节](../requirements/local-device-maintenance-spec.md#4-地磅仪表)、[D12 接入参数](../device/keli-d12-agent-integration-guide.md)。
