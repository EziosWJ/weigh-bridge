# ADR-0004: Go 后端架构与数据库兼容策略

## Status

Accepted

数据库兼容与部署策略中有关 SQLite 支持级别的内容，已由 [ADR-0010](0010-sqlite-production-support.md) 修订。

## Context

本仓库已有独立的 React 管理后台和 `base-api/` Java 后端。后端迁移目标是创建 Go 服务逐步替代 Java API，同时保持前端已依赖接口的兼容性（详见 ADR-0002）。当前仓库尚未创建 Go 服务、`go.mod` 或 Go 依赖；本 ADR 是 Go 后端的已决策目标架构，不把它们描述为现状。

系统属于内部管理系统，当前以长期维护和大量 Coding Agent 协作为重点，预期并发与部署复杂度有限。长期数据库兼容目标是 PostgreSQL、MySQL 和 SQLite；首发运行和集成测试只承诺 PostgreSQL。即使如此，业务层也不应无必要绑定 PostgreSQL。当前 Java 服务的默认配置仍使用 MySQL，且 Go 迁移尚未落地。

未来业务规模扩大时，可按业务模块逐步拆分为独立 REST 服务并通过 API Gateway 对前端提供统一入口。当前不具备引入服务注册、RPC、Kubernetes、配置中心、分布式事务、消息队列、Redis 或微服务治理框架的真实需求。

## Decision

### 1. 架构

采用模块化单体，代码按业务模块组织。清晰的模块边界为未来服务拆分做准备，但当前不实施微服务基础设施。

首次创建 `base-go-api/` 时使用 `cmd/api`、`cmd/migrate`、`configs`、`migrations`、`docs`、`internal/app`、`internal/config`、`internal/platform/{database,http}` 与首个 `internal/auth` 模块。测试与模块代码同目录放置；不创建全局 controller/service/repository 目录。

### 2. Web

Go 后端使用 Gin。Handler 只处理 HTTP 参数、DTO、校验、状态码及统一 API Response；业务逻辑进入 Service。新接口优先使用 `/api/v1/...`；迁移既有接口时，ADR-0002 的 `/api/**` 兼容契约优先，直到另行决定版本切换。Handler 与 DTO 注释通过 `swaggo/swag` 生成并提交 Swagger 2.0 文档，Swagger UI 仅在开发环境开放。CORS 默认允许跨域 Bearer Token 请求且不启用 Cookie 凭据；可由精确 `allowed_origins` 配置收紧来源范围，不使用允许凭据的通配来源。

### 3. 数据访问

使用 GORM 加 Go 标准 `database/sql`，而不是以 sqlc 加 pgx 为主数据访问方案。选择原因是本项目优先保障 PostgreSQL、MySQL、SQLite 的兼容性，而非单一 PostgreSQL 的最优体验。

### 4. Repository

数据访问必须通过 Repository 边界。Service 不直接依赖 GORM；Repository 先提供统一 GORM 实现，只有真实出现数据库方言差异时才增加局部的 dialect-specific 实现。普通模块不需要额外的 Repository interface、Impl 或 Factory；只有存在多种实现或跨模块能力边界时才定义 interface。

### 5. 数据库兼容

为后续 PostgreSQL、MySQL、SQLite 兼容，优先采用三种数据库的公共能力子集：普通 CRUD、事务、分页、排序、JOIN、聚合、常规约束与基础标量类型。数据库特有 SQL、数据类型或函数不得扩散到 Handler 和 Service；确有必要时必须局部化、说明原因并有测试覆盖。

实体主键延续现有 Java API 与前端的数据形态，使用数据库生成的 `int64` 数值 ID。PostgreSQL 使用 identity/sequence，业务层不依赖具体生成语法；迁移不改为 UUID。

### 6. Migration

Schema 使用版本化 Goose migration 管理。API 进程与生产启动不自动执行 migration 或 GORM `AutoMigrate()`；migration 由独立的显式 `migrate up` 步骤执行，Docker Compose 中使用一次性 migrate 服务并在成功后启动 API，本地开发也使用相同命令。表、索引和约束与管理员、根部门、`ADMIN`、菜单、字典、系统配置等内置数据分属独立 migration；种子数据只执行一次，不由 API 自动补种。migration 与代码一同提交，优先使用公共 DDL；只有确有方言语法差异时才增加针对性 migration。

### 7. DI

采用手工依赖组装：Config → Database → Repository → Service → Handler → Router。HTTP 应用以命名依赖对象接收完整的生产依赖，缺少任一已启用业务模块时启动失败；测试通过模块路由或应用内部基础 Router helper 构造最小场景，不以 `nil` 静默裁剪生产路由。基础设施可在启动阶段创建一次并共享指针，但不引入 DI Framework、Service Locator、业务 Singleton 或全局 Service。

