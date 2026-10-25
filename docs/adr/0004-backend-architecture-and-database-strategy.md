---
status: accepted (consolidated summary)
---

# Go 后端模块边界

2026-10-24 有效决定摘要，合并已确认修订，不构成新的业务决策。[原始决定与演进记录](../history/adr/0004-backend-architecture-and-database-strategy.md)保留来源与历史事实；实现及验收统一见[当前状态](../next.md)。

## 有效决定

沿用 Go、Gin、GORM，以 handler/service/repository/model 和显式 app 装配组织模块；Service 不直接依赖 GORM，持久化事务留在 Repository 边界，业务域避免无必要的横向耦合。API 路由、响应、鉴权和审计继续保持既有契约，结构变更通过独立 Goose migration。

## 取舍

选择现有分层与显式依赖，便于局部修改、测试和维护，暂不引入通用 IoC、服务注册或微服务框架。接受 Repository 层及必要映射的代码成本。原多数据库兼容目标已经退出，当前只用 PostgreSQL；原“首版不按 permissionCode 拦截”等阶段限制不覆盖已经建立的 RBAC。

## 当前细则

[开发指导](../../CLAUDE.md)、[总规格](../requirements/standalone-site-spec.md)、[数据库决策](0024-postgresql-only.md)。
