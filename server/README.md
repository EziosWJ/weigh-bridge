# weighbridge Server 本地开发与调试

`server` 是基于 Go、Gin、GORM 的 REST API，PostgreSQL 为默认数据库，保留 SQLite 单实例兼容能力；地磅 V1.0 Server 部署使用 PostgreSQL。数据库结构与内置数据由独立的 Goose migration 管理；API 启动时不会执行 migration 或 GORM `AutoMigrate()`。

## 前置条件

- Go 1.26（与 `go.mod` 保持一致）
- Docker 与 Docker Compose（运行本地 PostgreSQL、Docker Compose 开发模式及集成测试）
- 使用 VS Code 调试时，安装官方 Go 扩展

## 内嵌前端的发布构建

在仓库根目录执行 `npm --prefix web ci` 和 `task build`，生成 `bin/weighbridge-server`、`bin/weighbridge-migrate`、`bin/weighbridge-backup`。API 使用 `embedweb` Go build tag，将本次 React 生产构建嵌入二进制。构建任务先生成 `web/dist`，复制到 `server/internal/webui/dist`，随后编译 API；构建产物不提交 Git。手动执行 `go build -tags=embedweb` 前也必须完成资源准备。

发布 API 同时提供 `/` 下的管理页面与既有接口。直接访问和刷新页面回退到 `index.html`；缺失静态资源、未知 `/api/**`、运维路径和关闭的 Swagger 路径仍返回 JSON 404。前端使用同源 API，页面和资源响应设置 `Cache-Control: no-cache`，让浏览器使用前重新验证。

运行时无需前端文件或 Node.js；配置、数据库、上传目录、独立 migration 流程保持既有要求。可在 `server` 目录执行 `../bin/weighbridge-server`，或将产物和 `configs/` 部署到同一个工作目录。生产环境设置 `APP_ENV=prod`。`task build:check` 验证发布构建；`task api` 等开发入口仍配合独立 Vite 服务使用。

Docker 构建上下文为仓库根目录：

```bash
docker build -f server/Dockerfile --target api -t weighbridge-server .
```

API 镜像通过 Node 构建阶段生成前端并嵌入 Go 二进制，最终镜像不包含 Node。`migrate` 和 `backup` target 不执行前端构建。开发 Compose 已同步使用该上下文，并将 API 显式绑定到容器内 `:8080`。

## 配置说明

应用按以下顺序加载配置，后者覆盖前者：

```text
默认值
→ configs/config.yaml
→ configs/config.{APP_ENV}.yaml
→ configs/config.{APP_CONFIG_PROFILE}.yaml（如果设置）
→ APP_ 环境变量
```

`APP_ENV` 只能通过进程环境变量指定，支持 `dev`、`test`、`prod`，未设置时默认为 `dev`。因此本地开发读取 `configs/config.yaml` 和 `configs/config.dev.yaml`。

实际环境 YAML 一律不提交 Git：

- `configs/config.yaml`：基础配置，不含环境特定凭据；
- `configs/config.dev.example.yaml`、`configs/config.prod.example.yaml`：环境模板，值为占位符，随仓库提交；开发模板由根目录 `task init` 渲染为 `config.dev.yaml`；
- `configs/config.sqlite.yaml`：SQLite profile 配置，只覆盖数据库类型和本地文件路径，随仓库提交；
- `configs/config.dev.yaml`、`configs/config.prod.yaml`：实际环境配置，可含真实凭据，已被 `.gitignore` 忽略。

首次本地开发前，在仓库根目录生成配置并填写本机值：

```zsh
task init   # 生成 server/configs/config.dev.yaml：随机 JWT 与本机上传目录
# 编辑 server/configs/config.dev.yaml，至少替换 database.username/password
```

`task init` 不覆盖已有配置，也不轮换已有 JWT；需要 SQLite 时加 `-- --database sqlite`。生成后可运行 `task doctor` 检查工具、配置与数据库是否就绪。

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

## SQLite 正式部署

SQLite 必须显式配置 `database.driver: sqlite`，`database.url` 是本地持久文件路径；不配置 `username` 和 `password`：

```yaml
database:
  driver: sqlite
  url: .data/weighbridge.db
  # username/password 必须省略
```

开发环境无需修改 PostgreSQL 的 `config.dev.yaml`，直接使用 SQLite profile 任务：

```zsh
task db:migrate:sqlite
task api:sqlite
# 或：task dev:sqlite
```

任务会自动加载 `configs/config.sqlite.yaml`；如需更换数据库文件位置，直接修改该文件的 `database.url`。首次使用时请确保 `server/.data` 目录存在，并准备好 JWT 配置。

SQLite 生产支持的边界是单个 API 实例、一个本地持久卷和小规模低写并发。应用会为每个连接启用外键、WAL、5000ms 忙等待、`synchronous=NORMAL` 和 UTC 扫描；忙等待耗尽后返回统一的 HTTP 503（`code: 503`），不会把 `database is locked` 暴露给客户端，也不会重放业务事务。不要让多个 API 副本、网络文件系统或其他进程共享同一个数据库文件。持续出现锁超时、需要多副本/更高写吞吐，或恢复窗口不满足目标时，应使用默认的 PostgreSQL。

SQLite 使用独立的 `migrations/sqlite/schema` 和 `migrations/sqlite/seed` Goose 树；两套树的逻辑版本号必须锁步。执行 migration 前确保数据库目录已经存在且由 API 用户可读写：

```zsh
cd server
APP_ENV=prod go run ./cmd/migrate up --kind all
APP_ENV=prod go run ./cmd/api
```

备份使用内置 SQLite online backup API，不依赖系统 `sqlite3` 命令。源库可以保持运行，但备份调度、保留周期、父目录权限和跨介质复制仍由部署方负责：

```zsh
cd ..
task db:backup -- --source /var/lib/weighbridge/data/weighbridge.db \
  --destination /var/lib/weighbridge/backups/weighbridge-$(date +%Y%m%d%H%M%S).db \
  --verify
```

命令会执行 `PRAGMA integrity_check` 并将备份文件权限收紧为 `0600`。恢复时先停止 API，把已验证备份复制为新的数据库文件，再用当前发布执行 migration 并完成 `/ready`、管理员登录和核心读写冒烟验证；不要覆盖正在使用的源文件：

```zsh
cp /var/lib/weighbridge/backups/weighbridge-verified.db /var/lib/weighbridge/data/weighbridge-restored.db
APP_ENV=prod APP_DATABASE__URL=/var/lib/weighbridge/data/weighbridge-restored.db go run ./cmd/migrate up --kind all
APP_ENV=prod APP_DATABASE__URL=/var/lib/weighbridge/data/weighbridge-restored.db go run ./cmd/api
```

发布回退采用“恢复发布前备份 + 启动旧应用版本”，不承诺自动执行 migration `down`。生产数据文件、WAL/SHM 文件、备份文件和父目录都应只允许应用/备份用户访问；备份失败、完整性检查失败或恢复后的版本/业务校验失败时，不得切换流量。当前不支持 MySQL、SQLCipher 或 PostgreSQL/SQLite 之间的存量数据转换。

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

运行 SQLite 文件库集成测试（不需要 Docker）：

```zsh
cd server
GOCACHE=/tmp/server-build go test -tags=integration ./integration -run SQLite
```

仓库根目录的 `task db:check` 会执行后端检查以及 PostgreSQL、SQLite 两套集成契约；数据库相关变更必须同时通过两套数据库验证。
