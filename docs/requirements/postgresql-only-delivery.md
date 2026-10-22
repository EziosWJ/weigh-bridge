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

## 集成测试设施提速（2026-10-22）

本次仅调整 `server/integration` 的环境生命周期，保留此前全部 25 个业务测试，新增 1 个数据库隔离回归测试：

- 每次 Go 测试进程按需启动一个临时 PostgreSQL 容器；每次 `startPostgres` 调用创建独立空数据库，多节点及恢复节点也分别建库。仍执行真实 schema/seed migration，不复用已迁移的模板库。
- 每个测试／子测试结束删除其数据库；`DROP DATABASE ... WITH (FORCE)` 可清理测试失败遗留的连接。`TestMain` 在整套结束时删除容器和临时迁移程序，清理错误使检查失败；初始化失败也进入套件清理。
- 迁移程序按需编译一次，后续复用同一可执行文件；每次仍传入对应数据库及环境配置，保留开发／测试／生产 seed 差异和重复迁移验证。
- 迁移表检查、`pg_dump` 和 `pg_restore` 使用各自数据库名，保留多节点备份恢复验证。
- 保持串行执行；Goose 回退测试仍修改全局状态，本次未启用 `t.Parallel()`。独立浏览器和监控脚本的容器组织保持原样。

验证结果：

| 检查 | 结果 |
| --- | --- |
| `task backend:check` | 单元测试与 vet 通过 |
| `go vet -tags=integration ./integration` | 通过 |
| 隔离回归与最小迁移测试 | 通过；验证同容器不同库、表数据互不影响、带存量连接删除子测试库 |
| `CGO_ENABLED=0 task db:integration:postgres` | 26 个顶层测试全部通过，61.573 秒；包含迁移回退、通知容量、多节点同步及备份恢复 |
| 生命周期核对 | 运行中只有一个本套件临时容器，结束后无残留容器；迁移临时目录由套件清理 |

相比此前记录的 186.738 秒，本次单次测量减少 125.165 秒（约 67%，约为原来的三分之一）。这是与上一轮记录的对比，并非受控的多轮基准，耗时仍受本机负载及构建缓存影响。本次未启用可选 `LOCAL_FIRST_BROWSER=1`，不追加浏览器验收结论。后端检查最初受沙箱本机端口权限限制，随后在允许 Docker 和回环监听的执行环境通过。

日常运行受影响测试的命令见根 [README](../../README.md)；数据库改动与交付继续运行完整 `task db:check`，本次后端检查加完整数据库集成已覆盖其全部检查项。

## 默认门禁与历史同步拆分（2026-10-22，第二轮）

依据 [ADR 0023](../adr/0023-standalone-site-baseline.md)，中心与同步暂缓，不再作为当前本厂自治交付的默认必跑项目。本轮只调整测试组织，未修改业务实现、初始化校验、权限或迁移历史。

- `task backend:integration`、`task db:integration:postgres`、`task db:check` 及 CI 默认使用 `integration` 标签，运行当前本地业务及数据库回归；原有权限、厂区隔离、设备、通知容量浏览器和迁移回退覆盖仍保留。
- 原 `TestPostgresLocalFirst` 完整保留，改名为 `TestPostgresLegacyCentralSync`，增加 `integration && legacy_sync` 编译条件。使用 `task backend:integration:legacy-sync` 显式执行；CI 提供 `workflow_dispatch` 的 `legacy_sync` 开关。断网重连、中心下发／上报、同步幂等、中心审批及旧快照同步冲突断言均保留。
- 默认新增 `TestPostgresLocalBusinessRecovery`：从实际业务 HTTP、独立 mock 设备及真实 PostgreSQL 构造已完成单据、未完成任务和未决打印；备份并恢复至另一个独立数据库，比较单据、原始重量／抓拍依据、业务快照、称重记录、事件及打印记录，查询原提交结果；确认恢复后不自动完成任务、不增加单据、不向重新连接的 mock 重发打印，以及恢复旧备份后不复用备份之后生成的业务 ID／单号。
- 该恢复测试只绑定现有厂区数据身份分配器，未运行中心或同步 worker，未设置 `initialized` 或模拟中心启用。它使用既有业务 HTTP 测试入口，不证明正式独立运行模式的初始化与权限边界已完成；单厂独立初始化、无中心正式部署及完整图片／设备打印账本恢复验收，仍由本厂自治功能改造补齐。
- 迁移程序仍只编译一次，测试可执行文件不写入 VCS 构建标记，避免独立 worktree 的测试构建依赖 Git 元数据；发布构建不受影响。本轮未增加模板数据库或并行执行，先测量范围调整的实际收益。

