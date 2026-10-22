# weighbridge · 酒厂地磅管理系统

面向酒厂车辆进出厂称重的可追溯管理。客户有两个厂区，各一台地磅及工控机；当前目标是每厂独立运行的本地地磅系统，中心建设暂缓。

**开发入口：[CONTEXT](CONTEXT.md) → [当前阶段规格](docs/requirements/standalone-site-spec.md)。** 旧需求及决策的有效范围见[基线索引](docs/development-baseline.md)，架构理由见 [ADR 0023](docs/adr/0023-standalone-site-baseline.md)。

## 当前目标与实现状态

每厂部署三个核心服务：本地业务服务（内嵌 Web）、设备服务（现有 Agent）和 PostgreSQL。两厂分别维护账号、权限和基础资料，独立办理称重、出单、打印及查询；互联网中断不影响本地正常链路。统一安装升级、地址和账号；设备维护整合后退出独立维护服务。CUPS 为打印系统配套，不额外运行生产前端服务。

```text
每厂工控机
浏览器 → 本地业务服务（内嵌 Web） → PostgreSQL／业务图片
                   ↕ 本机设备协议
                设备服务 → D12／摄像头／打印机
```

这是已确认的目标部署。现有业务闭环、厂区隔离、Agent 通信、mock 和前端内嵌发布已实现；D12 零值／故障／恢复及 A4 单张打印已有现场结果。当前 site 模式仍依赖中心初始化并限制账号资料维护，维护服务仍独立存在；本地自治改造与完整真实生产闭环尚未完成。摄像头／识别、D12 非零带载等和五联连续纸继续按现场项目验收，见[交付索引](docs/history/README.md)。

系统继续复用用户、JWT、RBAC、系统管理和审计。识别候选必须人工核对，设备重量、对应抓拍和操作员确认经本地业务校验后形成正式记录；稳定状态本身不能确认称重。互联网断开与本地设备／业务链路故障分别处理，恢复不自动补称或重打。

## 技术栈

保留源项目版本：Go 1.26（Docker 构建使用 1.26.5）、Gin、GORM、Goose、Koanf、JWT、Swagger、Prometheus；React 19、TypeScript、Vite 6、Tailwind CSS、shadcn/ui、React Router、Zustand。npm 依赖以 lockfile 为准，本阶段未升级依赖。

Server 开发、数据库测试与生产统一使用 PostgreSQL；数据库相关改动只维护一套 schema/seed 和 PostgreSQL 测试。详见 [数据库决策](docs/adr/0024-postgresql-only.md)。Agent 当前没有新增 SQLite 的要求。

## 项目目录

```text
weighbridge/
├── server/              # Go API、现有模块和测试
│   ├── cmd/             # api / migrate / dev-setup / host-collector
│   ├── configs/         # 配置与示例
│   ├── internal/        # 按原脚手架组织的模块
│   ├── migrations/      # 原 Goose 历史，原样保留
│   ├── integration/     # 数据库/API 集成测试
│   ├── docs/            # Swagger 产物
│   ├── go.mod
│   └── Dockerfile
├── web/                 # React 管理后台、tests、design-system
├── agent/               # 现场部署／维护资源及 mock 说明；设备 Go 实现位于 server/
├── protocol/            # Agent 通信契约 v1
├── docs/
│   ├── architecture/    # 当前与规划架构说明
│   ├── requirements/    # 阶段范围、验收记录
│   ├── adr/             # 当前及历史决策，效力见基线索引
│   └── agents/          # AI 协作说明
├── scripts/             # 初始化、内嵌资源、监控验证脚本
├── record/              # 保留的脚手架历史记录
├── .github/             # 原有 CI 门禁
├── .vscode/             # 调试配置
├── Taskfile.yml
├── README.md
├── CONTEXT.md
├── CLAUDE.md
└── AGENTS.md -> ./CLAUDE.md
```

初始化时的文件树及检查结果保留于 [Phase 1 验收记录](docs/requirements/phase-1-delivery.md)，当前目录以仓库实际文件为准。

## 本地开发环境

需要 Go 1.26、Node.js 24/npm、Task；PostgreSQL 用于开发与生产，Docker 用于 Compose 和 PostgreSQL 集成测试。以下命令从仓库根目录执行：

```bash
npm --prefix web ci
task init
# 编辑 server/configs/config.dev.yaml，填写 PostgreSQL 连接和账号
# 预先创建配置中指定的数据库
task doctor
task dev
```

