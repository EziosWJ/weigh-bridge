# weighbridge · 酒厂地磅管理系统

面向酒厂车辆进出厂称重的可追溯管理，每厂独立运行本地业务服务（内嵌 Web）、设备 Agent 和 PostgreSQL。

**开发入口：[CONTEXT](CONTEXT.md) → [当前总规格](docs/requirements/standalone-site-spec.md)。** [当前状态](docs/next.md)区分已实现、软件验证及现场待验收；[文档索引](docs/README.md)提供按任务阅读的入口。

## 目标与职责

Server 负责本厂业务、权限、证据、网络摄像头及视频源管理；Agent 负责仪表、测量有效性和打印。支持同机或本厂局域网分机，互联网中断不影响正常本地链路；中心能力暂缓。

设备职责调整的最终要求见[设备规格](docs/requirements/local-device-maintenance-spec.md)。目标不等于当前二进制能力；安装和操作须核对说明中的适用版本。

## 项目目录

| 目录 | 内容 |
| --- | --- |
| `server/` | Go API、设备命令、internal 模块、PostgreSQL migration 及集成测试 |
| `web/` | React／TypeScript 管理后台、工作台和设计 Pattern |
| `agent/`、`protocol/` | 设备部署资源、运行说明及分版本通信契约 |
| `docs/requirements/` | 当前总规格、业务与设备细则、交付证据 |
| `docs/adr/` | 重要决策及有效范围索引 |
| `docs/device/` | 部署、参数、校准与现场资料 |
| `docs/history/` | 旧方案、阶段交付、访谈及脚手架历史 |

沿用 Go 1.26、Gin、GORM、Goose，以及 React、TypeScript、Vite、npm；精确依赖版本见 `server/go.mod` 和 `web/package-lock.json`。开发约定见 [CLAUDE.md](CLAUDE.md)，`AGENTS.md` 为其符号链接。

## 本地开发

准备 Go、Node.js 24/npm、Task 和 PostgreSQL；Docker 用于隔离数据库集成测试。根目录执行：

```bash
npm --prefix web ci
task init
# 编辑生成的 server/configs/config.dev.yaml，填写本地 PostgreSQL 连接
# 先创建对应开发数据库
task doctor
task dev
```

`task init` 生成随机 JWT 及本机上传目录配置，保留已有文件，不创建数据库；`task dev` 显式迁移后启动 API（8900）与 Web（5173）。配置缺失、端口及初始化细则见[开发初始化](docs/development-initialization.md)。真实凭据仅放忽略的本地配置。

Server 首次启动自动建立本厂内部身份，无需填写厂区及地磅名称；在“配置管理”确认过磅单标题。Agent／mock 的启动及连接配置见 [Agent 使用说明](agent/README.md)。真实 Agent 与同磅 mock 互斥，软件模拟不替代现场验收。

## 检查与发布

| 入口 | 用途 |
| --- | --- |
| `task api`／`task web` | 分别运行服务 |
| `task db:migrate` | 显式执行 schema 与 seed migration |
| `task backend:check` | Go 测试及 vet |
| `task frontend:lint`／`task frontend:build` | 前端检查与构建 |
| `task db:check` | 隔离临时 PostgreSQL 集成检查 |
| `task check`／`task build:check` | 工程检查／内嵌发布构建验证 |
| `task backend:swagger` | 更新 API 文档 |
| `task local:package VERSION=<唯一版本>` | 生成本厂发布包 |

按实际改动选择检查，纯文档只检查文档；全部任务可用 `task tasks` 查看。数据库测试不连接真实业务库，结束清理临时资源。API 启动不自动建表或迁移。

安装、显式迁移、启停、升级与恢复见[本厂部署说明](docs/device/local-deployment.md)；服务配置、Docker 和数据库测试方式见 [Server README](server/README.md)。生产 Web 内嵌，不需要 Node／Vite 常驻。备份包含业务库、图片、有效配置和打印账本；未知结果先核实，恢复不自动执行称重或打印。

## 来源

源自 [EziosWJ/base-project-golang](https://github.com/EziosWJ/base-project-golang) 的基准提交 `3dc5cbb3f3210547df1cf7a647ab14d30f887bdc`，当前仓库为 [EziosWJ/weigh-bridge](https://github.com/EziosWJ/weigh-bridge)，Go Module 保留 `github.com/EziosWJ/weighbridge/server`。来源及许可见 [SOURCE.md](SOURCE.md)；历史 Issue 保留源仓库身份，历史交付入口见[历史索引](docs/history/README.md)。
