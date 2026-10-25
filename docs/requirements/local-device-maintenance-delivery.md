# 单厂设备职责改造交付记录（DC-01～DC-09）

2026-10-25。对应[设备规格](local-device-maintenance-spec.md)、[实施计划](../history/requirements/local-device-maintenance-plan.md)与 [ADR 0030](../adr/0030-local-device-responsibilities-and-configuration.md)。本文记录代码范围、软件验证与仍需部署／现场确认的边界；完成度汇总见[当前状态](../next.md)。

## 1. 代码范围

| 工作包 | 实际落点 |
| --- | --- |
| DC-01 数据与协议契约 | `server/internal/agentproto` 的 v2 消息、能力与错误码；`server/migrations/schema/00017_local_site_only.sql`、`00018_local_device_contract.sql`、`00019_ticket_title_snapshot.sql`；`protocol/v1.md` 区分 v1 历史事实与 v2 契约 |
| DC-02 Agent 接入与测量 | `server/internal/agentbridge`（无密钥准入、首连接互斥、连接代次、命令关联）、`server/internal/agentscale`（逐帧锁存、测量代次、开始／结束／核实）、`cmd/agent-printer`、`cmd/agent-mock` |
| DC-03 Server 摄像头与视频 | 新增 `server/internal/camera`（CRUD、密码语义、并发抓拍、测试、识别、MediaMTX 托管路径与版本同步）；沿用 handler/service/repository/model 分层并接入 app 装配 |
| DC-04 本厂配置与入口 | `server/internal/sysconfig` 的标题与抓拍策略、`server/internal/weighing/local_setup.go`、`site_config_*`；`web/src/pages/system/configs/local-site-configuration.tsx` |
| DC-05 Agent 应用与备份 | `server/internal/agentmaintenance` 的单次应用、CUPS 枚举、设备配置持久化与回退、成功参数上报；`server/migrations` 中的备份表 |
| DC-06 称重编排与历史证据 | `server/internal/weighing/workflow_service.go`、`workflow_repository.go`、`device.go`；Server 编排测量窗口、冻结摄像头集合与版本、保存策略／图片／标题快照 |
| DC-07 摄像头页面与工作台 | `web/src/pages/cameras`、`web/src/pages/weighing/camera-panel.tsx`、`camera-live.tsx`、核对与历史图片列表；动态名称与逐台结果 |
| DC-08 安装升级与恢复 | `agent/deploy/install-local.sh`、`scripts/package-local.mjs`、`scripts/check-local-install.mjs`、`scripts/check-local-release.mjs`、`cmd/agent-provision`、`cmd/local-admin-init` |
| DC-09 联合回归与交付 | 跨包契约核对、浏览器回归修复、`protocol/v1.md`、部署说明、维护页 Pattern 与本记录 |

联合回归中修复的跨包问题：

1. 旧维护页浏览器回归仍按“草稿＋Agent 相机＋连接密钥”编写，与实际单次保存并应用页面不一致；已更新为串口／波特率、Agent CUPS 队列、节长小数、成功备份恢复、未知打印作业确认与只读权限。
2. 后端业务冒烟仍按“固定车头／车尾”取图；已改为 Server 直连两台动态命名摄像头，覆盖 0 台提示、动态集合、核对图片与放大预览。
3. 业务冒烟用 31 秒等待验证“无确认倒计时”，但把本机进程暂停造成的设备数据过期误判为倒计时失效；现改为区分“设备依据变化”和“固定时限”，只有 revision 未变而采样失效才失败。
4. Server 直连抓拍沿用了 Agent 观察的车牌候选，却把 `plateEvidence.source` 标为 `server`；现按实际来源保存 `agent`／`server`，并在协议文档说明“图片由 Server 保存”不等于“车牌由 Server 识别”。
5. 打印冒烟不再依赖 mock 控制页轮询标题，改为核对“打印尝试先登记、Agent 收到同一 attemptId”，避免浏览器轮询与接口时序造成的假失败。

## 2. 软件验证证据

2026-10-25 在工作区实际执行：

| 入口 | 结果 |
| --- | --- |
| `task dev:setup:test` | 5 项通过 |
| `task backend:check` | `go test ./...` 与 `go vet ./...` 通过 |
| `task frontend:lint` / `task frontend:build` | 通过 |
| `task build:check` | 内嵌 Web 发布构建、`go test -tags=embedweb ./...`、`go vet -tags=embedweb ./...` 通过 |
| `task db:check` | 隔离 PostgreSQL 集成测试通过（约 53 秒） |
| `task local:package` | 生成 Linux amd64 发布包与 SHA-256 manifest |
| `task local:install:smoke PACKAGE_DIR=...` | 通过：同机／Server-only／Agent-only、Agent-only 无数据库、旧 `agent.env` 清理与相机地址备份、unit/drop-in 核对与受控迁移、私有配置与账本保留、损坏包拒绝 |
| `task local:release:smoke PACKAGE_DIR=...` | 通过：隔离 PostgreSQL 迁移、一次性管理员初始化、内嵌页面、health/ready、本厂初始化、Agent 连接配置、重启保留身份／证据／未知账本 |
| `npm run test:camera-management` | 通过：0/1/3 台、逐台测试与结果时间、预览与测试分开、密码留空保持／显式清除、识别源删除停用、版本与同步失败、只读及窄屏 |
| `npm run test:local-management` | 通过：本厂配置统一入口与权限、单次保存并应用、成功备份恢复、Agent 队列、未知打印作业、页签键盘、记录日志与窄屏 |
| `node tests/site-configuration-smoke.mjs` | 通过：首次配置、重复初始化失败、身份持久化、仅本厂配置、在线阻止停用 |
| `npm run test:menu-order` / `test:business-layout` | 通过 |
| `npm run test:weighing:agent-capture` | 通过：无预览采集、动态摄像头多图、失败不生成采样、无图策略与人工确认 |
| `npm run test:weighing:camera-live` | 通过：WHEP 协商、动态名称、冻结／失败重连、同步失败表达、窄屏 |
| `npm run test:weighing:backend` | 通过：Server 直连抓拍、两方向业务闭环、作废更正重开、打印登记与未知核实、31 秒无确认倒计时（按设备依据判断）、识别候选、账号接管与踢出 |

