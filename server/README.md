# weighbridge Server 本地开发与调试

> 本页为当前单一本厂软件的开发与运行参考；目标见[总规格](../docs/requirements/standalone-site-spec.md)，实际软件与现场结果见[当前状态](../docs/next.md)。Server 管理业务、摄像头与视频，Agent 管理仪表及打印；协议 v1 历史部分仅用于旧客户端追溯。

`server` 是基于 Go、Gin、GORM 的 REST API，开发、测试及生产统一使用 PostgreSQL。数据库结构与内置数据由独立的 Goose migration 管理；API 启动时不会执行 migration 或 GORM `AutoMigrate()`。

## 称重业务与 Agent

称重模块提供资料、操作会话、采集确认、任务、异常处置、单据、查询／CSV 和打印核实，沿用既有 Bearer、RBAC、本厂操作范围、响应及审计。Agent 以局域网设备通道接入唯一地磅，只需 Server 地址，不取得用户业务确认权限；未初始化和同磅冲突明确拒绝。

Server 直接取得相机新图并编排 Agent 测量窗口，采样保留连接及测量代次至确认；无固定确认倒计时，设备、业务绑定或操作会话变化使依据失效。`session_ttl` 用于操作权续租，不是采样确认时限；重量消息为吨制十进制字符串，数据库存千分之一吨整数。细则见[业务规则](../docs/requirements/weighing-business-spec.md)与[分版本通信契约](../protocol/v1.md)。

开发运行见 [Agent／mock](../agent/README.md)，网络相机见[接入说明](../docs/device/camera-live-preview.md)。图片目录与业务库关联保留并一起备份；打印账本位于 Agent。单据标题由“系统管理 → 配置管理”维护，旧 YAML 标题仅作为升级固化来源，新票面快照与五联打印按业务规格执行。

## 前置条件

- Go 1.26（与 `go.mod` 保持一致）
- Docker 与 Docker Compose（运行本地 PostgreSQL、Docker Compose 开发模式及集成测试）
- 使用 VS Code 调试时，安装官方 Go 扩展

## 内嵌前端的发布构建

在仓库根目录执行 `npm --prefix web ci` 和 `task build`，生成 `bin/weighbridge-server`、`bin/weighbridge-migrate`。API 使用 `embedweb` Go build tag，将本次 React 生产构建嵌入二进制。构建任务先生成 `web/dist`，复制到 `server/internal/webui/dist`，随后编译 API；构建产物不提交 Git。手动执行 `go build -tags=embedweb` 前也必须完成资源准备。

发布 API 同时提供 `/` 下的管理页面与既有接口。直接访问和刷新页面回退到 `index.html`；缺失静态资源、未知 `/api/**`、运维路径和关闭的 Swagger 路径仍返回 JSON 404。前端使用同源 API，页面和资源响应设置 `Cache-Control: no-cache`，让浏览器使用前重新验证。

运行时无需前端文件或 Node.js；配置、数据库、上传目录、独立 migration 流程保持既有要求。可在 `server` 目录执行 `../bin/weighbridge-server`，或将产物和 `configs/` 部署到同一个工作目录。生产环境设置 `APP_ENV=prod`。`task build:check` 验证发布构建；`task api` 等开发入口仍配合独立 Vite 服务使用。

Docker 构建上下文为仓库根目录：

```bash
docker build -f server/Dockerfile --target api -t weighbridge-server .
```

API 镜像通过 Node 构建阶段生成前端并嵌入 Go 二进制，最终镜像不包含 Node。`migrate` target 不执行前端构建。开发 Compose 已同步使用该上下文，并将 API 显式绑定到容器内 `:8080`。

## 配置说明

应用按以下顺序加载配置，后者覆盖前者：

```text
默认值
→ configs/config.yaml
→ configs/config.{APP_ENV}.yaml
→ APP_ 环境变量
```

`APP_ENV` 只能通过进程环境变量指定，支持 `dev`、`test`、`prod`，未设置时默认为 `dev`。因此本地开发读取 `configs/config.yaml` 和 `configs/config.dev.yaml`。

