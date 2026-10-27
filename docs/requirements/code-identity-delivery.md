# 本厂编码身份与过磅单号交付记录

本记录保留当时交付事实；编码不可修改及本地编码关联已由 [ADR 0034](../adr/0034-editable-local-identity.md) 取代。

2026-10-27。范围与决策见 [ADR 0033](../adr/0033-code-identity-and-ticket-number.md)；目标契约见[总规格](standalone-site-spec.md#本厂身份初始化与管理)与[业务规格](weighing-business-spec.md)。

## 代码范围

- 数据库：新增 schema migration `00021_code_identity.sql`，就地回填 `site_code`/`scale_code` 后删除数字 `site_id`/`scale_id` 列，重建主键、外键、唯一约束与索引；`wb_site` 以编码为主键，`wb_scale` 以 `(site_code, code)` 为主键（单库单厂，`code` 保持库内唯一以支撑按编码的子表外键）。`wb_local_identity` 改存编码，`wb_number_sequence` 主键加入 `site_code`。迁移不可自动回滚，回滚须恢复升级前备份。种子 `00006`／`00008`／`00013` 增加按 schema 分支的兼容写法，使新 schema 全新安装与旧 schema 回放都能执行；历史语句保留在旧分支中。
- Server：`localsite` 身份与业务主键分配器去厂区乘数；`weighing` 全部模型、仓储、服务、处理器改用编码；`siteScope` 按编码校验归属；capabilities 返回 `localSiteCode`／`localScaleCode`；`agentbridge`、`agentproto`、`agentprinter`、`agentconfig`、`agentmaintenance` 与 Agent 命令行改用编码；`agent.hello.ack` 以 `siteCode`／`scaleCode` 替换数字字段，Server／Agent／mock 须协同升级；旧数字前缀图片引用只读兼容。
- 初始化：取消启动时自动建立身份；未配置时 Server 可启动、可登录，但不办理正式业务、不接入 Agent；管理员在配置管理填写预分配厂区编码（默认地磅编码 `WB01`），事务内建立厂区、地磅、绑定与管理员授权；已有业务数据但缺身份时拒绝重建。
- 单号：过磅单号 `厂区编码-YYYYMMDD-流水号`（至少四位，超出自动扩展），北京时间日期，`wb_number_sequence` 原子分配，与出单同事务；任务号同样改为 `RW-厂区编码-日期-流水号`；补打沿用原号，作废重开新号，历史单号不改写。恢复防重复：库内当日最大单号兜底 + 数据目录 `ticket-watermark.json` 高水位（仅恢复数据库时仍防重用）+ 配置管理“流水号水位”只升不降设置（留审计）。
- Web：`src/api`、`types`、store、工作台 URL／窗口／会话／设备、列表厂区列与权限判断全部改用编码；配置管理新增“本厂身份”（首次填写编码、编码只读、名称维护）与“流水号水位”卡片；过磅单标题样张改用新单号格式。

## 软件验证（2026-10-27）

- `task backend:check`（`go test ./...`、`go vet ./...`）：通过。
- `task db:check`（后端检查 + 完整 PostgreSQL 集成，隔离临时实例）：通过；最终复跑集成 69.2s，0 失败。覆盖恢复后不复用单号、旧 schema（含预 seed 旧库）升级回填、仅 `system:config` 角色完成身份与水位配置、相机预览、旧图片引用、监控、通知迁移与流水号水位边界。
- `task frontend:lint`、`task frontend:build`：通过。
- `task backend:swagger`：通过并重新生成 API 文档。
- 浏览器夹具回归：`test:business-layout`、`test:dashboard`、`test:operator-navigation`、`test:menu-order`、`test:local-management`、`test:weighing:camera-live`、`test:weighing:agent-capture` 通过。
- 真实 API + 临时 PostgreSQL 冒烟：`site-configuration-smoke.mjs`（首次填写编码、重复初始化拒绝、标题预览与保存、停用保护、390px）、`weighing-backend-smoke.mjs`（双方向称重、冻结采集、权限、不可变单据、更正重开、列表／CSV、Agent 打印、识别证据、真实账号接管）通过。
- 对抗式评审（迁移、身份、编号、协议、前端五个维度）确认的问题已修复：身份与水位接口改由 `system:config` 或 `weighing:site:manage` 任一权限放行；名称保存原样保留厂区启停状态；单据列表厂区列使用出单时保存的 `site_name`，任务／原始记录显示编码；`ticket-watermark.json` 改为临时文件原子替换并 fsync、损坏或为空时拒绝发号，水位取值限定大于 0 且不超过 BIGINT 安全上限；图片编号按结构解析，支持含下划线的地磅编码；恢复 Agent 维护状态文件名；协议提升到 v3 并在 hello 阶段拒绝旧版本；00021 为无法解析的旧编码占位行给出可操作处理。修复后前端 lint／build、`site-configuration-smoke.mjs`、`weighing-backend-smoke.mjs`、`local-management`／`business-layout`／`dashboard`／`agent-capture`／`operator-navigation`／`menu-order` 全部重新通过；`task backend:check` 与完整 PostgreSQL 集成（69.2s，0 失败）复跑通过。

## 未验证与边界

- 未在真实旧业务库上执行 00021；升级测试使用隔离临时 PostgreSQL（全新库全量迁移 + 旧 schema 数据回填用例）。现场升级前须备份并按 [部署说明](../device/local-deployment.md) 演练。
- 未连接真实仪表／相机／MediaMTX／打印机；本次不增加现场验收结论。
- 旧 v2 及更早 Agent 与旧维护连接文件不能继续用于正式称重（hello 阶段返回 `agent_protocol_unsupported`），须与 Server 同步升级到协议 v3；旧数字身份的打印账本在首次连接时会原子改写为新编码身份并继续使用，历史去重键不改写。
- 备份恢复后的正式操作流程仍待业务确认（当前规格暂缓决策）；实现仅提供只升不降的安全网，整体恢复数据目录后仍需人工核实已发单号。
- 业务规格中已确认但未实施的其他事项（单机阶段操作员默认资料维护权限、云端资料与单据同步、资料编码 `厂区编码-M/P/C-0001` 等）不在本次范围。
