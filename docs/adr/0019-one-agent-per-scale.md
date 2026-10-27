---
status: accepted (consolidated summary)
---

# 每台地磅只允许一个 Agent 在线

2026-10-24 有效决定摘要，合并已确认修订，不构成新的业务决策。[原始决定与演进记录](../history/adr/0019-one-agent-per-scale.md)保留来源与历史事实；实现及验收统一见[当前状态](../next.md)。

## 有效决定

同磅先连接者保持，后来者明确冲突，真实 Agent 与 mock 一致；不拆开仪表与打印连接。踢出需独立动作权限及本厂操作授权并留审计，只针对选定连接。收到 kick 后整个 Agent／mock 进程退出，停止本轮重连且不被守护立即拉起；可人工启动或随主机重启恢复。正常网络中断保留重连。

## 取舍

单一现场代理避免真实与 mock 互相抢占，以及设备状态和打印归属分裂。踢出使在途采集失败，成功采集的结果按 [ADR 0032](0032-freeze-successful-capture.md) 保留；不能撤销已提交业务或推定未出纸；退出未确认如实显示，打印未知先核实。相机已按新目标归 Server，不再由此决策要求 Agent 承担相机。

## 当前细则

[设备规格第 3 节](../requirements/local-device-maintenance-spec.md#3-agent-接入与维护)。