`task init` 生成随机 JWT 和本机上传路径，保留已有配置，不创建数据库。`task dev` 显式执行 Goose migration 后启动 API（8900）和 Web（5173）。开发前端通过 `web/.env.development` 的 `VITE_API_BASE_URL=http://localhost:8900` 直连 API，无需新增代理；生产构建使用同源 API。不要把开发数据库凭据提交到仓库。

### 接入独立 Agent mock

系统通过 Agent 接收重量、稳定状态、车牌候选和抓拍，不再提供设备模拟设置，也不需要 `test_mode` 开关。API 与 mock 使用同一个本地连接密钥：

```bash
# 初始化或补齐本地共享密钥（已有配置保留）
task init
# 终端一：API 和系统 Web
task dev
# 终端二：独立 mock 控制台
task agent:mock
```

本地测试 API 默认监听 `0.0.0.0:8900`，mock 默认监听 `0.0.0.0:8090`，局域网可通过本机 IP 和对应端口访问。mock 本机页面为 http://127.0.0.1:8090，API 连接地址自动跟随本地 `http.address`；初始化自动生成本地密钥，补齐旧配置后需重启 API；也可在两端使用同一环境变量覆盖。详细操作见 [Agent mock](agent/README.md)，接口见 [协议 v1](protocol/v1.md)。真实设备驱动和物理打印尚未验收。

管理员维护资料并分配真实角色；操作员核对 Agent 候选、采集重量及抓拍后确认首磅／回磅。业务记录由 Server 保存，旧浏览器演示库不导入；历史测试单据及其依据保留。Agent 执行打印，结果未知时沿用人工核实和补打规则。

浏览器回归入口：`npm --prefix web run test:weighing:backend`，使用隔离的临时 PostgreSQL 容器（需要 Docker）、API／Web／mock 端口 18085／4185／18090，需已有 Playwright Chromium；结束后清理服务与数据。

首次 schema/seed migration 保留脚手架开发账号 `admin / admin123`；部署环境应修改账号密码。

## 开发与构建命令

直接运行 `task`（或 `task --list`）只展示日常常用入口；`task tasks`（或 `task --list-all`）查看所有任务。设备、监控、历史同步和专项验证命令仍可按原名称直接执行，用 `task <任务名> --summary` 查看用途及执行步骤。

本厂三服务打包使用 `task package VERSION=<唯一版本>`，它是 `task standalone:package` 的别名；安装与发布验证见[自治部署说明](docs/device/standalone-deployment.md)。列表精简不改变 `init`／`dev` 的初始化和启动行为。

| 命令（仓库根目录） | 用途 |
| --- | --- |
| `task api` / `task web` | 分别运行 API / Web |
| `task db:migrate` | 执行所有 schema 与 seed migration |
| `task backend:check` | Go 测试和 vet |
| `task backend:swagger` | 按现有入口生成 Swagger |
| `task check` | 初始化脚本测试、Go 测试/vet、前端 lint/build |
| `task build` | 构建前端并嵌入 Go，生成发布二进制 |
| `task build:check` | 发布构建与 embedweb 测试/vet |
| `task db:check` | PostgreSQL 集成门禁（需 Docker） |

Go 原生命令从 `server/` 运行：`go test ./...`、`go vet ./...`；npm 原生命令从 `web/` 运行：`npm ci`、`npm run build`、`npm run lint`。

PostgreSQL 集成测试每次运行共用一个临时容器，迁移命令只编译一次。普通业务测试通过 `startBusinessPostgres` 从本次进程迁移一次的模板库克隆独立数据库；模板使用 test 环境种子，完成迁移后禁止连接，随容器一并清理。迁移、环境种子、升级／回退专项通过 `startPostgres` 创建空库并执行真实迁移，备份恢复目标也使用空库。各测试结束删除其数据库；整套结束清理容器和临时可执行文件。Goose 回退测试仍使用全局状态，当前保持串行执行。

- 日常修改：在 `server/` 运行受影响测试，例如 `go test -tags=integration ./integration -run 'TestPostgresWeighing' -count=1 -timeout=20m`。
- 数据库改动及当前交付：运行 `task db:check`，包含本地业务、权限、设备契约、迁移及数据库恢复回归。CI 默认执行同一集成集合。
- 维护历史中心同步：额外运行 `task backend:integration:legacy-sync`。旧中心／厂区同步测试由 `integration && legacy_sync` 标签单独启用，也可在 GitHub Actions 手动勾选 `legacy_sync` 运行；不作为当前本厂自治的默认门禁。