实际环境 YAML 一律不提交 Git：

- `configs/config.yaml`：基础配置，不含环境特定凭据；
- `configs/config.dev.example.yaml`、`configs/config.prod.example.yaml`：环境模板，值为占位符，随仓库提交；开发模板由根目录 `task init` 渲染为 `config.dev.yaml`；
- `configs/config.dev.yaml`、`configs/config.prod.yaml`：实际环境配置，可含真实凭据，已被 `.gitignore` 忽略。

首次本地开发前，在仓库根目录生成配置并填写本机值：

```zsh
task init   # 生成 server/configs/config.dev.yaml：随机 JWT 与本机上传目录
# 编辑 server/configs/config.dev.yaml，至少替换 database.username/password
```

`task init` 不覆盖已有配置，也不轮换已有 JWT。生成后可运行 `task doctor` 检查工具、配置与数据库是否就绪。

数据库与 JWT 可以配置在 YAML 中。本机直接运行或 IDE 调试前，确认 `configs/config.dev.yaml` 中的值正确：

```yaml
database:
  url: postgres://127.0.0.1:54329/weighbridge?sslmode=disable
  username: weighbridge
  password: your-local-password

jwt:
  secret: a-random-local-development-secret
```

`database.url` 只能包含数据库地址、数据库名和连接参数，不能包含用户名或密码；应用会使用 `username`、`password` 生成最终的 PostgreSQL 连接串。

`.env` 只由 Docker Compose 读取，Go 程序不会自动加载它。直接执行 `go run` 或通过 VS Code 调试时，应使用 YAML 配置或在启动配置中显式注入环境变量。

## 数据库支持与备份

Server 只接受 `database.driver: postgres`；`APP_CONFIG_PROFILE` 已退役，旧配置会明确报错。旧 SQLite 文件不会自动迁移或删除，需保留的数据应先封存并单独制定转换步骤。历史 SQL 位于 [归档](../docs/archive/sqlite-migrations/README.md)，不参与运行、发布或后续开发。

PostgreSQL 使用标准 `pg_dump -Fc` 备份和 `pg_restore --exit-on-error` 恢复到独立空库。凭据通过受保护配置或 PostgreSQL 密码文件提供；备份调度、保留与异机副本由部署方案负责。恢复后显式运行 migration，并验证 `/ready`、登录及核心业务读取，再切换应用。发布回退保留旧应用和发布前备份，不以生产 migration down 代替恢复。

## 方式一：Docker Compose 一键启动

此方式同时运行 PostgreSQL、一次性 migration 容器和 API 容器，适合不需要断点调试时使用。

```zsh
cd server
cp .env.example .env
# 编辑 .env：至少替换 POSTGRES_PASSWORD 和 APP_JWT__SECRET
docker compose -f docker-compose.dev.yml up --build
```

Compose 按以下顺序启动：`postgres`（健康检查通过）→ `migrate`（成功后退出）→ `api`。Compose 会把 `.env` 中的 PostgreSQL 与 JWT 值以 `APP_*` 环境变量注入容器内应用，因此该方式无需 `configs/config.dev.yaml` 也能完整运行。

启动后可访问：

- API：<http://127.0.0.1:8080>
- Swagger：<http://127.0.0.1:8080/swagger/index.html>
- 存活检查：`curl http://127.0.0.1:8080/health`
- 数据库就绪检查：`curl http://127.0.0.1:8080/ready`

首次 migration 会写入内置管理员账号 `admin / admin123`，仅供本地开发使用。

停止容器但保留本地数据：

```zsh
docker compose -f docker-compose.dev.yml down
```

确实需要重建本机开发数据库时，再删除命名卷：

```zsh
docker volume rm weighbridge_postgres_data
```

## 方式二：VS Code 断点调试

此方式只用 Docker 运行 PostgreSQL，API 与 migration 由本机 Go 调试器启动。请勿同时运行 Compose 的 `api` 服务，否则会占用本机 `8080` 端口。

### 1. 准备配置和数据库

