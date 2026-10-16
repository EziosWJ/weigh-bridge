你需要基于当前 Go 项目的实际代码和目录结构，完善项目级上下文文档，并建立 ADR（Architecture Decision Record）记录当前已经确定的架构约定。

不要直接按照模板覆盖现有内容。先阅读项目代码、`go.mod`、现有 README、context、配置、数据库相关代码和已有 docs，再结合下面已经确定的架构决策进行整理。

本次任务不要求大规模修改业务代码，重点是建立清晰、可持续维护的项目约定，方便后续 Agent 持续开发时遵守。

---

# 一、完善 context.md

首先查找项目中现有的：

* `context.md`
* `CONTEXT.md`
* `.context.md`
* 或其他承担 Agent 项目上下文作用的文件

如果已有 `context.md`，在原文件基础上完善，不要丢失仍然有效的项目背景。

如果不存在，则在项目根目录创建：

`context.md`

该文件的目标是：

> 让一个第一次进入项目的开发者或 Coding Agent，在阅读 context.md 后，能够快速理解项目定位、技术栈、目录边界、编码原则和架构约束。

至少补充以下内容。

## 1. 项目定位

当前后端采用 Go。

当前阶段采用：

**模块化单体（Modular Monolith）**

而不是一开始拆成多个微服务。

未来业务规模扩大后，可以按业务模块逐步拆成独立 REST 服务，并统一通过 API Gateway 对前端提供接口。

不要为了未来可能拆微服务，现在提前引入：

* 服务注册中心
* RPC 框架
* Kubernetes
* 配置中心
* 分布式事务框架
* MQ
* Redis
* 微服务治理框架

除非未来出现真实需求，并通过新的 ADR 决策后再引入。

前端是已有的独立 React 项目。

Go 后端只负责 REST API。

API 建议统一使用：

`/api/v1/...`

未来拆服务后，由 Gateway 保持对前端 URL 的兼容。

---

# 二、当前核心技术栈

记录实际项目已经使用的依赖，并结合下面目标架构。

Web：

* Gin

数据库访问：

* GORM
* Go 标准 `database/sql`

目标数据库：

* PostgreSQL
* MySQL
* SQLite

其中 PostgreSQL 可以作为默认开发/部署数据库，但代码设计不得无必要地绑定 PostgreSQL 特性。

不采用：

* sqlc 作为主数据访问方案
* pgxpool 作为全局数据库抽象
* PostgreSQL-only Repository

原因：

项目更重视 PostgreSQL / MySQL / SQLite 三种数据库之间的兼容性。

GORM 的定位是：

> 降低三种数据库之间普通 CRUD 和常规查询的差异。

但需要明确：

> ORM 不能保证所有数据库行为完全一致。

仍然需要避免数据库专属 SQL 和专属数据类型向业务层扩散。

---

# 三、数据库兼容原则

这是项目的重要架构约束，需要在 context.md 中单独列出。

## 1. 默认使用公共能力子集

业务代码优先使用三种数据库都能稳定支持的能力：

* CRUD
* 普通事务
* WHERE
* JOIN
* GROUP BY
* ORDER BY
* LIMIT/OFFSET 类型分页
* 普通索引
* 唯一约束
* 普通外键
* COUNT / SUM / AVG
* LIKE
* IN
* NULL
* 标准字符串、整数、浮点、布尔、时间等类型

不要为了开发方便，无必要使用 PostgreSQL/MySQL 专属能力。

例如谨慎或禁止直接散落使用：

* PostgreSQL JSONB
* ARRAY
* ILIKE
* RETURNING
* DISTINCT ON
* PostgreSQL Extension
* PostgreSQL 专属 UUID 类型
* MySQL 专属函数
* MySQL ENUM
* 特定数据库 UPSERT 语法
* 特定数据库全文搜索
* 数据库存储过程

确实存在业务需求时，可以使用，但必须隔离在数据库访问层，并通过 ADR 或代码注释说明原因。

## 2. Service 不依赖具体数据库

禁止：

```go
type UserService struct {
    db *gorm.DB
}
```

业务 Service 不直接编写 GORM 查询。

推荐：

```text
Handler
    ↓
Service
    ↓
Repository
    ↓
GORM
    ↓
database/sql
    ↓
PostgreSQL / MySQL / SQLite
```

Repository 负责：

* 查询
* 保存
* 数据持久化
* 事务相关数据库操作

Service 负责：

* 业务规则
* 状态流转
* 业务校验
* 业务编排

Handler 负责：

* HTTP 参数
* DTO
* 参数校验
* HTTP 状态码
* API Response

不要把项目做成 Java 式大量空壳：

* XXXRepository
* XXXRepositoryImpl
* XXXRepositoryFactory