### 8. 认证、日志与运行组件

目标认证为 JWT，由 Gin middleware 校验登录态。JWT 使用 HS256，密钥由运行配置提供（详见 ADR-0005），包含 `sub`、`jti`、`iat`、`exp` 并校验 `issuer`、`audience`；不包含角色或菜单。角色、用户角色与角色菜单关系继续由数据库管理，`ADMIN` 是内置角色；首版不按 `permissionCode` 拦截接口。会话以 PostgreSQL `auth_session` 表持久化，JWT 的 `jti` 用于校验会话状态；登出立即撤销当前会话。该设计不引入 Redis、Casbin、ABAC 或其他复杂权限引擎。应用日志使用 `log/slog`，业务审计日志落库：middleware 将 request_id、IP、User-Agent 写入标准 `context.Context`，Service 显式记录审计，Repository 持久化；认证记录成功与失败登录，其他操作只在业务成功后记录。`/health` 只检查进程存活，`/ready` 检查 PostgreSQL 并在不可用时返回 503，`/metrics` 不要求 JWT、仅经内部网络或反向代理白名单供 Prometheus 抓取且不应用默认 CORS。少量任务优先使用 Go 原生 timer/ticker；文件先使用本地文件系统和 Docker Volume，文件服务与具体存储实现解耦，不提前引入完整 tracing 或分布式锁组件。

配置机制与凭据存储策略由 ADR-0005 更新。开发 Docker Compose 启动独立的 PostgreSQL 命名 volume，并按 PostgreSQL、migrate、API 的顺序运行；数据库仅绑定本机端口。实际部署经外部 PostgreSQL 运行 migrate 与 API，不依赖 Compose 数据库容器。


### 9. Future microservices

未来拆分时，优先将现有业务模块边界转化为服务边界；服务间主要采用 REST，前端通过 API Gateway 保持外部 URL 兼容。当前阶段不提前引入 Gateway、注册中心或其他微服务基础设施。

## Consequences

正向影响：

- 普通 CRUD 对三种数据库有较高复用率，数据库切换主要局限在 infrastructure/Repository。
- Coding Agent 有清晰的 Handler、Service、Repository 修改边界，架构保持显式且易维护。
- 模块可逐步演变为 REST 服务，不需要先承受 IoC 或微服务基础设施复杂度。

代价：

- GORM 不能完全屏蔽数据库行为差异；高级 SQL 仍可能需要方言适配。
- 必须维护真实的 PostgreSQL、MySQL、SQLite 集成测试，不能只测试其中一种。
- 不能随意采用 PostgreSQL 高级能力；某些场景性能可能不及手写 PostgreSQL SQL。
- migration 可能需要少量数据库差异版本。

## Alternatives Considered

### sqlc + pgx

优点是 PostgreSQL 下类型安全、SQL 显式、性能和可控性优秀，也利于 Agent 编写 SQL。未选择是因为 sqlc 加 pgx 会使数据访问模型显著偏向 PostgreSQL，不满足本项目三数据库兼容优先的目标。

### GORM without Repository

优点是代码量少。未选择是因为 Service 会与 ORM 强耦合，未来出现方言差异时难以隔离，也弱化模块边界。

### 完全手写 database/sql

优点是控制力最高。未选择是因为三数据库下 CRUD 的重复工作较多，不符合快速开发和长期维护的目标。

### 使用微服务框架

未选择是因为当前业务规模和单实例部署模式不足以支撑额外复杂度。

## Compatibility Policy

数据库兼容不是理论上能建立连接，而是对应数据库须经过真实集成测试。首发仅以 PostgreSQL 验收；MySQL、SQLite 是后续兼容目标，未通过对应真实集成测试前不得宣称支持。三数据库的 Level 1 验收范围为 CRUD、事务、分页、排序、普通 JOIN、聚合、权限、审批和审计日志。高级数据库能力不默认列入兼容承诺。

单元测试不连接数据库。PostgreSQL 集成测试使用 Docker 提供的一次性隔离实例，验证 migration、Repository 与认证会话；自动化测试不得连接实际部署 DSN 或其数据。

## Revisit Conditions

出现以下任一情况时重新评估本 ADR：

- 只剩 PostgreSQL 一个目标数据库，或出现大量 PostgreSQL 专属查询需求。
- GORM 成为明显性能瓶颈，或 Repository 内的方言实现持续增加。
- 引入 DM8、Kingbase 等新数据库体系。
- 系统正式拆成多个独立服务，权限模型显著复杂化，或出现高并发、分布式需求。
