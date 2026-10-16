# 本地开发初始化设计

状态：已实现并通过本地验证。Spec 已发布为 [Issue #72](https://github.com/EziosWJ/base-project-golang/issues/72)，标记 `ready-for-agent`。本文记录 grill-with-docs 讨论中用户确认的范围与行为。

## 目标与范围

提高脚手架复用性，减少新项目首次启动时重复配置与排查环境的成本。本轮投入目标为 1–3 天。

已确认的范围：

- 仅初始化本地开发环境；项目名称、Go 模块名称与品牌替换留待后续讨论。
- 提供 `task init` 生成开发配置、`task doctor` 检查开发环境。
- PostgreSQL 保持默认选择，同时允许选择 SQLite。
- 初始化后提示运行既有的 `task dev` 或 `task dev:sqlite`；初始化入口不自动执行迁移或启动服务。
- 沿用实际开发配置 `server/configs/config.dev.yaml`，不另建根目录 `.env.local`。

## 当前基础

- README 的首次启动步骤要求复制开发配置模板并填写数据库与 JWT。
- Taskfile 已有 PostgreSQL/SQLite 的迁移与开发启动入口。
- 实际开发配置被 Git 忽略，模板文件由版本控制管理。
- PostgreSQL 迁移要求目标数据库已经存在；SQLite 可以在打开数据库时创建本地文件。
- 配置行为沿用 ADR-0005，数据库支持边界沿用 ADR-0010。

## 已确认行为

1. PostgreSQL 不可用或目标数据库不存在时，给出明确的准备步骤；本轮不自动启动 Docker 或创建 PostgreSQL 数据库。
2. 开发配置已经存在时，保留原文件及 JWT，不做覆盖；环境检查可以独立重复执行。

## 验收标准

| 场景 | 预期结果 |
| --- | --- |
| 干净检出，选择 PostgreSQL | 生成开发配置，提示准备目标数据库并运行 `task dev`。 |
| 干净检出，选择 SQLite | 生成开发配置，提示运行 `task dev:sqlite`；无需准备外部数据库。 |
| 缺少开发工具或配置不合法 | `task doctor` 指出具体问题与修复步骤；输出不包含数据库密码或 JWT 密钥。 |
| PostgreSQL 不可用或数据库不存在 | 检查报告未就绪并提示准备步骤，不自动启动 Docker、创建数据库或执行 migration。 |
| 已有开发配置，重复初始化 | 原文件内容及 JWT 保持不变，仍可独立运行环境检查。 |
| 初始化与后续启动完成 | 通过既有入口完成迁移与启动，API `/health` 与 `/ready` 检查通过，前端可访问。 |

## 实现与验证顺序

1. 实现配置生成并接入 `task init`；验证两个数据库选择与已有配置保护。
2. 实现环境检查并接入 `task doctor`；验证依赖缺失、配置错误、PostgreSQL 未就绪及凭据隐藏。
3. 更新 README 首次启动步骤；通过已有启动入口分别验证 PostgreSQL 与 SQLite，结束验证后清理启动进程。

涉及后端代码时运行 `task backend:check`。本轮沿用既有 migration，不增加数据库结构变更。

## 实现与验证结果

- 已接入 `task init`、`task doctor` 与 `task dev:setup:test`；初始化通过 `--database sqlite` 显式选择 SQLite。
- 初始化使用已有开发模板生成随机 JWT，将上传目录解析为当前检出目录下的绝对路径；独占创建配置文件并限制其访问权限，保留已有文件。
- 生成方式为对模板做字段级文本替换，模板注释原样保留；模板缺少必需字段时报错而不是写出占位值。
- 写入前校验解析出的项目根目录确实包含本检出，避免工作目录不符时静默写入错误的 `storage_root`。
- 环境检查复用配置加载规则；PostgreSQL 检查限制为 5 秒，SQLite 使用只读连接，缺少文件时仅提示显式迁移。配置与数据库错误不输出原始凭据。
- 启动指引根据实际选择的 profile 给出；未使用 profile 的自定义 SQLite 路径继续通过 `task dev` 启动，避免被 SQLite profile 的默认路径替换。
- 已通过 `task backend:check`（测试与 go vet）及 5 项 Node 命令入口测试。
- 已通过 PostgreSQL 与 SQLite 集成契约任务。
- Linux 下使用临时检出目录、临时 PostgreSQL 容器与独立端口验证了两种数据库的完整 init → doctor → dev 流程，包含配置保护、migration、API `/health` / `/ready` 和前端 HTTP 访问；临时进程与容器已清理。
- 生成逻辑改为文本级替换后，在临时检出副本中重跑了 SQLite 全链路（init → doctor → db:migrate:sqlite → doctor → api:sqlite），迁移前报告待迁移且未创建数据库文件，迁移后 `doctor` 全项通过，`/health` 与 `/ready` 均返回 200；同时在真实检出上确认 `task init` 未改动已有 `config.dev.yaml`。**本次改动后未重跑 PostgreSQL 全链路**：本机无 Docker，`task doctor` 如实报告 PostgreSQL 未就绪。
- Node 子进程测试和模拟无响应数据库的本机监听测试需要允许子进程输出捕获与临时端口绑定；本次在具备这些权限的环境中执行通过。Windows/macOS 未进行真机启动验收。

本文属于功能设计，不向 CONTEXT.md 添加通用开发工具术语；当前选择沿用已有架构约定，无需新增 ADR。