普通模块可以直接：

```go
type UserRepository struct {
    db *gorm.DB
}
```

只有真正存在多种实现或跨模块能力边界时，再定义 interface。

## 3. 特殊数据库逻辑必须局部化

如果某个高级查询无法通过统一 GORM 表达式实现：

允许在 Repository 层提供：

* 通用 GORM 实现
* 必要的 Dialect-specific SQL

数据库判断不得进入 Service 或 Handler。

禁止出现：

```go
if databaseType == "postgres" {
    // business logic
}
```

这类判断只能存在于 database/repository/infrastructure 层。

---

# 四、数据库初始化和连接池

数据库组件属于长生命周期组件。

启动阶段创建一次 GORM DB：

```text
App 生命周期
    ↓
GORM DB
    ↓
database/sql Pool
```

Repository 和 Service 持有的不是某一条 TCP 数据库连接。

底层连接池可以：

* 创建连接
* 回收连接
* 连接失效
* 重建连接

而上层 Repository / Service 不需要重新构造。

连接池统一通过：

```go
sqlDB, err := gormDB.DB()
```

取得 `*sql.DB` 后配置，例如：

* MaxOpenConns
* MaxIdleConns
* ConnMaxLifetime
* ConnMaxIdleTime

不要额外引入第三方数据库连接池框架。

---

# 五、数据库配置

数据库类型必须配置化，例如：

```yaml
database:
  driver: postgres
  dsn: ...
```

driver 预留：

* postgres
* mysql
* sqlite

数据库初始化逻辑集中在 infrastructure/database 或类似明确位置。

业务代码不得判断 driver。

---

# 六、Migration

数据库 Schema 变更必须通过 Migration 管理。

当前优先考虑 Goose。

禁止将：

`AutoMigrate()`

作为正式生产环境数据库版本管理机制。

GORM AutoMigrate 可以用于本地快速开发或测试，但不能替代正式 migration。

由于需要支持 PostgreSQL / MySQL / SQLite：

优先编写三种数据库都兼容的 DDL。

确实存在语法差异时，可以按数据库提供差异 migration，但不要提前复制三整套完全相同的 migration。

建议遵循：

```text
公共 migration
        ↓
有数据库差异？
    ├─ 否 → 共用
    └─ 是 → dialect-specific migration
```

Migration 必须和代码版本一起进入 Git。

---

# 七、模块组织

项目按业务模块组织优先于纯技术分层。

倾向：

```text
internal/
├── app/
├── config/
├── auth/
├── user/
├── xxx/
├── audit/
├── job/
├── file/
├── middleware/
└── platform/
    └── database/
```

而不是：

```text
controller/
service/
repository/
entity/
dto/
```

把所有业务模块拆散。

每个业务模块可以根据需要包含：

* handler.go
* service.go
* repository.go
* dto.go
* model.go

但不要为了形式完整强制每个模块都有所有文件。

---

# 八、依赖管理

采用显式依赖和手工组装。

依赖主要在：

`internal/app`

或项目现有 bootstrap/composition root 中集中构建。

例如：

```text
Config
  ↓
Database
  ↓
Repositories
  ↓
Services
  ↓
Handlers
  ↓
Router
```

暂不使用：

* Fx
* Dig
* Wire
* Service Locator
* 业务 Singleton

基础设施可以在启动阶段创建一次，并共享其指针。

注意：

> 整个程序只有一个实例，不等于需要实现 Singleton Pattern。

禁止为了方便创建：

```go
var GlobalUserService *UserService
var GlobalOrderService *OrderService
```

---

# 九、API 与认证

当前认证采用：

* JWT
* 简单 Role 权限

目前不引入复杂权限引擎。

例如：

* admin
* user

通过 Gin middleware 做认证和角色检查。

当前不需要：

* Casbin
* ABAC
* 多租户权限
* 组织树数据权限

未来需求出现后另开 ADR。

API 以 Go Handler / DTO 为事实来源，并自动维护 Swagger/OpenAPI 文档。

统一 API Response、错误码、分页格式。

已有 React 脚手架的接口规范需要兼容，但允许在当前 Go 后端建设阶段进行一次规范化调整。

---

# 十、日志与审计

区分：

## 应用日志

使用：

`log/slog`

记录：

* request_id
* method
* path
* status
* latency
* user_id
* error

## 业务审计日志

登录、退出以及关键业务操作落数据库审计表。

例如：

* 新增
* 修改
* 删除
* 审核
* 导入
* 导出

应用日志不能替代业务审计日志。

---

# 十一、审批

目前只存在简单状态流转，例如：

```text
Pending
   ↓
Approved / Rejected
```

