# Server PostgreSQL 单库收敛交付

日期：2026-10-22。用户授权收敛双库实现、完善文档并移除 SQLite 要求。决策见 [ADR 0024](../adr/0024-postgresql-only.md)。

## 实现范围

- Server 运行、迁移、开发配置、数据库集成、浏览器及监控验收统一使用 PostgreSQL。移除 SQLite 驱动直接依赖、连接／锁错误处理、在线备份命令及 profile；旧 driver/profile 明确报错，避免静默连接到其他数据库。
- 保留 PostgreSQL schema/seed 的路径、版本及 SQL，API 继续不执行 AutoMigrate。26 个 SQLite 历史 SQL 原样移至 [归档](../archive/sqlite-migrations/README.md)，不再进入发布、执行及锁步检查。
- Task 和 CI 运行完整 PostgreSQL 集成集，不再按测试名称筛选造成遗漏。缺少 Docker 时明确失败。单元测试仍不要求数据库。
- 浏览器测试使用随机名称／端口的临时 PostgreSQL；监控脚本使用独立数据库容器和网络。测试均清理容器及临时进程，不使用开发或现场数据库凭据。
- Docker 构建与发布验证使用 `CGO_ENABLED=0`；Server 初始化检查不再要求 CGO／C 编译器，Dockerfile 不再安装 C 工具链或构建备份镜像。
- 更新根开发指导、README、配置示例、初始化规格、数据库设计及当前需求入口；旧 ADR 明确被替代，历史验收事实保留。本厂自治和现场设备范围不因本次数据库调整扩大。

## 保留的业务测试覆盖

| 原 SQLite 覆盖 | PostgreSQL 中的对应覆盖 |
| --- | --- |
| 综合 HTTP 业务、持久化、权限与审计 | `TestPostgresSharedHTTPBusinessContract` 及既有认证、RBAC、文件等集成测试 |
| 通知会话到期、升级保留已读历史 | `TestPostgresNotificationSessionExpiry`、`TestPostgresNotificationUpgradePreservesHistory` |
| 通知实时、公告、容量及业务入口 | 复用原 PostgreSQL 共享契约，移除重复 SQLite 调用 |
| 称重审计失败回滚、非空库 Down/Up | `TestPostgresWeighingFoundation`、`TestPostgresWeighingMasterMigrationRoundTrip` |
| 旧备份恢复后编号不复用 | `TestPostgresLocalFirst` 使用临时容器内 `pg_dump`／`pg_restore` 验证 |
| 厂区隔离、配置、监控授权 | 保留现有 PostgreSQL 契约，浏览器测试改接临时 PostgreSQL |
| SQLite PRAGMA、文件备份、busy/locked | 随该数据库运行支持退役，不作为当前产品要求 |

迁移称重回滚断言时发现旧权限测试写死单据 ID 为 1；PostgreSQL 序列在回滚后不回退，后续权限断言现使用实际生成的 ID。未改变业务授权规则。通知升级测试中显式插入历史 ID 后同步调整测试序列，避免测试夹具与真实自动编号不一致。

## 验证结果

- `task check`：通过（后端单元测试／vet、初始化命令测试、前端 lint／build）。
- `CGO_ENABLED=0 task build:check`：通过（内嵌 Web 发布构建、embedweb 测试／vet）。
- `CGO_ENABLED=0 task db:integration:postgres`：通过，25 个集成测试，186.738 秒。结合 `task check` 中通过的后端检查，覆盖 `task db:check` 的全部检查项。
- `node web/tests/site-configuration-smoke.mjs`：通过，覆盖厂区创建、配置下载、mock 归属、在线阻止停用、离线停用／启用与窄屏。
- `npm --prefix web run test:weighing:backend`：通过，覆盖两种称重方向、持久化、权限、资料、快照／单据版本、更正重开、打印反馈、查询／CSV、账号接管与多厂区。
- `task monitoring:smoke:docker`：通过，覆盖原生／Docker API、内嵌页面、宿主机监控、socket 权限、断开过期、重连与历史缺口。
- 历史 SQL 逐文件比对：26 个文件与原版本字节一致；API／migration 的运行依赖图无 SQLite 包；AGENTS.md 链接有效。

首次数据库全量检查仅称重权限断言因固定 ID 失败，修正后该测试独立重跑通过。沙箱内首次运行遇到本机端口权限限制，随后在允许本机测试服务与 Docker 的执行环境完成验证。

## 使用与切换边界

新开发环境：准备 PostgreSQL → `task init` → 填写忽略的本地连接配置并创建目标库 → `task doctor` → `task dev`。集成测试另外创建隔离数据库，要求 Docker 可用和 `postgres:17-alpine` 镜像可获取。

已有 PostgreSQL 库继续使用原 migration 历史。已有 SQLite 数据文件、真实本地配置及 Agent 打印账本均未修改；不提供自动跨库数据转换。旧 SQLite 环境应先封存数据与旧应用，再按确认的数据范围迁入 PostgreSQL；仅修改 driver 不能转换旧数据。本轮不部署现场、不提交或推送。

`go.sum` 中可能保留上游模块测试依赖的 SQLite 校验和；它们不表示 Server 运行或测试仍支持 SQLite，直接依赖及运行依赖图均已清除。
