# 后端 Maven 多模块边界

Status: accepted

后端将从单个 Spring Boot Maven 项目演进为 `base-api/` 内部的 Maven 多模块项目，同时仓库根目录继续作为前后端 monorepo 根目录。当前后端服务定位为 `system-api`，共享能力只在确实需要跨 API 服务复用时抽取。

## 决策

`base-api/pom.xml` 将成为后端 Maven 聚合父工程，并在第一阶段继续继承 `spring-boot-starter-parent`。现有 `base-api/src` 整体迁入 `base-api/system-api/src`，不改变 Java package、接口路径、响应结构和前端行为。

共享模块使用 Maven groupId `cn.ezios.base`。`base-common` 只放无业务含义的基础类型，例如响应模型、分页模型、错误码、异常和简单工具类。可复用的 Spring Boot 集成能力通过 Boot 3 自动装配 starter 提供，例如 web、auth、MyBatis starter；各 API 服务按需引入。

`system-api` 拥有认证签发和系统管理能力，包括用户、角色、菜单、部门、配置管理、SQL 初始化脚本、文件上传、登录日志和操作日志。其他 API 服务不得依赖 `system-api` 的实现模块，只能消费共享 starter、共享 common 类型、token 校验能力，或显式的服务/client 契约。

认证继续沿用 Sa-Token 路线。如果多个 API 服务需要校验同一登录态，将通过共享 Sa-Token 存储（例如 Redis）实现，而不是在 Maven 拆分时同时切换认证模型。

## 备选方案

曾考虑把仓库根目录作为 Maven parent，但仓库同时包含 React 前端，不应把整个 monorepo 强行纳入 Maven 语义，因此拒绝。

曾考虑把认证拆成独立 auth 服务，但当前登录签发依赖系统用户、角色、菜单和登录日志，第一阶段独立拆出会过早引入跨服务边界，因此拒绝。

曾考虑使用一个大的 framework 模块，但服务只需要少量公共能力时，不应被迫引入 Web、MyBatis、Sa-Token、OpenAPI 等整套基础设施，因此拒绝。

曾考虑立刻加入依赖约束工具，但第一阶段优先保持迁移低复杂度。初期使用 Maven dependency management 即可；如果后续模块边界开始漂移，再考虑 Maven Enforcer 或 ArchUnit。

## 后果

第一阶段迁移验收以行为不变为核心：`react-admin` 对后端结构调整无感，现有 `/api/**` 路径继续可用，`Authorization: Bearer ...` 继续作为 token 契约，业务 API 默认响应结构仍为 `ApiResponse<T>`。

测试应逐步做到不依赖外部开发 MySQL。common 和 starter 模块避免数据库相关测试；可启动 API 服务的集成测试可以使用 Testcontainers MySQL。

本 ADR 只记录架构形态和模块归属。未来服务设想在成为真实项目领域概念前，不写入 `CONTEXT.md` 作为领域术语。
