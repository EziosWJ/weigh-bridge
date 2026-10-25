> **历史档案（2026-10-24 归档）**：以下保留当时的决定、计划或验证事实；其中“当前／待实施／已授权”均指原记录时点。开发与验收使用[当前规格](../../requirements/standalone-site-spec.md)，追溯入口见[历史索引](../README.md)。旧步骤不作为本轮部署或数据操作指令。

# 实施交接 prompt

以下内容可直接交给实施智能体：

```text
请在当前 weighbridge 仓库完成“本厂初始化简化与 Agent 连接地址归位”的实现、必要验证和交付文档更新。用户已确认方案，不需要再次讨论是否取消两人门槛。中文交流，按已确认范围自主完成；不提交或推送 Git，不操作真实业务库或真实设备。

先读根 AGENTS.md、CONTEXT.md、docs/requirements/standalone-site-spec.md 和 docs/development-baseline.md，再读：
- docs/adr/0029-local-initialization-and-agent-connection.md
- docs/requirements/local-initialization-simplification-spec.md（本次实施和验收依据）
- docs/agents/development.md
前端改动前读取 web/AGENTS.md 及对应页面 Pattern。

注意：工作区已有大量未提交及未跟踪的单一本厂改造，请在其当前状态上增量实现，保留已有工作；不要 reset、清理未跟踪文件或重做旧改造。ADR 0028 原版交付已经验证，不等于本次补充已经实现。

完成以下行为：
1. 本厂身份、地磅与绑定首次事务成功即完成初始化。取消至少两名非内置实名操作员启用检查；状态查询不再统计人员或写启用标记。保留 local-status 与 capabilities 字段契约，configured/initialized 均表达配置完成，兼容已配置但旧 initialized=0 的库。无身份仍拒绝正式业务写入；现有 JWT、RBAC、账号状态、本厂操作范围及设备/业务校验继续生效，不额外自动授权。
2. 本厂配置详情、首次设置、编辑表单移除“本厂 Server 地址”及两人提示，首次 API 不要求 serverUrl；状态文案表达配置完成，不代表设备就绪。兼容保留的旧 serverUrl 字段按补充规格处理，不作为可编辑地址来源。
3. Agent 连接地址归设备配对/连接设置。agent-provision 增加显式地址输入（建议 --server-url）；首次生成要求有效地址，更新已有文件未传地址时保留已有有效值，显式传入才更新。不再读取 wb_site.server_url 或 AgentSetting.server_url 作连接依据。保持 --update-connection、0600、原子写入、敏感数据不输出，保留设备参数、维护草稿、未知行和打印账本。
4. 维护页展示 Agent 实际连接地址及同机/分机含义，遵循现有连接身份编辑与应用能力；只读时说明通过配对工具修改及显式应用/重启。离线不能依赖网页首次配对，不新增自动发现或远程离线配对服务。Server 监听配置和浏览器 API 地址不变。
5. 检查旧导出连接 API、mock、安装/发布与冒烟脚本，修复对数据库厂区地址和两人条件的残留依赖。保留既有路由、鉴权和非本次调整的响应契约；不得仅隐藏前端或将人数改成一人来替代后端门槛移除。

优先核查：
- server/internal/localsite/identity.go 的 ReadStatus
- server/internal/weighing/local_setup.go、site_handler.go、site_config_repository.go/service.go/handler.go
- server/cmd/agent-provision/main.go 及测试
- server/internal/agentmaintenance/device.go 和连接身份/草稿保护
- web/src/pages/weighing/site-config.tsx、web/src/api/site-configuration.ts、维护页实际连接区域
- server/integration/local_site_initialization_integration_test.go、local_site_business_integration_test.go
- web/tests/site-configuration-smoke.mjs、weighing-backend-smoke.mjs
- scripts/check-local-install.mjs、check-local-release.mjs 及相关发布入口

必要行为测试覆盖：无身份拒绝、初始化不填地址、并发/重复/失败回滚、一名已授权账号完成正式业务、无权限/无操作范围仍拒绝、旧 initialized=0 升级兼容、配对地址首次缺失/无效/有效、更新保留与替换地址、不再读取旧库地址、设备配置和账本保护，以及本厂页面只读/错误/390px 窄屏。原有多人接管等有意义场景继续保留，不通过批量删测试让检查通过。

按改动范围运行 task backend:check、task frontend:lint、task frontend:build、task db:check，以及受影响页面/业务闭环/配对/安装发布冒烟。隔离临时 PostgreSQL 用后清理，不连接真实业务库；长驻验证设超时并清理。新增数据库变更使用 Goose migration，不改历史 migration；本次不重建现有业务库。

完成后更新补充规格实施状态、部署命令与实际交付记录，分别说明决策、代码、软件验证和未完成的现场验收。不要把旧交付中的两人测试记录改写成本次已验证。最后报告改动、验证结果及剩余限制。
```