本轮在独立 worktree `/tmp/weighbridge-standalone-test-gates`、分支 `optimize/standalone-test-gates` 上执行，基于 `f06cecd`，带入上一轮四个测试设施／文档文件；不包含原工作区正在修改的本地自治及维护整合业务代码。

验证与耗时：

| 检查 | 结果 |
| --- | --- |
| `CGO_ENABLED=0 task backend:check` | 单元测试与 vet 通过 |
| `go test -tags=integration ./integration -count=1 -timeout=20m -json`（`CGO_ENABLED=0`） | 默认 26 项全部通过，45.034 秒；等价于默认集成 Task，增加 JSON 用于逐项计时 |
| `TestPostgresLocalBusinessRecovery` | 完整集合中 6.30 秒，包含真实备份恢复及新 mock 连接后的不自动打印检查 |
| `CGO_ENABLED=0 task backend:integration:legacy-sync` | 独立历史入口通过，23.107 秒，含该进程自身环境准备 |
| `go vet -tags=integration,legacy_sync ./integration` | 通过，两组测试均编译并检查 |
| Task／CI 配置 | Task 入口可解析；workflow YAML 结构检查通过，默认集成 job 补齐通知容量测试所需 Node、锁定依赖及 Chromium；未触发远程 CI |

相比上一轮最近实测的 52.590 秒，默认门禁本次减少 7.556 秒（约 14%）；与较早的 61.573 秒相比减少约 27%。这些是单次测量，且本轮明确调整了默认验收范围；历史同步仍可独立执行，不将范围变化全部归为测试执行效率提升。新增恢复测试保留了必要的本地验证，因此没有省去旧测试的全部耗时。

本轮未启用历史测试的可选 `LOCAL_FIRST_BROWSER=1`；默认通知容量测试内的浏览器验证实际执行并通过。独立 worktree 的前端测试依赖按原锁文件安装；无前端源码或依赖版本变更。默认集合在新进程中创建并清理临时容器，各次检查结束后核对容器清理。

### 分段计时复测

使用独立 worktree 的 `/tmp` Go overlay 插桩和 CPU profile；仅增加测量，未调整业务代码或断言。26 项全部通过，本次 **49.318 秒**，此前无插桩单次为 **45.034 秒**；计时受运行负载及采样影响，以下是复测分布，不是此前一轮的精确追踪。

| 环节 | 耗时 |
| --- | ---: |
| 执行迁移命令（29 次） | 16.885 秒 |
| 容器启动及删除 | 3.872 秒 |
| 创建／删除独立数据库（各 30 次） | 3.550 秒 |
| 编译迁移程序（1 次） | 1.111 秒 |
| 直接 Goose 回退／升级（10 次） | 1.669 秒 |
| 数据库备份／恢复 | 0.975 秒 |
| 业务测试设备状态等待（18 次） | 5.892 秒 |
| 通知容量浏览器子进程 | 3.826 秒 |
| 流关闭等待（2 次，含会话过期验证） | 1.561 秒 |
| 其余业务 HTTP、SSE 等待及其他开销 | 9.977 秒 |

