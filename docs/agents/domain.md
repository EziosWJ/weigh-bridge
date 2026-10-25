# 领域文档

采用 single-context：根 CONTEXT.md 维护领域术语及稳定约束，docs/adr/ 保存重要决策理由；server、web、agent、protocol 不另设领域上下文。

开发先读 [CONTEXT](../../CONTEXT.md) 和[当前总规格](../requirements/standalone-site-spec.md)，按任务读业务／设备细则。实施前核对[当前状态](../next.md)，前端修改按 web/AGENTS.md 选择 Pattern。历史文档只在追溯来源或实际交付时读取，不作为隐含需求。

Issue、需求、设计与测试使用既有领域术语；发现术语冲突时澄清后更新 CONTEXT。业务行为、字段和验收写对应规格，技术取舍需要保留理由时写 ADR，进度写当前状态。新决策直接更新有效规格正文，旧条款归档，避免层层补丁；具体维护规则见[开发基线](../development-baseline.md)。