直接使用明确业务状态和 Service 方法实现。

优先：

* Approve()
* Reject()

不要设计成万能：

* UpdateStatus()

目前不引入：

* BPMN
* 工作流引擎
* Temporal
* Flowable 类系统

---

# 十二、定时任务

当前只有少量：

* 数据同步
* 状态检查
* 日报
* 周期任务

简单任务优先使用 Go 原生：

* time.Timer
* time.Ticker

如果需要 cron 表达式，再引入轻量 cron 库。

部分任务要求单实例执行。

由于项目现在强调 PostgreSQL / MySQL / SQLite 兼容：

不要将 PostgreSQL advisory lock 作为通用任务锁方案。

如果确实需要多实例调度锁，应采用数据库无关的：

* lock table
* lease
* 唯一任务键
* 事务 + 超时机制

并单独设计。

当前 Docker Compose 单实例部署情况下，不要提前引入 Redis 分布式锁。

---

# 十三、其他组件约定

配置：

* dev / test / prod 多环境
* YAML + 环境变量覆盖
* 可使用 Koanf
* 密码和密钥不要进入 Git

监控：

第一阶段提供：

* `/health`
* `/ready`
* `/metrics`

使用 Prometheus client。

当前不引入完整 OpenTelemetry tracing。

文件：

第一阶段使用本地文件系统 + Docker Volume。

文件 Service 应尽量与具体存储方式解耦，为未来 MinIO/S3 保留替换空间。

Excel：

使用 Excelize。

部署：

当前：

* Docker
* Docker Compose
* PostgreSQL/MySQL/SQLite 按部署模式选择

当前不需要 Kubernetes。

---

# 十四、Agent Coding 约定

context.md 中增加明确的 Agent 开发规则。

Agent 修改代码时应遵守：

1. 先理解现有模块边界。
2. 优先修改现有代码，不随意新建新的抽象层。
3. 不随意引入新依赖。
4. 新依赖需要说明解决什么问题。
5. Handler 不写业务逻辑。
6. Handler 不直接操作数据库。
7. Service 不依赖 Gin Context，应使用 `context.Context`。
8. Service 不依赖具体数据库。
9. Repository 负责数据访问。
10. 不创建业务 Singleton。
11. 不创建全局 Service。
12. 不为了“以后可能需要”提前实现复杂架构。
13. 不使用 PostgreSQL/MySQL 专属特性，除非确有必要。
14. 数据库专属实现必须隔离。
15. 修改数据库 Schema 必须增加 migration。
16. 修改 REST API 时同步更新 Swagger/API 文档。
17. 新增业务应优先在已有业务 package 内聚完成。
18. 不修改 generated code。
19. 完成修改后至少执行：

```bash
go fmt ./...
go test ./...
go vet ./...
golangci-lint run
```

如果项目当前环境不支持其中某个命令，应说明原因，而不是跳过后声称成功。

---

# 二、创建 ADR

检查项目是否已有 ADR 目录和命名规范。

如果已有：

遵循项目现有 ADR 规范和编号。

如果不存在：

创建：

`docs/adr/`

并创建：

`docs/adr/0004-backend-architecture-and-database-strategy.md`

ADR 使用标准结构：

# ADR-0004: Go 后端架构与数据库兼容策略

## Status

Accepted

## Context

说明：

项目是已有 React 前端对应的新 Go 后端。

当前采用模块化单体。

未来可能拆成多个 REST 服务，通过 API Gateway 对外。

项目属于内部管理系统，低并发，但需要长期维护并大量使用 Coding Agent 开发。

数据库希望兼容：

* PostgreSQL
* MySQL
* SQLite

因此数据访问层不能过度绑定 PostgreSQL。

同时项目希望保持 Go 简单、显式的工程风格，不复制 Spring Boot 的复杂依赖和容器模型。

## Decision

明确记录以下决策：

### 1. 架构

采用模块化单体。

按业务模块组织代码。

通过明确模块边界，为未来拆微服务做准备。

不提前实施微服务基础设施。

### 2. Web

使用 Gin。

Handler 只负责 HTTP 层。

业务逻辑进入 Service。

### 3. 数据访问

使用：

GORM + database/sql

而不是：

sqlc + pgx 作为项目主数据访问方案。

主要原因是项目把 PostgreSQL / MySQL / SQLite 兼容性置于 PostgreSQL 单库最优体验之上。

### 4. Repository

数据库访问必须通过 Repository 边界。

Service 不直接依赖 GORM。

Repository 首先采用统一 GORM 实现。

只有真正出现数据库方言差异时，才增加 dialect-specific 实现。

### 5. 数据库兼容

优先使用三种数据库的公共能力子集。

数据库特有能力不得扩散到业务层。

