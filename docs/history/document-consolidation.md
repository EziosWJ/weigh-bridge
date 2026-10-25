# 2026-10-24～25 文档收敛记录

本次只整理文档，不修改业务代码、migration 或真实配置，不执行数据库／设备操作，不提交、推送或更新远程 Issue。以整理前的工作区文件为来源，包含尚未提交的新增 ADR、规格和交付；未从 Git 旧版本恢复覆盖。原始阶段事实保留，只有当前规格和有效决定摘要直接表达最终规则。

## 条款去向与已废止范围

| 原材料／主题 | 当前权威位置 | 收敛处理 |
| --- | --- | --- |
| 原 CONTEXT 的阶段、进度和实现细节 | [总规格](../requirements/standalone-site-spec.md)、[状态](../next.md) | CONTEXT 只留术语与稳定约束，清除旧清库和配对进度 |
| ADR 0023／0028、原单厂总规格 | [总规格](../requirements/standalone-site-spec.md) | 单一运行形态、固定身份、本地权限、分机、发布和数据保护；旧多模式与一次性清库退出当前操作 |
| ADR 0029、初始化补充 | [总规格初始化](../requirements/standalone-site-spec.md#本厂身份初始化与管理) | 两名称初始化、配置即完成、旧标记兼容、状态／API 契约和账号授权；配对工具仅旧版运行资料保留 |
| 后端闭环、可用性改版、业务访谈 | [业务规则第 1～2 节](../requirements/weighing-business-spec.md#1-资料任务与授权) | 资料、页面分工、任务原子性、同厂唯一与会话；旧浏览器权威存储及测试阶段限制退出 |
| 独立工作台与改版 | [业务规则第 2 节](../requirements/weighing-business-spec.md#2-工作台与人工确认)、[页面 Pattern](../../web/design-system/pages/weighing.md) | 独立窗口、车牌核对、先采集后补资料、当前车辆保护；固定两图按设备规格更新，旧模拟交付保留 |
| ADR 0013／0014、人工调整及操作员修订 | [业务规则第 3、5 节](../requirements/weighing-business-spec.md#3-重量与称重证据) | 吨制、双向最终值、默认备注、隔日、单磅重称、版本与原始证据；主管审批、单向扣减、必填调整备注退出 |
| 旧多厂与中心材料 | [业务规则第 1、5 节](../requirements/weighing-business-spec.md#1-资料任务与授权)、总规格 | 保留同厂归属、资料快照、编号及权限；取消跨厂管理、共享资料、中心审批／同步 |
| 单据查看与打印 | [业务规则第 6 节](../requirements/weighing-business-spec.md#6-单据查询与打印) | 精简票面、五联一次复写、版本查询／汇总、未知核实及补打；标题快照按设备规格 |
| ADR 0027、可选抓拍规格 | [业务规则第 4 节](../requirements/weighing-business-spec.md#4-抓拍策略) | 受保护配置、默认／非法值、策略快照、缺图事实、混合策略与失效；前后固定两图退出 |
| ADR 0015／0019、Agent mock 及多厂访谈 | [业务规则第 7 节](../requirements/weighing-business-spec.md#7-留存故障与验收)、[设备规格第 3 节](../requirements/local-device-maintenance-spec.md#3-agent-接入与维护) | 统一业务链路、独立模拟输入、单磅互斥、踢出权限／进程退出／打印核实；不恢复 testMode 或测试身份登记 |
| ADR 0016／0017／0020／0022／0025／0026、旧设备维护 | [设备规格](../requirements/local-device-maintenance-spec.md) | 维护保护、仪表依据、参数与打印账本保留；旧配对、Agent 相机、用户草稿、固定仪表选项退出目标 |
| ADR 0030 与当前设备工作包 | [设备规格](../requirements/local-device-maintenance-spec.md)、[计划](requirements/local-device-maintenance-plan.md) | 保留测量代次至确认、相机配置版本、MediaMTX 漂移／删除恢复、标题快照及升级保护，AC-01～10 和 DC-01～09 不删减 |
| 原有交付及混合规格中的验证 | [历史索引](README.md)、[当前状态](../next.md) | 日期、原始验证、失败及现场限制保留；原单厂交付路径保留供发布脚本使用 |
| 工程 ADR、通知／上传／监控资料 | [决策索引](../adr/README.md)、[模块参考](../reference/README.md) | 保留有效模块约束和来源；Java、SQLite、多库及已结束计划归档，旧状态不自动变为当前任务 |
| README、next、架构／需求索引与 Agent 指导 | [开发基线](../development-baseline.md) | 入口改为导航，完成状态只维护一处；新决策直接更新有效正文，不再堆覆盖补丁 |

## 保留的完成边界

初次收敛（2026-10-24）时，ADR 0028／0029 有软件交付、业务浏览器冒烟存在时序失败，新设备职责尚无完整交付证据。暂停期间另行产生了[2026-10-25 新设备软件交付](../requirements/local-device-maintenance-delivery.md)，原失败已有修复和复验记录；本次续作保留这些结果，不把旧失败重复列为当前未解决。

D12 原始带载报文核验不等于 Agent 非零业务闭环；A4 单张确认不等于连续五联验收；模拟 RTSP 不等于真实摄像头。当前软件及现场边界由[状态页](../next.md)维护，文档整理不产生新的代码验证结果。

## 文件归档映射

原路径为本次整理前的仓库位置；目标保留原文并增加历史效力说明、修复相对链接。目录链接和原源仓库 Issue 身份保留。

| 原路径 | 归档文件 |
| --- | --- |
| `docs/adr/0001-backend-maven-multi-module-boundaries.md` | [0001-backend-maven-multi-module-boundaries.md](adr/0001-backend-maven-multi-module-boundaries.md) |
| `docs/adr/0002-go-api-replaces-java-with-compatible-contract.md` | [0002-go-api-replaces-java-with-compatible-contract.md](adr/0002-go-api-replaces-java-with-compatible-contract.md) |
| `docs/adr/0010-sqlite-production-support.md` | [0010-sqlite-production-support.md](adr/0010-sqlite-production-support.md) |
| `docs/adr/0016-agent-local-maintenance.md` | [0016-agent-local-maintenance.md](adr/0016-agent-local-maintenance.md) |
| `docs/adr/0017-agent-maintenance-config-and-lan.md` | [0017-agent-maintenance-config-and-lan.md](adr/0017-agent-maintenance-config-and-lan.md) |
| `docs/adr/0018-site-business-isolation.md` | [0018-site-business-isolation.md](adr/0018-site-business-isolation.md) |
| `docs/adr/0020-managed-agent-bindings-and-local-activation.md` | [0020-managed-agent-bindings-and-local-activation.md](adr/0020-managed-agent-bindings-and-local-activation.md) |
| `docs/adr/0021-local-weighing-authority-and-central-sync.md` | [0021-local-weighing-authority-and-central-sync.md](adr/0021-local-weighing-authority-and-central-sync.md) |
| `docs/adr/0023-standalone-site-baseline.md` | [0023-standalone-site-baseline.md](adr/0023-standalone-site-baseline.md) |
| `docs/adr/0025-camera-stream-maintenance.md` | [0025-camera-stream-maintenance.md](adr/0025-camera-stream-maintenance.md) |
| `docs/adr/0026-agent-owned-device-maintenance.md` | [0026-agent-owned-device-maintenance.md](adr/0026-agent-owned-device-maintenance.md) |
| `docs/adr/0029-local-initialization-and-agent-connection.md` | [0029-local-initialization-and-agent-connection.md](adr/0029-local-initialization-and-agent-connection.md) |
| `docs/architecture/backend-business-loop-design.md` | [backend-business-loop-design.md](architecture/backend-business-loop-design.md) |
| `docs/architecture/local-first-sync-design.md` | [local-first-sync-design.md](architecture/local-first-sync-design.md) |
| `docs/device/local-first-deployment.md` | [local-first-deployment.md](device/local-first-deployment.md) |
| `docs/device/multi-site-agent-setup.md` | [multi-site-agent-setup.md](device/multi-site-agent-setup.md) |
| `docs/file-upload-plan.md` | [file-upload-plan.md](platform/file-upload-plan.md) |
| `docs/menu-search-plan.md` | [menu-search-plan.md](platform/menu-search-plan.md) |
| `docs/notification-enhancement-design.md` | [notification-enhancement-design.md](platform/notification-enhancement-design.md) |
| `docs/notification-implementation.md` | [notification-implementation.md](platform/notification-implementation.md) |
| `docs/notification-plan.md` | [notification-plan.md](platform/notification-plan.md) |
| `docs/prompts/local-initialization-simplification.md` | [local-initialization-simplification.md](prompts/local-initialization-simplification.md) |
| `docs/requirements/agent-maintenance-delivery.md` | [agent-maintenance-delivery.md](requirements/agent-maintenance-delivery.md) |
| `docs/requirements/agent-maintenance-discovery.md` | [agent-maintenance-discovery.md](requirements/agent-maintenance-discovery.md) |
| `docs/requirements/agent-maintenance-revision-delivery.md` | [agent-maintenance-revision-delivery.md](requirements/agent-maintenance-revision-delivery.md) |
| `docs/requirements/agent-maintenance-revision-spec.md` | [agent-maintenance-revision-spec.md](requirements/agent-maintenance-revision-spec.md) |
| `docs/requirements/agent-maintenance-spec.md` | [agent-maintenance-spec.md](requirements/agent-maintenance-spec.md) |
| `docs/requirements/agent-mock-delivery.md` | [agent-mock-delivery.md](requirements/agent-mock-delivery.md) |
| `docs/requirements/agent-mock-discovery.md` | [agent-mock-discovery.md](requirements/agent-mock-discovery.md) |
| `docs/requirements/agent-mock-issue-spec.md` | [agent-mock-issue-spec.md](requirements/agent-mock-issue-spec.md) |
| `docs/requirements/agent-mock-spec.md` | [agent-mock-spec.md](requirements/agent-mock-spec.md) |
| `docs/requirements/backend-business-loop-delivery.md` | [backend-business-loop-delivery.md](requirements/backend-business-loop-delivery.md) |
| `docs/requirements/backend-business-loop-plan.md` | [backend-business-loop-plan.md](requirements/backend-business-loop-plan.md) |
| `docs/requirements/backend-business-loop-spec.md` | [backend-business-loop-spec.md](requirements/backend-business-loop-spec.md) |
| `docs/requirements/business-v1-discovery.md` | [business-v1-discovery.md](requirements/business-v1-discovery.md) |
| `docs/requirements/business-v1-ui-scope.md` | [business-v1-ui-scope.md](requirements/business-v1-ui-scope.md) |
| `docs/requirements/continuous-paper-print-delivery.md` | [continuous-paper-print-delivery.md](requirements/continuous-paper-print-delivery.md) |
| `docs/requirements/customer-confirmation-checklist.md` | [customer-confirmation-checklist.md](requirements/customer-confirmation-checklist.md) |
| `docs/requirements/d12-agent-integration-delivery.md` | [d12-agent-integration-delivery.md](requirements/d12-agent-integration-delivery.md) |
| `docs/requirements/d12-simulator-delivery.md` | [d12-simulator-delivery.md](requirements/d12-simulator-delivery.md) |
| `docs/requirements/industrial-workbench-ui-delivery.md` | [industrial-workbench-ui-delivery.md](requirements/industrial-workbench-ui-delivery.md) |
| `docs/requirements/local-first-deployment-delivery.md` | [local-first-deployment-delivery.md](requirements/local-first-deployment-delivery.md) |
| `docs/requirements/local-first-deployment-discovery.md` | [local-first-deployment-discovery.md](requirements/local-first-deployment-discovery.md) |
| `docs/requirements/local-first-deployment-issue-spec.md` | [local-first-deployment-issue-spec.md](requirements/local-first-deployment-issue-spec.md) |
| `docs/requirements/local-first-deployment-spec.md` | [local-first-deployment-spec.md](requirements/local-first-deployment-spec.md) |
| `docs/requirements/local-initialization-simplification-spec.md` | [local-initialization-simplification-spec.md](requirements/local-initialization-simplification-spec.md) |
| `docs/requirements/manual-weight-adjustment-delivery.md` | [manual-weight-adjustment-delivery.md](requirements/manual-weight-adjustment-delivery.md) |
| `docs/requirements/manual-weight-adjustment-discovery.md` | [manual-weight-adjustment-discovery.md](requirements/manual-weight-adjustment-discovery.md) |
| `docs/requirements/manual-weight-adjustment-spec.md` | [manual-weight-adjustment-spec.md](requirements/manual-weight-adjustment-spec.md) |
| `docs/requirements/manual-weight-adjustment.md` | [manual-weight-adjustment.md](requirements/manual-weight-adjustment.md) |
| `docs/requirements/multi-site-delivery.md` | [multi-site-delivery.md](requirements/multi-site-delivery.md) |
| `docs/requirements/multi-site-discovery.md` | [multi-site-discovery.md](requirements/multi-site-discovery.md) |
| `docs/requirements/operator-weighing-corrections-delivery.md` | [operator-weighing-corrections-delivery.md](requirements/operator-weighing-corrections-delivery.md) |
| `docs/requirements/operator-weighing-corrections-spec.md` | [operator-weighing-corrections-spec.md](requirements/operator-weighing-corrections-spec.md) |
| `docs/requirements/optional-weighing-capture-spec.md` | [optional-weighing-capture-spec.md](requirements/optional-weighing-capture-spec.md) |
| `docs/requirements/phase-1-changed-files.md` | [phase-1-changed-files.md](requirements/phase-1-changed-files.md) |
| `docs/requirements/phase-1-delivery.md` | [phase-1-delivery.md](requirements/phase-1-delivery.md) |
| `docs/requirements/phase-2a-delivery.md` | [phase-2a-delivery.md](requirements/phase-2a-delivery.md) |
| `docs/requirements/phase-2a-plan.md` | [phase-2a-plan.md](requirements/phase-2a-plan.md) |
| `docs/requirements/phase-2a-usability-delivery.md` | [phase-2a-usability-delivery.md](requirements/phase-2a-usability-delivery.md) |
| `docs/requirements/phase-2a-usability.md` | [phase-2a-usability.md](requirements/phase-2a-usability.md) |
| `docs/requirements/postgresql-only-delivery.md` | [postgresql-only-delivery.md](requirements/postgresql-only-delivery.md) |
| `docs/requirements/sample-validity-delivery.md` | [sample-validity-delivery.md](requirements/sample-validity-delivery.md) |
| `docs/requirements/site-agent-configuration-delivery.md` | [site-agent-configuration-delivery.md](requirements/site-agent-configuration-delivery.md) |
| `docs/requirements/site-agent-configuration-discovery.md` | [site-agent-configuration-discovery.md](requirements/site-agent-configuration-discovery.md) |
| `docs/requirements/standalone-site-delivery.md` | [standalone-site-delivery.md](requirements/standalone-site-delivery.md) |
| `docs/requirements/standalone-workbench-delivery.md` | [standalone-workbench-delivery.md](requirements/standalone-workbench-delivery.md) |
| `docs/requirements/standalone-workbench-plate-recognition-spec.md` | [standalone-workbench-plate-recognition-spec.md](requirements/standalone-workbench-plate-recognition-spec.md) |
| `docs/requirements/ticket-view-print-delivery.md` | [ticket-view-print-delivery.md](requirements/ticket-view-print-delivery.md) |
| `docs/requirements/ticket-view-print-spec.md` | [ticket-view-print-spec.md](requirements/ticket-view-print-spec.md) |
| `docs/requirements/weight-entry-tonne-plan.md` | [weight-entry-tonne-plan.md](requirements/weight-entry-tonne-plan.md) |
| `docs/requirements/workbench-redesign.md` | [workbench-redesign.md](requirements/workbench-redesign.md) |
| `docs/server-monitoring-extension-validation.md` | [server-monitoring-extension-validation.md](platform/server-monitoring-extension-validation.md) |
| `docs/server-monitoring-implementation.md` | [server-monitoring-implementation.md](platform/server-monitoring-implementation.md) |
| `docs/server-monitoring-tickets.md` | [server-monitoring-tickets.md](platform/server-monitoring-tickets.md) |
| `docs/server-monitoring-tickets/01-host-overview.md` | [01-host-overview.md](platform/server-monitoring-tickets/01-host-overview.md) |
| `docs/server-monitoring-tickets/02-history-and-warnings.md` | [02-history-and-warnings.md](platform/server-monitoring-tickets/02-history-and-warnings.md) |
| `docs/server-monitoring-tickets/03-docker-host-collector.md` | [03-docker-host-collector.md](platform/server-monitoring-tickets/03-docker-host-collector.md) |
| `docs/server-monitoring-tickets/04-disk-monitoring.md` | [04-disk-monitoring.md](platform/server-monitoring-tickets/04-disk-monitoring.md) |
| `docs/server-monitoring-tickets/05-network-monitoring.md` | [05-network-monitoring.md](platform/server-monitoring-tickets/05-network-monitoring.md) |
| `docs/server-monitoring-tickets/06-process-ranking.md` | [06-process-ranking.md](platform/server-monitoring-tickets/06-process-ranking.md) |
| `docs/server-monitoring-tickets/07-nvidia-monitoring.md` | [07-nvidia-monitoring.md](platform/server-monitoring-tickets/07-nvidia-monitoring.md) |
| `docs/server-monitoring.md` | [server-monitoring.md](platform/server-monitoring.md) |
| `record/base-api-refactor.md` | [base-api-refactor.md](scaffold/base-api-refactor.md) |
| `record/react-admin-refactor.md` | [react-admin-refactor.md](scaffold/react-admin-refactor.md) |

## 有效决定摘要与完整原文

下列原路径继续存在，用短摘要表达已确认的最终决定和取舍；完整原文另存同号档案，未重排 ADR 或伪造新决策。

| 当前摘要 | 完整原文 |
| --- | --- |
| [0004-backend-architecture-and-database-strategy.md](../adr/0004-backend-architecture-and-database-strategy.md) | [原始记录](adr/0004-backend-architecture-and-database-strategy.md) |
| [0013-formal-weighing-evidence.md](../adr/0013-formal-weighing-evidence.md) | [原始记录](adr/0013-formal-weighing-evidence.md) |
| [0014-manual-weight-adjustment.md](../adr/0014-manual-weight-adjustment.md) | [原始记录](adr/0014-manual-weight-adjustment.md) |
| [0015-agent-owned-simulation.md](../adr/0015-agent-owned-simulation.md) | [原始记录](adr/0015-agent-owned-simulation.md) |
| [0019-one-agent-per-scale.md](../adr/0019-one-agent-per-scale.md) | [原始记录](adr/0019-one-agent-per-scale.md) |
| [0022-keli-d12-serial-integration.md](../adr/0022-keli-d12-serial-integration.md) | [原始记录](adr/0022-keli-d12-serial-integration.md) |
| [0027-optional-weighing-capture.md](../adr/0027-optional-weighing-capture.md) | [原始记录](adr/0027-optional-weighing-capture.md) |
| [0028-single-local-runtime-identity.md](../adr/0028-single-local-runtime-identity.md) | [原始记录](adr/0028-single-local-runtime-identity.md) |

## 2026-10-25 续作归档与收敛

工作台页面去除逐轮补丁，保留工业视觉、集中核对、失效提示、键盘与响应式规则；设备维护收敛为串口／打印单次应用，摄像头有独立覆盖说明。设备资料保留实际帧、参数、命令及故障经验，旧实现计划和实验原文按需查档案。Server／Agent 运行说明与已交付 v2 同步，旧 mock 图片控制明确限于 v1 兼容；工程 ADR 中的 SQLite 范围按已确认单库决定修正，初始化不再写生成 Agent 密钥。

| 原位置 | 原文／档案 | 当前处理 |
| --- | --- | --- |
| `docs/adr/0005-yaml-based-runtime-configuration.md` | [原始记录](adr/0005-yaml-based-runtime-configuration.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `docs/adr/0007-atomic-site-notifications.md` | [原始记录](adr/0007-atomic-site-notifications.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `docs/adr/0012-notification-realtime-boundary.md` | [原始记录](adr/0012-notification-realtime-boundary.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `docs/device/agent-local-maintenance.md` | [原始记录](device/agent-local-maintenance.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `docs/device/agent-remote-maintenance.md` | [原始记录](device/agent-remote-maintenance.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `docs/device/camera-live-preview.md` | [原始记录](device/camera-live-preview.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `docs/device/camera-simulation-dev-memo.md` | [原始记录](device/camera-simulation-dev-memo.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `docs/device/ffmpeg-rtsp-camera-simulation-blog.md` | [原始记录](device/ffmpeg-rtsp-camera-simulation-blog.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `docs/device/keli-d12-agent-integration-guide.md` | [原始记录](device/keli-d12-agent-integration-guide.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `web/design-system/pages/agent-maintenance.md` | [原始记录](ui/agent-maintenance-page-evolution.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `web/design-system/pages/weighing.md` | [原始记录](ui/weighing-page-evolution.md) | 原路径维护收敛版，原决定或实验内容保留于档案 |
| `docs/requirements/local-device-maintenance-plan.md` | [已交付工作包](requirements/local-device-maintenance-plan.md) | 完成度及下一步见状态页，避免重复启动 |
| `docs/requirements/file-tree.txt` | [历史文件树](requirements/file-tree.txt) | 与 Phase 1 原始交付一起保存 |

## 整理结果与验证

当前需求入口只列总规格、业务规则、设备维护三份有效规格；交付事实另有两个原始记录入口。原有 86 份 Markdown 移出当前开发目录，8 份多轮 ADR 与 11 份页面／设备／工程说明的完整原文另存，合计 105 份原文保留；另归档历史文件树。历史保留会增加档案文件，压缩的是有效正文与默认阅读量，并非删除追溯证据。

原文保留检查逐行比较整理前快照与档案，仅规范化相对链接、空行及新增历史提示；105 份均无正文差异。当前文档核查覆盖有效条款来源映射、业务 B-01～B-08、设备 AC-01～AC-10、原 DC-01～DC-09、链接及标题锚点、已删除路径引用、AGENTS.md 符号链接及 Git 空白差异。

文档整理未执行 Go／Web 构建或设备、数据库操作；测试通过结论仅引用已有交付记录。本次不提交或推送，保留原有代码与配置工作区改动。