首次调试前，先创建本机实际开发配置并设置本地数据库密码：

```zsh
task init   # 若尚未创建 server/configs/config.dev.yaml；已有文件不会被覆盖
# 编辑 server/configs/config.dev.yaml：替换 database.username/password
cd server
cp .env.example .env
```

将 `configs/config.dev.yaml` 的 `database.password` 改为与 `.env` 中 `POSTGRES_PASSWORD` 相同的值。然后只启动 PostgreSQL：

```zsh
docker compose -f docker-compose.dev.yml up -d --wait postgres
```

### 2. 在 VS Code 执行 migration

仓库根目录的 [`.vscode/launch.json`](../.vscode/launch.json) 已提供以下调试配置：

- `Go Migration (dev)`：等价于 `APP_ENV=dev go run ./cmd/migrate up --kind all`；
- `Go API (dev)`：等价于 `APP_ENV=dev go run ./cmd/api`。

在“运行和调试”面板先选择 `Go Migration (dev)` 并按 `F5`。该任务是一次性执行，成功后会自动结束；新增或修改 migration 后也应重新执行一次。

### 3. 启动并调试 API

选择 `Go API (dev)` 并按 `F5`，即可在 Go 代码中设置断点。启动成功后使用上述 `/health`、`/ready` 或 Swagger 地址验证服务。

不使用 VS Code 时，可在 `server` 目录执行相同命令：

```zsh
APP_ENV=dev go run ./cmd/migrate up --kind all
APP_ENV=dev go run ./cmd/api
```

## 测试

运行普通单元测试：

```zsh
cd server
GOCACHE=/tmp/server-build go test ./...
```

运行 PostgreSQL 集成测试。测试会启动随机名称和随机本机端口的临时 PostgreSQL 容器，不读取工作区开发数据库配置：

```zsh
cd server
GOCACHE=/tmp/server-build go test -tags=integration ./integration
```

集成测试需要 Docker daemon 可用，会执行 migration 并检查 Goose 版本表。

仓库根目录的 `task db:check` 执行后端检查和完整 PostgreSQL 集成测试；测试使用临时容器、随机本机端口并自动清理，Docker 不可用时检查失败。普通单元测试不需要 Docker。

## 数据库测试组织

PostgreSQL 集成测试每次运行共用一个临时容器，迁移命令编译一次。普通业务测试通过 `startBusinessPostgres` 从本次迁移后的模板克隆独立数据库；模板使用 test 种子，迁移完成后禁止连接。迁移、环境种子、升级／回退专项通过 `startPostgres` 创建空库并执行真实迁移；恢复目标也使用空库。各测试结束删库，整套结束清理容器和临时可执行文件。Goose 回退测试涉及全局状态，保持串行。

`TestPostgresLocalBusinessRecovery` 核对恢复后业务证据、未决操作不重放及不复用后续编号；图片和设备打印账本的完整恢复不在该测试范围。一次性本厂初始化见 `TestPostgresLocalSiteInitializationHTTP`。业务浏览器入口 `npm --prefix web run test:weighing:backend` 使用临时 PostgreSQL、端口 18085／4185／18090 及 Playwright Chromium，结束清理；已知失败仍以当前状态为准。


## 厂云基础资料同步

在未提交的环境配置文件（如 `configs/config.dev.yaml`）中设置：

```yaml
master_sync:
  enabled: true
  address: "http://云端地址:端口"
  credential: "厂区凭据"
```

支持 HTTP 和 HTTPS。地址不包含 `/api/v1`；凭据由总部发放，不写入受版本控制的文件。修改后重启 Server。先在本厂身份设置与云端相同的厂区及地磅编码，由管理员打开资料页面点“立即同步”；首次成功前不能正式称重。后续每小时自动同步，操作员可主动同步，失败保留上一版。关闭开关并重启后回到单机模式；再次启用后须管理员手动同步成功才恢复云端模式。管理员可在资料页查看和清理同步日志。

部署前执行 Goose schema 与 seed 迁移。恢复测试、升级及联调须使用隔离库；单据上传不属于此功能。
