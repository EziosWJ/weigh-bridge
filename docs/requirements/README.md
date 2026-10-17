# 阶段需求

| 阶段或材料 | 状态 | 入口 |
| --- | --- | --- |
| Phase 1：工程初始化 | 已验收，保留脚手架模块、依赖与 migration 历史 | [交付与验收记录](phase-1-delivery.md) |
| 业务一期需求访谈 | 当前界面阶段范围已确认，后续现场事项仍待核实 | [访谈结论](business-v1-discovery.md) |
| Phase 2A：界面与流程验证 Spec | 已完成前端模拟；正式现场能力未实现 | [需求与验收依据](business-v1-ui-scope.md) |
| Phase 2A 任务计划 | P2A-01～06 已完成，2026-10-16 验证 | [任务顺序与依赖](phase-2a-plan.md) |
| Phase 2A 交付与验收 | 核心测试与简短页面冒烟通过 | [实际验证与限制](phase-2a-delivery.md) |
| Phase 2A 可用性改版 | U01～U06 实现及约定验证完成 | [范围与执行计划](phase-2a-usability.md)、[改版交付记录](phase-2a-usability-delivery.md) |
| 人工重量调整与双列表 | 最终重量录入、系统反算扣减、吨制千分之一精度与有效小数位显示已完成前端模拟与约定验证；正式业务未实现 | [Spec](manual-weight-adjustment-spec.md)、[业务规则](manual-weight-adjustment.md)、[交付记录](manual-weight-adjustment-delivery.md) |
| 最终重量录入与吨制调整 | 前端、缓存迁移与验证已完成；正式业务未实现 | [页面、数据与交付记录](weight-entry-tonne-plan.md) |
| 后续现场业务与设备接入 | 待设计和确认，未实现 | [领域与项目约束](../../CONTEXT.md) |

业务一期与已完成的工程初始化 Phase 1 分开命名。当前 Phase 2A 采用车牌手填、模拟重量、浏览器保存、测试单据、打印预览与演示面板；摄像头未真实接入，不生成正式业务记录。

后续正式称重仍要求操作员确认、设备重量、对应抓拍及 Server 校验，相关取舍见 [ADR 0013](../adr/0013-formal-weighing-evidence.md)。真实业务后端、Agent、WSS 和设备联调不作为 Phase 2A 已完成能力。