`TestPostgresLocalBusinessRecovery` 覆盖数据库恢复后单据／称重依据不变、未决业务和打印不自动重放、旧备份不复用后续编号。它使用现有业务 HTTP 测试入口及厂区数据身份分配器，不是独立运行模式的初始化验收；独立初始化和正式 `standalone` HTTP／权限边界由现有 `standalone_*_test.go` 验证，不通过设置 `initialized` 或假造中心完成。图片文件和设备打印账本的完整备份恢复也不在此测试范围内。独立浏览器／监控脚本的环境组织不受本次拆分影响。

## 称重业务入口

登录后从“称重工作台”打开独立操作窗口，后台保留运输任务、原始称重、最终单据及基础资料。设备控制仅位于独立 Agent mock 页面。工作台实时展示 Agent 上报状态，稳定后由操作员采集并确认；异常处置、隔日回磅、人工调整、单磅重称及作废重开沿用现有业务规则。

历史界面模拟交付仍见 [Phase 2A 记录](docs/requirements/phase-2a-delivery.md)；其浏览器模拟库和演示设置不再用于当前业务。历史行为测试保留为测试夹具。当前范围见[本厂自治规格](docs/requirements/standalone-site-spec.md)，设备通信细则继续参考 [Issue #3 Spec](docs/requirements/agent-mock-issue-spec.md)。

## 部署方式

以下为已有构建和运行能力；完整本厂自治安装与维护整合仍待实现，不应直接沿用旧中心部署步骤或空角色开发配置上线。当前交付条件见[当前规格](docs/requirements/standalone-site-spec.md)。

### 单二进制

`task build` 生成 `bin/weighbridge-server`、`bin/weighbridge-migrate` 和独立的 `bin/weighbridge-host-collector`（宿主机监控工具，不是现场 Agent）。API 内嵌 Web，无需在运行服务器上安装 Node 或提供 dist。

部署时准备 `configs/config.yaml`、按模板填写的 `configs/config.prod.yaml`、`migrations/`、PostgreSQL 和可写上传目录。从包含 configs 与 migrations 的工作目录，设置 `APP_ENV=prod`，先运行迁移工具 `up --kind all`，成功后运行 Server。生产环境按部署网络配置反向代理、TLS 和持久卷，详见 [Server 运维说明](server/README.md)。

### Docker Compose（保留的开发部署流程）

```bash
cd server
cp .env.example .env
# 编辑 .env，填写 POSTGRES_PASSWORD 和随机 APP_JWT__SECRET
docker compose -f docker-compose.dev.yml config --quiet
docker compose -f docker-compose.dev.yml up --build
```

该流程使用仓库根目录为 build context，依次启动 PostgreSQL、migration 和内嵌 Web 的 API，默认访问 `http://127.0.0.1:8080`，Swagger `/swagger/index.html`。这是开发示例，生产需单独管理配置、密钥、备份和服务暴露。

## 当前状态与后续阶段

数据库单库收敛已完成，见 [PostgreSQL 交付](docs/requirements/postgresql-only-delivery.md)。接下来按[当前规格](docs/requirements/standalone-site-spec.md)完成本地初始化与管理、本地发布恢复、真实设备闭环及维护入口整合；中心同步暂缓。

历史初始化、界面模拟、业务后端、设备协议和各轮现场结果统一见[交付索引](docs/history/README.md)，不以历史阶段的“未实现”代替当前事实。

## 来源与历史资料

源自 [EziosWJ/base-project-golang](https://github.com/EziosWJ/base-project-golang) 的 main，基准提交 `3dc5cbb3f3210547df1cf7a647ab14d30f887bdc`。本项目使用独立 Git 历史，远程仓库为 [EziosWJ/weigh-bridge](https://github.com/EziosWJ/weigh-bridge)。本地目录和项目名仍为 `weighbridge`，Go Module 保留 `github.com/EziosWJ/weighbridge/server`。

历史 ADR、`record/` 和历史工单说明保留原文及源项目 Issue 链接，它们用于追溯脚手架背景，不能作为地磅功能已经实现的证据。来源与许可状态见 [SOURCE.md](SOURCE.md)。
