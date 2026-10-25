# 历史与交付证据

此目录保留原决定、讨论、实施计划和验证记录，文中的“当前、待实施、已授权”只指当时上下文。日常开发读取[当前总规格](../requirements/standalone-site-spec.md)，实际差距见[当前状态](../next.md)；历史正文不追加新需求，也不授权部署、清库、提交或远程发布。

## 交付证据入口

| 主题 | 原始记录及范围 |
| --- | --- |
| 本厂形态及初始化 | [单一本厂交付及 ADR 0029 补充](../requirements/local-site-only-delivery.md)；保留当时开发库切换、隔离安装验证及冒烟失败；后续修复见新交付 |
| 新设备职责 | [2026-10-25 软件交付](../requirements/local-device-maintenance-delivery.md)；DC-01～DC-09、AC-01～AC-10 的软件范围与现场限制；[原工作包](requirements/local-device-maintenance-plan.md)归档 |
| PostgreSQL | [单库交付](requirements/postgresql-only-delivery.md)；旧 SQLite SQL 在[专用档案](../archive/sqlite-migrations/README.md) |
| 业务后端 | [业务闭环交付](requirements/backend-business-loop-delivery.md)；原阶段测试性质和数据库范围不代表当前需求 |
| 操作员更正 | [前端规则验证](requirements/operator-weighing-corrections-delivery.md)；双向调整、隔日、单磅重称及重开 |
| Agent／mock | [协议及 mock 交付](requirements/agent-mock-delivery.md)、[采样有效性](requirements/sample-validity-delivery.md)、[单磅互斥及隔离](requirements/multi-site-delivery.md) |
| 维护 | [首期交付](requirements/agent-maintenance-delivery.md)、[改版交付](requirements/agent-maintenance-revision-delivery.md)、[配置交付](requirements/site-agent-configuration-delivery.md)；独立维护、旧配对及草稿不代表最终产品 |
| 打印 | [单据流程](requirements/ticket-view-print-delivery.md)、[连续纸软件](requirements/continuous-paper-print-delivery.md)、[SK-820 实机](../device/aisino-sk820-agent-test.md)；A4 单张与连续五联分别验收 |
| D12 | [Agent 接入](requirements/d12-agent-integration-delivery.md)、[模拟器](requirements/d12-simulator-delivery.md)、[原始串口实验](../device/keli-d12-rs232-blog.md)；非零原始抓包与业务闭环区分 |
| 视频与抓拍策略 | [旧视频实现／验证](device/camera-live-preview.md)、[模拟源实验](device/camera-simulation-dev-memo.md)、[可选抓拍旧版软件交付](requirements/optional-weighing-capture-spec.md#已完成的软件交付) |
| 旧部署 | [中心同步](requirements/local-first-deployment-delivery.md)、[双模式自治](requirements/standalone-site-delivery.md)；不证明当前目标或生产切换 |
| UI 与工程 | [Phase 1](requirements/phase-1-delivery.md)、[Phase 2A](requirements/phase-2a-delivery.md)、[可用性](requirements/phase-2a-usability-delivery.md)、[独立工作台](requirements/standalone-workbench-delivery.md)、[工业工作台](requirements/industrial-workbench-ui-delivery.md)；浏览器模拟不替代真实设备 |

## 原始材料

- [requirements/](requirements/)：访谈、问卷、历史规格、已结束计划和阶段交付。
- [adr/](adr/)：被替代 ADR，以及现行短摘要对应的完整原决定；原编号不重排。
- [architecture/](architecture/)：历史业务模型设计和旧中心同步方案。
- [device/](device/)：旧拓扑、配对／相机／草稿操作及实机实验原稿；当前接入、参数及排障在 docs/device/。
- [ui/](ui/)：工作台与设备维护的逐轮设计、模拟阶段和当时验证；现行规则在 web/design-system/pages/。
- [platform/](platform/)：脚手架模块的计划、设计、工单和验证；模块维护入口见[参考索引](../reference/README.md)。
- [scaffold/](scaffold/)：原 record/ 的重构背景；源仓库与许可见 [SOURCE](../../SOURCE.md)。
- [prompts/](prompts/)：旧任务启动提示，不作为本轮执行指令。
- [原多模式上下文](context-before-standalone-baseline.md)：保留当时领域和阶段描述。

本次条款去向、废止范围及文件路径对应见[文档收敛记录](document-consolidation.md)。历史 `base-project-golang` 与当前 `weigh-bridge` Issue 身份分别保留，不按同号映射，不宣称远程正文或标签已同步。
