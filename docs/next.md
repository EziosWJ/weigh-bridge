# 当前实现、验证与剩余工作

更新：2026-10-25 DC-09 联合回归。本文区分代码、软件验证、隔离模拟、部署演练和现场验收；[总规格](requirements/standalone-site-spec.md)表达最终目标，本文表达已知完成程度。详细代码范围与命令结果见[设备职责改造交付记录](requirements/local-device-maintenance-delivery.md)。

| 范围 | 代码／软件证据 | 部署／现场边界 |
| --- | --- | --- |
| PostgreSQL 单库 | [收敛交付](history/requirements/postgresql-only-delivery.md)记录实现、迁移及相关软件验证 | 当前数据库范围已经确定，不再维护 SQLite |
| 单一本厂运行形态（ADR 0028） | [交付记录](requirements/local-site-only-delivery.md)记录身份、业务、发布安装和隔离验证；开发库切换已执行 | 当时的开发环境操作是历史事实；生产切换及完整现场验收未完成 |
| 初始化简化（ADR 0029） | [补充交付](requirements/local-site-only-delivery.md#adr-0029-补充交付2026-10-24)记录相关后端、PostgreSQL、前端及安装验证 | 真实分机和设备验收待完成 |
| 业务、Agent、打印及可选抓拍旧版 | [交付索引](history/README.md)可追溯业务、协议、维护与抓拍软件验证 | 旧版 Agent 相机／配对／草稿验证不能证明新职责通过 |
| 新设备与配置职责（ADR 0030，DC-01～DC-09） | [交付记录](requirements/local-device-maintenance-delivery.md)记录代码范围、跨包修复、AC-01～AC-10 软件结论及实际检查结果；本轮 `task check`、`task build:check`、`task backend:check`、`task frontend:lint`、`task frontend:build`、`task db:check`、安装与发布冒烟、相关浏览器回归均通过 | 真实主机安装、真实旧库迁移、真实仪表／相机／MediaMTX、连续纸与断网运行未验收；软件通过不替代现场结论 |
| D12 | [Agent 接入交付](history/requirements/d12-agent-integration-delivery.md)记录软件与零值／故障／恢复；[原始串口联调](device/keli-d12-rs232-blog.md)另有带载报文及吨制核验 | 原始带载抓包不等于完整 Agent 业务闭环；非零称重、稳定灯对照、重启权限仍需现场验收 |
| 打印 | A4 单张版面有现场确认，[连续纸交付](history/requirements/continuous-paper-print-delivery.md)有软件验证 | 首张定位、连续累计偏移、撕纸停靠、第五联清晰度仍须实物验收 |
| 摄像头与识别 | [模拟源联调](device/camera-simulation-dev-memo.md)记录 RTSP 等软件／实验结果；本轮补充 Server 直连抓拍、动态摄像头和 MediaMTX 同步的软件回归 | 真实相机对应图片、识别、昼夜视角及长时间运行待验收；本轮未运行需要 `MEDIAMTX_BIN` 的真实 MediaMTX 集成 |

## 2026-10-25 联合回归结论

- `weighing-backend-smoke.mjs` 原“31 秒旧采样”失败已定位：失效来自设备数据超过新鲜度阈值（本机进程短暂停顿时同样会发生），不是确认倒计时。测试现区分“设备 revision 变化导致的失效”和“无依据失效”，并在设备依据变化时重新采集；无固定确认时限由 `expiresAt=null`、无客户端计时器及该断言共同验证。
- 后端业务冒烟改为 Server 直连两个动态命名摄像头，覆盖 0 台提示、动态集合、核对图片和放大预览；不再以固定 front／rear 作为验收依据。
- 修复 Server 直连抓拍沿用 Agent 车牌候选时误标 `plateEvidence.source=server` 的问题；现按实际识别／观察来源保存，协议文档已同步。
- 维护页浏览器回归改为单次保存并应用、Agent CUPS 队列、成功备份恢复和未知打印作业确认，不再验证已删除的草稿与 Agent 相机配置。

## 仍未验证或未通过

- 真实 D12 非零称重、稳定灯对照、串口权限变更后重启、真实 Agent 长时间运行。
- 真实 RTSP 相机、昼夜画面、外部识别接口、真实 MediaMTX 异机部署／重启／离线删除。
- 连续五联纸首张定位、累计走纸偏差、撕纸停靠、第五联清晰度。
- 在真实旧业务库上执行升级迁移与回退演练；本轮集成测试使用隔离临时 PostgreSQL。
- 真实主机 systemd 单元、旧 drop-in 退役和分机网络部署；安装冒烟只使用临时目录与替身。
- 断互联网长时间运行、Server 电脑灾难恢复与单盘保护；Agent 参数备份不是整套业务系统备份。
- 客户特殊运输、结算日期与专用报表口径仍未确认，不自动加入当前实现。

## 下一步

1. 按[本厂部署说明](device/local-deployment.md)在隔离或演练主机完成同机与分机安装、升级和换机恢复演练，记录实际 unit、队列和串口权限结果。
2. 准备真实仪表、相机与 MediaMTX 环境，按 AC-01～AC-10 分别补现场记录；软件与模拟通过不替代现场结论。
3. 连续纸校准按“先节长、再位置、必要时缩放”实测；不把程序参数当作已修复走纸偏差。
4. 现场验收前保持 ADR 0030 的职责边界：摄像头、识别与视频配置只在 Server，串口与打印只在 Agent，Server 只保存成功配置副本用于明确恢复。

## 状态维护

代码完成、软件验证、隔离模拟、部署演练、现场验收分别写范围、日期和证据；失败、未运行、未验收如实保留。新增交付记录链接到本页，README／CONTEXT／ADR 只引用本页，不重复维护进度。旧交付正文保留当时事实，不覆盖最新状态。