未执行：真实 MediaMTX 集成（`task camera:integration` 需要 `MEDIAMTX_BIN`）、通知与监控浏览器测试（本次未改动对应模块）、真实仪表与相机联调。

## 3. AC 对照（软件层面）

| AC | 结论 | 主要证据 |
| --- | --- | --- |
| AC-01 接入 | 仅填 Server 地址可接入；未初始化、缺地磅、连接冲突有明确错误；首连接不被抢占；无 key／配对门槛 | `agentbridge/local_admission_test.go`、`agentmock/mock_test.go`、`cmd/agent-printer/measurement_test.go`、`local:release:smoke` |
| AC-02 仪表 | 页面只有串口与波特率；固定 D12 `tF=05`、吨制与 1 秒新鲜度；未配置／打开失败仍可维护并显示实际账号提示 | `agentmaintenance/device_config_test.go`、`agentscale/monitor_test.go`、`test:local-management` |
| AC-03 应用与打印 | 单次提交区分 applied／rejected／unknown／rolled_back／rollback_failed；队列来自 Agent；节长支持小数；打印去重与未知核实不退化 | `agentmaintenance/device_config_test.go`、`test:local-management`、`test:weighing:backend` |
| AC-04 可变相机 | 0、1、3 台及任意名称可维护、预览、测试、抓拍、核对和查看历史；无前后两图限制 | `test:camera-management`、`integration/camera_module_test.go`、`test:weighing:backend`（两台 HTTP 源） |
| AC-05 策略 | 强制开启时集合非空且全部成功；部分失败拒绝；关闭时保留逐台状态；单次统一截止默认 6 秒 | `camera/service_test.go`、`integration/measurement_capture_test.go`、`test:camera-management` |
| AC-06 设备依据 | 抓拍期间及结束后、确认前的变重、失稳、故障、过期、断连、重载、迟到回复均拒绝；同批帧变化识别；无确认倒计时 | `agentscale/measurement_test.go`、`integration/measurement_capture_test.go`、`cmd/agent-printer/measurement_test.go`、`test:weighing:backend` |
| AC-07 视频与识别 | MediaMTX 异机／离线／重启／同步失败／漂移／迟到表达；其他流不受影响；Agent 离线仍可维护预览；直连抓拍不依赖预览；识别复用图片且失败不阻断 | `camera/service_test.go`、`integration/camera_module_test.go`、`test:camera-management`、`test:weighing:camera-live` |
| AC-08 权限与入口 | 本厂配置统一入口、原分组权限有效、标题由配置管理生效；Agent 页不再显示相机／身份秘密／草稿 | `test:local-management`、`site-configuration-smoke`、`sysconfig` 测试、`integration/ticket_title_integration_test.go` |
| AC-09 安装恢复 | 同机与 Server-only／Agent-only 安装、旧配置升级、参数备份滞后、换机仅恢复设备参数且不覆盖连接与运行状态 | `local:install:smoke`、`local:release:smoke`、`agentmaintenance/device_config_test.go` |
| AC-10 历史 | 旧图片、单据及打印尝试可查询；新图片保存实际相机名与来源；新单标题快照与旧单基准保留 | `integration/measurement_capture_test.go` 的 `TestPostgresLegacyAndServerImagesRemainReadable`、`ticket_title_integration_test.go`、`local_recovery_integration_test.go`、`test:weighing:backend` |

## 4. 边界与剩余验收

- 以上为软件与隔离模拟结论。安装冒烟使用临时目录、替身 unit 和临时 PostgreSQL；没有在真实业务库、真实主机或 systemd 上部署。
- 未验证：真实 D12 非零称重与稳定灯、串口权限变更后的服务重启、真实 RTSP 相机与昼夜画面、外部识别接口、真实 MediaMTX 异机部署与重启、连续五联纸首张定位／累计偏移／撕纸停靠、断互联网长时间运行。
- 未验证：在真实旧库上执行迁移与回滚演练；集成测试只证明新增迁移和查询在临时 PostgreSQL 上成立。
- 打印结果未知仍需人工核实；任何恢复、换机或重连都不自动补称、确认、出单或重发旧打印。
