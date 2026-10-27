# 重要决策索引

本页维护决策的当前适用范围。开发行为直接读取[总规格](../requirements/standalone-site-spec.md)、[业务规格](../requirements/weighing-business-spec.md)及[设备规格](../requirements/local-device-maintenance-spec.md)；只有需要理解取舍时才读 ADR。完成状态见[当前状态](../next.md)。

## 当前有效决定

| ADR | 保留的决定与理由 |
| --- | --- |
| [0028 单一本厂形态](0028-single-local-runtime-identity.md) | 每厂自治、固定身份及单一路径，减少中心和多模式依赖；初始化已合并 0029 修订，旧清库仅为历史切换 |
| [0031 本厂身份自动建立](0031-automatic-local-identity.md) | 取消人工两名称初始化，保留内部身份与历史关联，网页聚焦过磅单标题 |
| [0024 PostgreSQL](0024-postgresql-only.md) | 开发、数据库测试、生产一套数据库，退出双库成本；保留 migration 历史 |
| [0030 设备职责与配置](0030-local-device-responsibilities-and-configuration.md) | Server 管相机与视频，Agent 管仪表与打印；局域网无配对接入、单次应用及成功参数备份，换取安装维护简化 |
| [0033 编码身份与单号](0033-code-identity-and-ticket-number.md) | 单号使用编码、日期和日流水并防备份恢复复用；本地编码固定及关联键的决定由 0034 取代 |
| [0034 可维护的本地身份](0034-editable-local-identity.md) | 本地业务不以可变厂区／地磅编码关联，保留历史快照和打印防重 |
| [0032 成功采集后冻结](0032-freeze-successful-capture.md) | 采集时校验并冻结，确认不依赖后续设备状态，避免实时变化打断人工核对 |
| [0013 正式称重证据](0013-formal-weighing-evidence.md) | 设备依据、人工确认及本地校验，不用手输原始值代替采集 |
| [0014 原始与最终重量](0014-manual-weight-adjustment.md) | 双向调整、单磅重称和单据版本留痕，适应现场纠错同时保留原始依据 |
| [0027 可选抓拍](0027-optional-weighing-capture.md) | 默认强制、管理员可关闭，以当时策略和缺图状态保留例外依据 |
| [0019 单磅单 Agent](0019-one-agent-per-scale.md) | 首连接保持、明确踢出和结果核实，避免真实 Agent 与 mock 抢占 |
| [0015 独立模拟](0015-agent-owned-simulation.md) | 独立 mock 或设备输入模拟，业务使用统一处理链路 |
| [0022 D12 接入](0022-keli-d12-serial-integration.md) | 单 Agent 独占串口，以已核验帧与单位作为依据，参数简化合并 0030 |

## 按模块读取的工程决定

| ADR | 当前适用范围 |
| --- | --- |
| [0004 Go 分层](0004-backend-architecture-and-database-strategy.md) | 分层与显式装配继续有效，多库及早期权限限制已退出 |
| [0005 YAML 配置](0005-yaml-based-runtime-configuration.md) | Server 启动配置及环境覆盖；业务标题／摄像头等可维护参数按设备规格归位 |
| [0006 审计原子性](0006-atomic-operation-audit.md) | 业务成功写入与审计原子提交 |
| [0007 通知原子性](0007-atomic-site-notifications.md) | 对应角色变更／群发、审计和通知在同一事务；软件证据见链接的通知实现记录 |
| [0008 上传资源](0008-file-upload-resource-bounds.md) | 请求、文件、并发和临时资源有界，保留既有响应契约 |
| [0009 登录防护](0009-login-brute-force-protection.md) | 进程内准入及限流，先于密码验证和数据库日志写入 |
| [0011 宿主机监控](0011-host-resource-collection-boundary.md) | 采集本机、容器用宿主机 collector；后续 Windows／可选开关见[模块参考](../reference/README.md) |
| [0012 通知实时边界](0012-notification-realtime-boundary.md) | 数据库先持久化、SSE 提示、重连重查；当前数据库仅 PostgreSQL |

这些模块决定不新增地磅实施任务。原文“尚未实施”和历史数据库表述不用于判断现状。

## 已退出默认阅读路径的决定

原编号、原始取舍及源仓库引用保留于[历史 ADR](../history/adr/)。0001／0002 为 Java 与 Go 迁移历史；0010 为已退役 SQLite 支持；0016／0017 为独立维护形态；0018／0020／0021／0023 为多厂、配对、中心同步及双模式历史；0025／0026 的相机和草稿配置归属、0029 的旧连接工具已被最终设备职责收敛。仍有效的隔离、初始化、维护保护等条款已经合入当前规格。

部分有效 ADR 已整理为短摘要，原文存于同号历史文件，详见[收敛映射](../history/document-consolidation.md)。以后直接更新当前规格；重大取舍另写 ADR 并指明替代范围，不把新增补丁链留给开发者推算。
