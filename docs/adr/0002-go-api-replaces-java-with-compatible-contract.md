# Go API 兼容替代 Java system-api

Status: accepted（认证内部实现部分由 ADR-0004 更新；数据库正式支持边界由 ADR-0010 修订）

本 ADR 中关于 PostgreSQL 的表述记录 Go 后端首次迁移基线；当前 SQLite 正式生产支持、单实例边界和双数据库兼容矩阵以 [ADR-0010](0010-sqlite-production-support.md) 为准。

后端迁移目标是由 `base-go-api/` 中的 Go 服务最终替代 Java `system-api`，前端无需感知后端实现变化。迁移期间保留现有 `/api/**` 路径、HTTP 方法、请求参数、`ApiResponse<T>` 响应结构、分页格式和 Bearer Token 认证；Go 端继续兼容现有逻辑数据结构，但物理存储改为 PostgreSQL 17。Java 端只作为现状和行为参考，不再为本次迁移修改，也不参与 Java/Go 双写；迁移完成后生产环境只运行 Go 服务。

本次迁移不保留现有 MySQL 数据。PostgreSQL 17 使用全新数据库，由 Go 端提供建表和初始化数据流程；因此不包含 MySQL 到 PostgreSQL 的历史数据迁移。

全新 PostgreSQL 数据库原样初始化现有内置数据，包括管理员、根部门、超级管理员角色、菜单、字典和系统配置；不迁入任何 MySQL 历史业务数据。

本次范围只覆盖当前前端已经接入 Java 的真实 `/api/**` 接口。前端“权限点管理”页面仍使用 mock 数据，暂不新增其 Go API、PostgreSQL 表或管理能力。

PostgreSQL 17 的结构使用版本化 SQL migration 管理，初始化数据与结构变更分离；管理员、根部门、`ADMIN`、菜单、字典和系统配置使用独立、只执行一次的 Goose seed migration，API 不在启动时自动补种。生产启动不使用不可控的自动建表或 ORM 自动迁移。

认证内部实现采用 JWT，由 Gin middleware 校验登录态；此决策更新了本 ADR 原先的进程内随机 Bearer Token 方案，详见 ADR-0004。JWT 使用 HS256，密钥经环境变量注入，包含 `sub`、`jti`、`iat`、`exp` 并校验 `issuer`、`audience`；不包含角色或菜单。角色、用户角色与角色菜单关系继续由数据库管理，`ADMIN` 是内置角色；首版不按 `permissionCode` 拦截接口。登录、登出、当前用户、当前菜单和受保护接口继续保持既有外部 Bearer Token 契约。会话以 PostgreSQL `auth_session` 表持久化，JWT 的 `jti` 对应会话记录；鉴权时校验会话仍有效，登出立即撤销当前会话。

为兼容现有行为，首版 JWT 与会话有效期均为 7,200 秒；同一用户允许并发登录，每次登录创建独立会话。禁用、删除用户或重置密码时撤销该用户全部会话。密码继续使用 BCrypt；现有内置管理员种子使用 BCrypt `$2a$10` 哈希。Token claims 与续期策略尚未确定，必须在实现前补充决策；当前不引入 Redis 或复杂权限引擎。

迁移时以 `react-admin` 的实际请求代码和 TypeScript 类型作为 API 契约第一来源，确保前端无需修改；Java Controller、Service 与测试仅用于补足行为和边界。核对后的契约由 Gin Handler/DTO 注释生成的 Swagger 2.0 文档和 HTTP 接口测试固定；生成文档随代码提交，Swagger UI 只在开发环境开放。

错误响应同样保持现有契约：响应体使用 `{ code, message, data }`，字段校验错误以字段名到错误消息的映射放入 `data`，认证错误保留对应 HTTP 状态。一般业务异常沿用现有“HTTP 200 且响应体 `code` 非 200”的行为，本阶段不重构错误语义。

文件 API 继续使用本地目录存储：单文件上限 50 MB，文件按日期和随机文件名保存，PostgreSQL 只保存元数据与相对路径，下载和预览接口由服务流式返回内容。部署时上传根目录必须挂载持久化卷；本阶段不引入对象存储。

登录日志与操作日志在 Go 第一版即持续写入并提供现有查询、详情和清空接口。记录范围、字段、参数脱敏和 2,000 字符截断规则保持现状，不借迁移扩展审计能力。