设备等待中，本地恢复约 3.064 秒，称重完整流程约 2.828 秒；现有独立 mock 每 500 毫秒上报状态。通知容量浏览器子进程包括 Vite／Chromium 启停、页面加载、截图等，实际页面通知更新约 95 毫秒。备份恢复命令本身合计约 0.975 秒。

HTTP 辅助函数另观测到 384 次请求累计约 6.535 秒（已包含在余项中，并发耗时可能重叠），其中 38 次登录约 2.367 秒。Go 测试进程 CPU profile 采样到 5.62 秒 CPU，主要热点是密码哈希与 SHA256；此数据不包括数据库、迁移程序及浏览器子进程。

后续提速优先评估模板数据库与 mock 控制动作后的及时上报；迁移契约仍使用空库，稳定与鉴权行为保持。原始测量材料位于 `/tmp/weighbridge-local-profile/`，未将插桩加入源码。

## 业务测试模板数据库（2026-10-22，第三轮）

本轮继续在 `optimize/standalone-test-gates` 独立 worktree 实施，仅调整测试设施及调用点，不修改业务、migration SQL 或依赖版本。

- 新增 `startBusinessPostgres`：每个测试进程按需创建一个独立模板数据库，以 test 环境执行一次真实 schema／seed migration；迁移子进程退出并关闭连接后，将模板设为不允许连接。普通业务测试从模板克隆各自独立的数据库，不在用例中重复迁移。
- 模板只在本次临时容器中存活，不在测试之间共享写入，不跨运行缓存；其生命周期由套件管理，不挂在首次使用它的测试清理上。各克隆库仍分别删除，模板随容器结束清理。
- `startPostgres` 明确从 PostgreSQL `template0` 创建空库。空库迁移、开发／测试／生产 seed 差异、通知历史升级、称重回退／升级、监控重复 migration 验证继续使用空库及真实迁移；备份恢复目标库也保持空库。
- 本地恢复回归及普通 HTTP 业务改用模板；历史同步测试的运行节点也使用模板，恢复节点仍使用空库。不改变标签或门禁选择，不引入并行。
- 新增 `TestPostgresBusinessTemplateIsolation`：先修改并清理首个克隆的用户数据、表结构及用户序列，再创建第二个克隆；验证模板仍可用、数据／DDL／序列互不污染，schema／seed 版本和 test 种子一致。原有空库隔离测试增加“无业务表”断言。

验证与计时（合入本地 main 前，基线仍为 `f06cecd`）：

- `CGO_ENABLED=0 task backend:check`：单元测试及 vet 通过。
- 模板隔离、空库隔离、空库迁移和不同环境 seed 专项：通过。
- 完整默认集合：**27 项通过，37.492 秒**；相比上一轮无插桩的 45.034 秒减少 7.542 秒（约 16.7%），并增加 1 项模板隔离测试。单次测量仍受本机负载影响。
- `CGO_ENABLED=0 task backend:integration:legacy-sync`：通过，19.536 秒。
- `go vet -tags=integration,legacy_sync ./integration`：通过。

按默认测试调用路径，迁移子进程由此前 29 次降为 11 次（模板 1 次，迁移专项共 10 次）。测试库仍独立，迁移专项覆盖未减少。

## 使用与切换边界

新开发环境：准备 PostgreSQL → `task init` → 填写忽略的本地连接配置并创建目标库 → `task doctor` → `task dev`。集成测试另外创建隔离数据库，要求 Docker 可用和 `postgres:17-alpine` 镜像可获取。

已有 PostgreSQL 库继续使用原 migration 历史。已有 SQLite 数据文件、真实本地配置及 Agent 打印账本均未修改；不提供自动跨库数据转换。旧 SQLite 环境应先封存数据与旧应用，再按确认的数据范围迁入 PostgreSQL；仅修改 driver 不能转换旧数据。本轮不部署现场、不提交或推送。

`go.sum` 中可能保留上游模块测试依赖的 SQLite 校验和；它们不表示 Server 运行或测试仍支持 SQLite，直接依赖及运行依赖图均已清除。
