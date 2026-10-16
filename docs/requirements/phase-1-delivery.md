# Phase 1 交付与验收记录

验收日期：2026-10-16。源 main 基准提交：`3dc5cbb3f3210547df1cf7a647ab14d30f887bdc`。

## 交付状态

**Phase 1 完成，已具备进入 Phase 2 的工程基础。** 工程初始化和文档改造完成；本记录仅声明实际验证的能力。现场 Agent、通信协议、称重 API/业务表、设备、识别、打印和新业务页面均未实现。

Phase 1 验收时，新仓库位于 `weighbridge/`，使用独立的 main 分支，未创建提交、未设置 remote、未推送、未创建远程仓库。后续用户指定发布仓库为 `EziosWJ/weigh-bridge`，项目目录和 Go Module 保持原命名。只清理了本次新克隆副本的旧 .git；未修改其他项目。源码全部保留；32 个 migration 文件逐字节与源提交一致。

## 名称与路径映射

| 来源 | 当前 |
| --- | --- |
| base-project-golang | weighbridge（中文名：酒厂地磅管理系统） |
| base-go-api/ | server/ |
| react-admin/ | web/ |
| github.com/EziosWJ/base-project-golang/base-go-api | github.com/EziosWJ/weighbridge/server |
| react-admin-template（npm） | weighbridge-web（含 lockfile 根元数据） |
| bin/base-go-api | bin/weighbridge-server |
| bin/base-go-migrate、base-go-backup、base-go-host-collector | bin/weighbridge-migrate、weighbridge-backup、weighbridge-host-collector |
| /var/lib/base-go-api、/run/base-go-api | /var/lib/weighbridge、/run/weighbridge |
| base_go_api（开发数据库/用户及 Compose 卷前缀） | weighbridge |
| .data/base-go-api.db | .data/weighbridge.db |
| 原计划 EziosWJ/weighbridge | 后续按用户要求发布到 EziosWJ/weigh-bridge |

## 主要修改

- Server：go.mod 与所有 Go import；dev-setup 根目录定位与配套测试、配置路径和示例、Dockerfile/build context、Compose、Swagger 描述。已有业务代码仅必要的机械性适配，不改变 JWT/RBAC/用户逻辑。
- Web：package name 与 lockfile 根元数据、浏览器标题、登录品牌、侧栏、设置和 mock 中的系统名称。保留页面布局、菜单、认证和权限组件。
- 工具：Taskfile 的工作目录/发布产物、CI 路径与缓存、VS Code cwd/program、初始化脚本与测试、prepare-webui 资源路径、监控验证工具、Docker ignore 和本地配置 ignore。
- 文档：重写根 README、CONTEXT、CLAUDE；保留 AGENTS.md → ./CLAUDE.md；更新前端指导标题和现有使用文档路径；新增 agent/protocol 预留 README、架构/需求索引与 SOURCE 来源说明。
- README 包含业务背景、V1.0 计划、架构、技术栈、实际目录、开发/构建/部署、阶段状态；CONTEXT 定义职责、入厂/出厂、毛皮净重、人工确认与断网约束。

完整源码文件树见 [file-tree.txt](file-tree.txt)，逐文件修改清单见 [phase-1-changed-files.md](phase-1-changed-files.md)。生成产物和依赖目录不列入源码树。

## 实际检查

环境：Go 1.26.5、Node 24.16.0、npm 11.13.0、Task 3.53.1、Docker Compose 5.6.0、Docker daemon 29.8.2。没有降低 Go 或依赖版本。

| 检查及工作目录 | 结果 |
| --- | --- |
| server/: go test ./... | 通过（授权环境支持原测试所需本地 socket） |
| server/: go vet ./... | 通过 |
| web/: npm ci（使用 /tmp 缓存，no-audit/no-fund） | 通过，安装 252 个锁定依赖包；未运行安全审计 |
| web/: npm run build（由 task frontend:build 调用） | 通过，TypeScript 与 Vite 构建成功 |
| 根目录: task check | 通过：原 5 个初始化脚本测试、Go test/vet、ESLint、前端构建 |
| 根目录: task build:check | 通过：四个工具构建、Web 内嵌、embedweb 测试/vet |
| 根目录: task backend:integration | 通过（112.518s），包含 PostgreSQL 与现有 API/通知等集成契约 |
| 根目录: task db:integration:sqlite | 通过（18.678s），包含文件数据库、migration、API 与备份契约 |
| server/: docker compose --env-file .env.example -f docker-compose.dev.yml config --quiet | 通过 |
| 全源文件保留、migration 字节对比 | 通过，无源文件丢失，32 个 migration 文件不变 |
| go.mod/go.sum、npm package/lockfile 与基准对比 | 通过，仅 module 和 npm 根名称变化，无依赖升级 |
| Swagger JSON 对比 | 通过，info 以外路由、schema、响应定义相同 |
| 模块/import 和可执行配置的旧目录扫描 | 无失效旧工程路径；兼容标识及历史引用见下文 |
| Git remote、main、AGENTS 符号链接 | Phase 1 验收时无 remote、main 无提交、链接有效；后续发布另按用户授权执行 |

最终配置复核后再次执行 task check 和 task build:check，均以退出码 0 完成。最初受沙箱网络/端口/Docker socket 限制的检查已在授权环境重跑；没有删测试、禁用 CI 或移除配置校验。

## 保留的旧标识

- JWT issuer=base-go-api、audience=react-admin，浏览器 react-admin-auth / react-admin-remembered-username，Prometheus base_go_api_* 指标：有意保留兼容契约，配套测试仍使用原值。
- docs/adr/、record/、历史工单正文与 Issue URL：保留来源身份。docs/README.md 给出历史路径映射；不把源项目同号 Issue 当作目标项目工作项。
- 第三方依赖名不替换。来源项目标识保留在 SOURCE.md 和来源说明中。

## 限制与风险

- 源提交未包含 LICENSE/COPYING；未捏造开源许可，公开发布前需由维护者明确许可条件。
- go-sqlite3 原版本 C 编译存在 const qualifier 提示；前端存在 Browserslist 数据过期与大 chunk 提示，均非阻断，遵守本阶段不升级依赖的约束。
- Compose 配置已验证；未执行整个多阶段 Docker 镜像构建或生产部署。PostgreSQL 测试使用原套件创建并清理的临时容器，不代表完整生产环境验收。
- 未新增迁移或修改 schema；并未接入真实 D12、摄像机、打印机，也未进行现场设备验收。

## 下一阶段

可进入 Phase 2 的业务设计与实现：先确认运输方向/称重状态、重称/作废/异常、重量单位精度、正式单据条件及断网处理，再定义 Agent/Server WSS 契约，按原模块方式开发业务及必要测试；数据库变更使用新增 Goose migration。
