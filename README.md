# weighbridge · 酒厂地磅管理系统

面向酒厂原粮、辅料及成品车辆进出厂的地磅管理项目，建设目标是让现场采集、人工确认、云端记录与后续查询形成可追溯的称重流程。

**当前处于 Phase 1：工程初始化。** 本仓库保留原脚手架的用户管理、JWT 认证、RBAC、菜单、部门、数据字典、系统配置、文件管理、操作/登录日志、公告通知与服务器监控模块。称重、车牌识别、设备接入、打印和现场 Agent 通信均未实现；现有示例页面不代表地磅业务已交付。

## 业务背景与 V1.0 范围（计划）

酒厂车辆需要关联入厂、出厂与两次称重结果。V1.0 从单地磅起步，保留多地磅扩展能力；支持首次、二次称重，结合运输方向计算毛重、皮重和净重。第一期采用自动识别车牌、操作员人工确认称重，识别结果不能直接形成正式称重记录。

已确认的部署与设备约束：

- Server 部署在远程服务器或云端，使用 PostgreSQL。
- Agent 部署在酒厂地磅房 Linux 工控机，计划使用 SQLite 保存现场状态。
- 计划接入柯力 D12 仪表、普通 RTSP 摄像机与 HyperLPR3、Linux 针式打印机。
- Agent 与 Server 计划通过 WSS 通信；云端断网时不得擅自生成正式电子过磅单。

这些约束是后续开发范围，不是本阶段已实现能力。

## 总体架构

```text
地磅房 Linux 工控机                       远程服务器 / 云端
D12 / RTSP 摄像机 / 针式打印机
             ↕（计划设备接入）
           Agent ───── WSS（计划） ───── Server ─── PostgreSQL
           SQLite（计划）                  ↕ HTTP API
                                         Web（浏览器）
```

Server 负责认证、权限、数据权威与后续正式业务记录；Agent 负责未来的现场设备采集与执行；Web 负责管理和未来的操作员确认交互。职责与术语见 [CONTEXT.md](CONTEXT.md)。

## 技术栈

保留源项目版本：Go 1.26（Docker 构建使用 1.26.5）、Gin、GORM、Goose、Koanf、JWT、Swagger、Prometheus；React 19、TypeScript、Vite 6、Tailwind CSS、shadcn/ui、React Router、Zustand。npm 依赖以 lockfile 为准，本阶段未升级依赖。

Server 的 PostgreSQL/SQLite 双数据库兼容能力保留，用于复用脚手架和本地开发；地磅 V1.0 的 Server 部署选用 PostgreSQL，Agent SQLite 尚未实现。

## 项目目录

```text
weighbridge/
├── server/              # Go API、现有模块和测试
│   ├── cmd/             # api / migrate / backup / dev-setup / host-collector
│   ├── configs/         # 配置与示例
│   ├── internal/        # 按原脚手架组织的模块
│   ├── migrations/      # 原 Goose 历史，原样保留
│   ├── integration/     # 数据库/API 集成测试
│   ├── docs/            # Swagger 产物
│   ├── go.mod
│   └── Dockerfile
├── web/                 # React 管理后台、tests、design-system
├── agent/               # 现场设备代理预留，仅 README
├── protocol/            # 通信协议预留，仅 README
├── docs/
│   ├── architecture/    # 当前与规划架构说明
│   ├── requirements/    # 阶段范围、验收记录
│   ├── adr/             # 原脚手架历史决策
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

完整文件树及本次检查结果见 [Phase 1 验收记录](docs/requirements/phase-1-delivery.md)。

## 本地开发环境

需要 Go 1.26、Node.js 24/npm、Task、CGO C 编译器；PostgreSQL 用于默认开发配置，Docker 用于 Compose 和 PostgreSQL 集成测试。以下命令从仓库根目录执行：

```bash
npm --prefix web ci
task init
# 编辑 server/configs/config.dev.yaml，填写 PostgreSQL 连接和账号
# 预先创建配置中指定的数据库
task doctor
task dev
```

`task init` 生成随机 JWT 和本机上传路径，保留已有配置，不创建数据库。`task dev` 显式执行 Goose migration 后启动 API（8099）和 Web（5173）。开发前端通过 `web/.env.development` 的 `VITE_API_BASE_URL=http://localhost:8099` 直连 API，无需新增代理；生产构建使用同源 API。不要把开发数据库凭据提交到仓库。

无 PostgreSQL 的本地验证可使用保留的 SQLite profile：

```bash
task init -- --database sqlite
task doctor -- --database sqlite
task dev:sqlite
```

首次 schema/seed migration 保留脚手架开发账号 `admin / admin123`；部署环境应修改账号密码。

## 开发与构建命令

| 命令（仓库根目录） | 用途 |
| --- | --- |
| `task api` / `task web` | 分别运行 API / Web |
| `task db:migrate` | 执行所有 schema 与 seed migration |
| `task db:backup -- --source /path/app.db --destination /path/backup.db --verify` | 保留的 SQLite 在线备份工具 |
| `task backend:check` | Go 测试和 vet |
| `task backend:swagger` | 按现有入口生成 Swagger |
| `task check` | 初始化脚本测试、Go 测试/vet、前端 lint/build |
| `task build` | 构建前端并嵌入 Go，生成发布二进制 |
| `task build:check` | 发布构建与 embedweb 测试/vet |
| `task db:check` | 包含 PostgreSQL/SQLite 的集成门禁（需 Docker） |

Go 原生命令从 `server/` 运行：`go test ./...`、`go vet ./...`；npm 原生命令从 `web/` 运行：`npm ci`、`npm run build`、`npm run lint`。

## 部署方式

### 单二进制

`task build` 生成 `bin/weighbridge-server`、`bin/weighbridge-migrate`、`bin/weighbridge-backup` 和独立的 `bin/weighbridge-host-collector`（宿主机监控工具，不是现场 Agent）。API 内嵌 Web，无需在运行服务器上安装 Node 或提供 dist。

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

- Phase 1：工程迁移、命名、配置与文档初始化；实际验证范围及限制以验收记录为准。
- Phase 2（计划）：细化业务状态与协议，按现有模块规范开发称重业务和 Agent，配套 Goose migration 与测试。
- 后续设备联调（计划）：D12、RTSP/HyperLPR3、WSS、Linux 打印机及异常恢复验收。

## 来源与历史资料

源自 [EziosWJ/base-project-golang](https://github.com/EziosWJ/base-project-golang) 的 main，基准提交 `3dc5cbb3f3210547df1cf7a647ab14d30f887bdc`。本项目使用独立 Git 历史，远程仓库为 [EziosWJ/weigh-bridge](https://github.com/EziosWJ/weigh-bridge)。本地目录和项目名仍为 `weighbridge`，Go Module 保留 `github.com/EziosWJ/weighbridge/server`。

历史 ADR、`record/` 和历史工单说明保留原文及源项目 Issue 链接，它们用于追溯脚手架背景，不能作为地磅功能已经实现的证据。来源与许可状态见 [SOURCE.md](SOURCE.md)。
