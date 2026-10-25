# 系统基础能力参考

仅在修改对应模块时读取。现有系统管理、鉴权、通知、上传和监控沿用既有能力及权限；这些资料不新增当前地磅工作项。旧阶段、旧数据库和源码路径以当前代码及 PostgreSQL 总规格为准，完成证据见链接的原记录。

| 主题 | 规则与操作 | 取舍或历史证据 |
| --- | --- | --- |
| 配置、API 与数据库 | [Server](../../server/README.md)、[migration](../../server/migrations/README.md)、[开发初始化](../development-initialization.md) | [初始化规格](../development-initialization-spec.md)、ADR 0004／0005／0024 |
| 审计、通知 | [通知细则](../notification-enhancement-spec.md) | ADR 0006／0007／0012；[通知实现](../history/platform/notification-implementation.md) |
| 上传 | [上传 Pattern](../../web/design-system/patterns/upload.md) | ADR 0008；[上传需求与验证](../history/platform/file-upload-plan.md) |
| 登录防护 | 既有认证与 RBAC 不因地磅改造改变 | ADR 0009 |
| 可选宿主机监控 | [部署](../server-monitoring-deployment.md)、[模块规格](../server-monitoring-spec.md)、[可选启用与 Windows 扩展](../server-monitoring-extension-spec.md) | ADR 0011；[基础验证](../history/platform/server-monitoring-implementation.md)、[扩展验证](../history/platform/server-monitoring-extension-validation.md) |
| 菜单搜索 | [页面 Pattern](../../web/design-system/patterns/README.md) | [实现与验证](../history/platform/menu-search-plan.md) |

ADR 的当前适用范围和原文链接统一见[决策索引](../adr/README.md)。历史工单属原仓库，不能按同号套用到当前仓库，也不因 ready 标签自动启动工作。
