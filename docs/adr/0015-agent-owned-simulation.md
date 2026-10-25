---
status: accepted (consolidated summary)
---

# 模拟通过独立设备工具提供

2026-10-24 有效决定摘要，合并已确认修订，不构成新的业务决策。[原始决定与演进记录](../history/adr/0015-agent-owned-simulation.md)保留来源与历史事实；实现及验收统一见[当前状态](../next.md)。

## 有效决定

业务 Server 统一按设备契约处理，不在系统内维护 testMode、模拟／正式两套流程或测试身份登记。独立 agent-mock 可替代真实 Agent；独立虚拟串口、模拟 RTSP 也可向真实驱动提供设备输入。真实 Agent 与同磅 mock 互斥，输入模拟器不占用 Agent 连接。

## 取舍

统一业务链路，避免为了测试维护两套称重规则；代价是验证记录必须明确模拟范围，软件通过不能证明真实设备可用。相机迁至 Server 后，其模拟源仍只提供输入，不恢复业务 Web 或真实驱动内伪造数据的分支。

## 当前细则

[业务规格](../requirements/weighing-business-spec.md#7-留存故障与验收)、[D12 模拟器](../device/d12-simulator.md)。
