# 当前开发基线与文档效力

生效日期：2026-10-22。用户已接受本厂自治、三个核心服务和中心暂缓，授权本轮收敛文档。决策见 [ADR 0023](adr/0023-standalone-site-baseline.md)，当前阶段唯一总规格为[本厂自治与三服务部署](requirements/standalone-site-spec.md)。架构目标已确认；代码改造与现场切换尚未完成。

## 开发阅读顺序与冲突处理

1. 先读根 [CONTEXT](../CONTEXT.md)，了解领域规则和实际状态。
2. 开发当前阶段时读[当前规格](requirements/standalone-site-spec.md)；需要架构理由时读 ADR 0023。
3. 按下表进入仍有效的业务／设备细则，先看文档顶部效力说明。历史交付记录用于确认做过什么，不作为追加需求。
4. 被替代条款不参与当前验收；暂缓能力没有自动恢复期限。旧 Issue 或文档的“确认／ready／已授权／待实施”属于当时范围，不能覆盖本基线或视作新一轮启动指令。
5. 如当前有效的材料互相冲突，列明具体条款并解决范围冲突后再做依赖它的变更；不自行拼接新旧两套方案。实现与目标不一致时登记差距，不通过改写交付事实宣称已完成。

## 文档效力索引

| 主题／材料 | 当前效力 | 保留与替代范围 |
| --- | --- | --- |
| [ADR 0024](adr/0024-postgresql-only.md)、[数据库交付](requirements/postgresql-only-delivery.md) | 当前数据库依据，已完成收敛 | 开发、数据库集成测试与生产仅 PostgreSQL；旧 SQLite 要求退役，历史验证事实保留 |
| [ADR 0023](adr/0023-standalone-site-baseline.md)、[当前规格](requirements/standalone-site-spec.md) | 当前架构与阶段依据 | 本地自治、三服务、维护整合、中心暂缓；代码待调整 |
| [ADR 0021](adr/0021-local-weighing-authority-and-central-sync.md)、[中心同步业务规格](requirements/local-first-deployment-spec.md)、[Issue #4 规格](requirements/local-first-deployment-issue-spec.md)、[同步设计](architecture/local-first-sync-design.md) | 部分被替代／中心能力暂缓 | 保留本地业务权威、断互联网可用、编号与快照；替代中心管理权和首次下发；同步与跨厂查询退出本期 |
| [Issue #4 交付](requirements/local-first-deployment-delivery.md)、[旧部署说明](device/local-first-deployment.md) | 历史实现事实／旧拓扑操作说明 | 软件验证事实保留；旧三实例启用步骤不用于新自治部署 |
| [ADR 0018](adr/0018-site-business-isolation.md)、[多厂区需求](requirements/multi-site-discovery.md) | 部分被替代 | 集中部署、共享资料与跨厂入口退出本期；厂区隔离、同厂两次称重、权限范围保留 |
| [ADR 0016](adr/0016-agent-local-maintenance.md)、[ADR 0017](adr/0017-agent-maintenance-config-and-lan.md)、[维护首期](requirements/agent-maintenance-spec.md)、[维护改版](requirements/agent-maintenance-revision-spec.md) | 部分被替代／过渡可用 | 独立维护服务与密码不再是最终形态；配置与作业保护、核实及审计保留，整合后退役 |
| [ADR 0020](adr/0020-managed-agent-bindings-and-local-activation.md)、[厂区配置需求](requirements/site-agent-configuration-discovery.md) | 部分被替代 | 中心归属管理及同机常规下载导入流程调整；本地身份、明确应用、停用检查、账本保护保留 |
| [ADR 0013](adr/0013-formal-weighing-evidence.md)、[ADR 0014](adr/0014-manual-weight-adjustment.md)、[操作员处置规格](requirements/operator-weighing-corrections-spec.md) | 业务规则继续有效，含后续修订 | 设备证据、人工确认、原始／最终重量、双向调整、隔日续称、单磅重称、作废重开；断网按互联网与本地链路区分 |
| [服务端业务规格](requirements/backend-business-loop-spec.md)、[业务设计](architecture/backend-business-loop-design.md) | 业务模型可复用，阶段限制属历史 | 单据版本、幂等、审计、资料快照保留；全局车牌占用、浏览器测试打印、系统内模拟等以之后的厂区／Agent 规则为准 |
| [ADR 0015](adr/0015-agent-owned-simulation.md)、[ADR 0019](adr/0019-one-agent-per-scale.md)、[协议 v1](../protocol/v1.md) | 继续有效 | 模拟在独立 mock，真实与 mock 互斥；设备接本厂业务服务，不赋予用户确认权 |
| [ADR 0022](adr/0022-keli-d12-serial-integration.md)、[D12 交付](requirements/d12-agent-integration-delivery.md)、[接入指导](device/keli-d12-agent-integration-guide.md) | 驱动细则与现场事实继续有效 | 保留参数、单位、独占与保鲜规则；非零、稳定灯、重启权限等未验收项继续跟踪 |
| [单据打印规格](requirements/ticket-view-print-spec.md)、[连续纸校准](device/sk820-continuous-paper-calibration.md) | 继续有效 | 不因部署收缩改变单据内容、打印未知核实或实物验收要求 |
| Phase 1／Phase 2A、早期访谈、脚手架 ADR 与 record/ | 历史阶段材料 | 当时的实现限制与“尚未实现”不表示现在；业务条款按后续有效规格应用 |

## 已有实现与当前差距

已具备服务端业务闭环、厂区隔离、Agent 协议与 mock、内嵌前端发布、打印与 D12 部分真实接入。Issue #4 中心同步已有软件实现和双数据库／浏览器验证；真实生产未切换。详见[历史与交付索引](history/README.md)。

当前 `site` 配置、初始化与写入权限仍依赖中心，独立维护服务仍存在；摄像头未接入，真实非零称重及连续纸等尚待现场验收。具体改造项与验收以当前规格为准，不把“暂停中心”理解为现有 site 实例关掉中心即可正式上线。

## 工作项映射

| 工作项 | 本地基线中的用途 |
| --- | --- |
| [EziosWJ/weigh-bridge #4](https://github.com/EziosWJ/weigh-bridge/issues/4) | 旧中心＋厂区同步实现来源；本地需求／设计已标效力，不再作为当前启动或验收总依据 |
| [EziosWJ/weigh-bridge #3](https://github.com/EziosWJ/weigh-bridge/issues/3) | Agent 契约与 mock 实现来源；设备通信细则继续复用，部署与断网规则按 ADR 0023 |
| 本厂自治改造 | 当前规格定义下一步范围，尚未创建远程工作项；后续工作项必须链接本规格和 ADR 0023 |

本轮未读取或修改远程 Issue 状态，未发布、评论或关闭工作项；表格不宣称远程标签或正文已同步。远程工作项后续更新遵循[工作项约定](agents/issue-tracker.md)，保留源仓库身份和历史事实。

## 后续维护规则

新架构决策用新 ADR 记录替代范围，并更新本索引、CONTEXT 和当前规格入口；旧正文保留，在顶部添加精确的效力说明。实际完成后更新交付记录和现状，不删除尚未验收的限制。中心恢复需新范围确认，不能移除“暂缓”标签就直接实施旧同步方案。
