# weighbridge AI 开发指导

使用中文交流。项目为酒厂地磅管理系统；先阅读根目录 CONTEXT.md 了解领域约束，再按需读取涉及模块。AGENTS.md 是指向本文件的符号链接，更新规则时维护本文件。

## 目录与当前阶段

- server/：Go 1.26、Gin、GORM，沿用现有 internal 模块、cmd 入口、configs、migrations、integration。
- web/：React + TypeScript + Vite，npm 及 package-lock.json；前端开发前读取 web/AGENTS.md。
- agent/ 和 protocol/：当前仅预留 README，未实现现场代理与通信。
- docs/architecture/ 和 docs/requirements/：架构规划和阶段验收；docs/adr/ 与 record/ 包含脚手架历史，涉及既有决策时按需阅读。

Phase 1 仅工程初始化。业务规划必须明确标注“计划/未实现”，不得把称重、车牌识别、打印或 Agent 通信描述为已完成。

## 开发约定

1. 新业务模块遵守现有代码组织与命名；Go 沿用 handler/service/repository/model 分层及 app 装配，Web 复用 src/api、pages、components、store、types 与既有页面 Pattern。先理解同类模块，避免未经要求的大规模重构。
2. 所有 API 复用现有鉴权、RBAC、错误处理、响应封装和日志审计规范；保持已有路由和响应契约。不擅自改变 JWT、权限或用户管理逻辑。
3. 数据库结构变化必须新增 Goose migration，遵循既有 schema/seed 和 PostgreSQL/SQLite 目录方式，保留历史 migration；API 启动不执行 AutoMigrate。
4. 新增代码提供必要的行为测试；不通过删除测试、禁用 CI 或跳过配置检查规避失败。依赖和技术栈保持当前版本，升级需明确需求。
5. 正式称重由操作员确认；断网不得擅自生成正式电子过磅单。后续实现以 CONTEXT.md 与当期需求为准。
6. 真实凭据仅写入忽略的本地配置；示例配置使用占位符。保持 AGENTS.md 链接有效。
7. Git 操作遵守用户授权；origin 指向 https://github.com/EziosWJ/weigh-bridge.git，提交和推送须在用户授权范围内执行。历史 Issue 引用保留源仓库身份。

## 开发和验证入口

在仓库根目录优先使用 Taskfile.yml：task init 生成 server/configs/config.dev.yaml（随机 JWT 与本机上传目录），task doctor 检查环境，task db:migrate 显式迁移，task api 与 task web 分别启动服务，task dev 先迁移再并行启动。SQLite 本地 profile 使用 task init -- --database sqlite 和 task dev:sqlite。

Go 改动运行 task backend:check；Web 改动运行 task frontend:lint 与 task frontend:build；工程迁移运行 task check、task build:check。数据库改动加跑 task db:check（PostgreSQL 集成需要 Docker）。Swagger 入口 task backend:swagger。必要时在 server/ 运行 go test ./...、go vet ./...，在 web/ 运行 npm ci、npm run build。

长驻服务验证应有超时和清理；/health 检查存活、/ready 检查数据库就绪。保留原 SQLite 本地开发/兼容测试能力；V1.0 Server 部署数据库仍是 PostgreSQL。
