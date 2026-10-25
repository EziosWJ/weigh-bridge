---
status: accepted
---

> **原始决策档案**：保留各次讨论、补充和当时交付事实；现行有效决定已整理到[同号摘要](../../adr/0028-single-local-runtime-identity.md)，开发行为以当前规格为准。

# 单一本地运行形态与一次性厂区身份

> 2026-10-24 最新部分替代：[ADR 0030](../../adr/0030-local-device-responsibilities-and-configuration.md) 取消 Agent 配对及密钥，将本厂入口并入系统配置管理、相机迁 Server；单一本厂及一次性身份继续有效。本次设备改造保留现有业务库，不复用下文旧模式清库切换。新设备规格已确认，代码待实施；ADR 0029 的初始化简化已有代码及软件验证记录。

> 2026-10-24 部分替代：下文“两名实名操作员”启用门槛及初始化必填 `serverUrl` 已被 [ADR 0029](0029-local-initialization-and-agent-connection.md) 替代。旧正文保留决策历史；其余单一本厂架构按后续有效补充执行。

2026-10-24，决定每套业务服务只有一种本厂运行形态，彻底移除中心、`site`、`standalone` 与空角色兼容分支。此前多实例同步方案和双模式自治方案都不再是当前架构；中心能力暂缓，未来如有需要另行决策。单厂身份通过专用数据库单例记录一次性初始化，避免将部署身份混入可编辑配置或依赖中心审批；旧业务库中的历史业务数据按新方案整体移除并重建，不做旧模式在线转换。

业务数据库使用 `wb_local_identity` 单例（`id=1`），保存 `site_id`、`next_id`、`initialized`，不保存角色。既有 `wb_site` 保留并绑定该身份，厂区身份与编号初始化后锁定，名称等资料可编辑。本厂配置页保留为配置管理入口，配置管理只指向唯一身份和资料，不重复保存其值。初始化端点为 `POST /api/weighing/site-config/initialize`，请求包含 `siteId`（1..255）、`code`、`name`、`scaleCode`、`scaleName`、`agentId`、`serverUrl`；单次事务成功后无需重启服务，后续初始化必须拒绝。无身份时 API 仍可启动，允许登录和管理；正式业务须先具备两名实名操作员及后续 RBAC 授权。

能力描述只返回 `localSiteId`、`configured`、`initialized`，不再包含 `nodeRole`；`GET /api/weighing/local-status` 返回 `{siteId, configured, initialized}`。旧节点同步、审批和下发路由删除。历史 Goose migration 文件不改写；新增迁移删除同步表及旧示例 seed。清空并重建业务库是当前切换路径，生产遗留业务数据不做兼容迁移。

设备 Agent 继续独立运行并保留串口、相机、打印参数和打印账本；切换时只重新配对 Agent 连接身份，不重建设备参数或打印账本。软件验证、部署演练和现场验收分别登记，本文决策不表示这些验收已通过。
