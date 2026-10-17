# 领域文档

本仓库采用 single-context 布局：
根目录 CONTEXT.md 保存领域术语与项目约束，docs/adr/ 保存决策记录。
server、web、agent、protocol 不分别建立领域上下文。

## 阅读顺序

1. 探索代码前先读取根 CONTEXT.md。
2. 涉及既有决策时，读取 docs/adr/ 中与当前主题相关的记录。
3. 确认业务范围和阶段验收时，按需读取 docs/requirements/ 对应文档。
4. 修改前端前，读取 web/AGENTS.md 及涉及页面的 Pattern。

文档尚不存在时继续当前工作；由 domain-modeling 在术语或决策明确后按需创建。

## 领域语言

Issue、需求、测试和设计使用 CONTEXT.md 已定义的领域术语。
发现术语冲突或缺口时明确指出，通过 domain-modeling 澄清并记录。
需求与实现细节写入对应需求或设计文档，不作为术语定义堆入词汇表。

## 决策与阶段边界

方案与既有 ADR 冲突时，说明冲突及重新决策的理由。
历史脚手架 ADR 保留来源与适用背景，不能直接作为酒厂业务已实现的证据。
区分计划能力、当前界面模拟能力与实际完成能力。