任何数据库专属 SQL 应：

* 局部化
* 有明确原因
* 有测试覆盖

### 6. Migration

使用明确 migration 管理 Schema。

不以 GORM AutoMigrate 作为正式生产 Schema 管理方案。

### 7. DI

采用手工依赖组装。

暂不引入 DI Framework。

禁止业务 Singleton 和全局 Service。

### 8. Future microservices

未来拆服务时：

模块边界优先转化为服务边界。

服务之间主要使用 REST。

前端统一通过 API Gateway。

当前阶段不提前引入 Gateway/注册中心等基础设施。

## Consequences

写清楚正向影响：

* CRUD 对三种数据库有较高复用率
* 避免应用直接绑定 PostgreSQL
* 数据库切换主要局限于 infrastructure/repository
* 对 Coding Agent 更容易建立清晰修改边界
* 模块可以逐步拆微服务
* 架构简单，没有复杂 IoC 魔法

同时写清楚代价：

* GORM 无法完全屏蔽数据库差异
* 高级 SQL 仍可能需要针对数据库适配
* 必须维护真实的跨数据库测试
* 无法随意使用 PostgreSQL 特有高级能力
* 某些场景性能可能不如手写 PostgreSQL SQL
* Migration 可能需要数据库差异版本

## Alternatives Considered

至少记录：

### sqlc + pgx

优点：

* PostgreSQL 下类型安全
* SQL 显式
* Agent 编写 SQL 方便
* 性能和可控性优秀

未选择原因：

项目要求 PostgreSQL / MySQL / SQLite 多数据库兼容，sqlc + pgx 会让数据访问模型明显偏向 PostgreSQL。

### GORM without Repository

优点：

简单，代码量少。

未选择原因：

会让 Service 与 ORM 强耦合，不利于未来处理数据库方言差异，也不利于模块边界。

### 完全手写 database/sql

优点：

控制力最高。

未选择原因：

三数据库 CRUD 重复工作较多，不符合项目以 Agent 快速开发和长期维护为目标的定位。

### 使用微服务框架

未选择原因：

当前业务规模和部署模式不足以支撑额外复杂度。

## Compatibility Policy

ADR 中明确写出：

数据库兼容不是“理论上能连接”。

兼容意味着对应数据库必须经过真实集成测试。

当前目标兼容数据库：

* PostgreSQL
* MySQL
* SQLite

优先保证：

Level 1：

* CRUD
* 事务
* 分页
* 排序
* 普通 JOIN
* 聚合
* 权限
* 审批
* 审计日志

高级数据库能力不默认列入兼容承诺。

## Revisit Conditions

出现以下情况时重新评估此 ADR：

* 只剩 PostgreSQL 一个目标数据库
* 出现大量 PostgreSQL 专属查询需求
* GORM 成为明显性能瓶颈
* Repository 中数据库特定实现大量增加
* 引入 DM8、Kingbase 等新的数据库体系
* 系统正式拆成多个独立服务
* 权限模型显著复杂化
* 出现高并发或分布式需求

---

# 三、文档之间的关系

ADR 是长期架构决策记录。

`context.md` 是 Agent 当前开发上下文。

因此：

context.md 中只写：

> 当前采用 GORM + database/sql，并要求 PostgreSQL/MySQL/SQLite 兼容，详细原因见 ADR-0004。

不要把 ADR 的全部论证重复复制进 context.md。

ADR 负责记录：

* 为什么这样选
* 考虑过什么方案
* 有什么代价
* 什么情况下重新决策

context.md 负责告诉 Agent：

* 当前应该怎么写代码
* 哪些约定必须遵守

---

# 四、执行要求

完成后：

1. 检查 `context.md` 是否与真实项目现状冲突。
2. 不要虚构尚未安装的依赖为“已使用”。
3. 对于规划中但尚未实现的组件，明确标记为“目标架构”或“规划”，不要写成当前事实。
4. ADR 中可以记录已经确定的架构决策，即使对应代码尚未全部完成。
5. 不修改业务行为。
6. 不为了匹配文档进行大范围代码重构。
7. 最后输出：

   * 修改了哪些文件
   * context.md 新增/调整了哪些核心约定
   * 创建了哪个 ADR
   * 当前代码中有哪些地方与 ADR 不一致
   * 建议后续按什么顺序逐步调整，但本次不要擅自大规模重构

核心原则：

**文档描述真实现状，同时明确目标架构；不要为了让文档“看起来一致”而伪造当前项目状态。**


其他补充：
java项目连接的是mysql数据库，但我已经将数据库迁移到postgresql了：
192.168.1.48:5432
用户名：pgsql
密码：pgsql
数据库：base_project_golang