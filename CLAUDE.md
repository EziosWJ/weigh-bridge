# weighbridge AI 开发指导

使用中文交流。项目为酒厂地磅管理系统；先阅读根目录 CONTEXT.md，再读[当前阶段规格](docs/requirements/standalone-site-spec.md)；按任务读[业务规则](docs/requirements/weighing-business-spec.md)或[设备细则](docs/requirements/local-device-maintenance-spec.md)，实施前核对[当前状态](docs/next.md)。AGENTS.md 是指向本文件的符号链接，更新规则时维护本文件。

## 目录与当前阶段

- server/：Go 1.26、Gin、GORM，沿用现有 internal 模块、cmd 入口、configs、migrations、integration。
- web/：React + TypeScript + Vite，npm 及 package-lock.json；前端开发前读取 web/AGENTS.md。
- agent/ 和 protocol/：现场设备运行与维护资料、部署资源及分版本通信契约（当前 v2，v1 为历史）；设备 Go 实现沿用 server/cmd 与 server/internal，真实能力按交付记录判断。
- docs/requirements/：当前规格与交付证据；docs/adr/：重要决策理由；docs/history/：旧阶段和交付原始证据，追溯时按需阅读。

每厂本地自治的最终职责以总规格正文为准；实现、软件验证和现场验收分别记录在当前状态。历史文档不作为隐含需求；文档权威位置和更新规则见[开发基线](docs/development-baseline.md)。

## 开发约定

1. 新业务模块遵守现有代码组织与命名；Go 沿用 handler/service/repository/model 分层及 app 装配，Web 复用 src/api、pages、components、store、types 与既有页面 Pattern。先理解同类模块，避免未经要求的大规模重构。
2. 所有 API 复用现有鉴权、RBAC、错误处理、响应封装和日志审计规范；保持已有路由和响应契约。不擅自改变 JWT、权限或用户管理逻辑。
3. 数据库结构变化必须新增 Goose migration，仅维护 PostgreSQL 的 `server/migrations/schema` 与 `seed`，保留历史 migration；API 启动不执行 AutoMigrate。
4. 新增代码提供必要的行为测试；不通过删除测试、禁用 CI 或跳过配置检查规避失败。依赖和技术栈保持当前版本，升级需明确需求。
5. 正式称重需设备重量、操作员确认和本地业务校验；对应抓拍默认强制，本厂管理员可按[业务规则](docs/requirements/weighing-business-spec.md#4-抓拍策略)关闭并保留无图状态。互联网断开不阻塞本地正常业务；本地服务或设备链路故障不得用缓存绕过确认。提交与打印结果未知时先核实，恢复不自动重放。
6. 真实凭据仅写入忽略的本地配置；示例配置使用占位符。保持 AGENTS.md 链接有效。
7. Git 操作遵守用户授权；origin 指向 https://github.com/EziosWJ/weigh-bridge.git，提交和推送须在用户授权范围内执行。历史 Issue 引用保留源仓库身份。

## AI 开发执行方式

收到开发启动指令后，按已确认范围自主完成实现、相关验证和文档更新，任务之间直接衔接；常规实现选择和可逆操作由 Agent 决定，不逐项重复确认。执行与测试遵循 [开发约定](docs/agents/development.md)：采用满足当前需求的简单方案，按实际风险处理防护，按改动范围选择必要测试。

## 开发和验证入口

在仓库根目录优先使用 Taskfile.yml：task init 生成 server/configs/config.dev.yaml（随机 JWT 与本机上传目录），task doctor 检查环境，task db:migrate 显式迁移，task api 与 task web 分别启动服务，task dev 先迁移再并行启动。

Go 改动运行 task backend:check；Web 改动运行 task frontend:lint 与 task frontend:build；工程迁移运行 task check、task build:check。数据库改动加跑 task db:check（PostgreSQL 集成需要 Docker）。按实际改动选择入口，等价检查通过后无需重复执行底层命令；纯文档改动只检查文档。Swagger 入口 task backend:swagger；依赖缺失或锁文件变化时按需运行 npm ci。

长驻服务验证应有超时和清理；/health 检查存活、/ready 检查数据库就绪。Server 开发、数据库集成测试和生产统一使用 PostgreSQL，见 [数据库决策](docs/adr/0024-postgresql-only.md)。数据库测试使用隔离的临时实例，结束清理，不连接真实业务库。

## Agent skills

### Issue tracker

工作项使用 GitHub Issues；读取或发布工作项前，参阅 docs/agents/issue-tracker.md。

### Triage labels

采用五类标准 triage 标签；执行分类前，参阅 docs/agents/triage-labels.md。

### Domain docs

采用 single-context 布局：根目录 CONTEXT.md 与 docs/adr/。探索代码、讨论领域或记录决策时，遵循 docs/agents/domain.md。
