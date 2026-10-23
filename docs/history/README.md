# 历史与交付索引

本文记录来源与交付事实，不定义当前待开发范围。当前依据见[基线索引](../development-baseline.md)；各交付记录中的测试环境和“待实现”是当时状态，后续交付只覆盖明确新增的验证。

| 阶段 | 事实与入口 |
| --- | --- |
| 上下文收敛 | [收敛前快照](context-before-standalone-baseline.md)保存历史术语、阶段说明及原有引用；根上下文已改为当前入口 |
| 工程与前端模拟 | [Phase 1](../requirements/phase-1-delivery.md)、[Phase 2A](../requirements/phase-2a-delivery.md)、[可用性改版](../requirements/phase-2a-usability-delivery.md) |
| 业务修订 | [人工重量调整](../requirements/manual-weight-adjustment-delivery.md)、[吨制调整](../requirements/weight-entry-tonne-plan.md)、[操作员处置](../requirements/operator-weighing-corrections-delivery.md)、[独立工作台](../requirements/standalone-workbench-delivery.md) |
| 服务端及设备协议 | [业务闭环](../requirements/backend-business-loop-delivery.md)、[Agent mock](../requirements/agent-mock-delivery.md) |
| 打印与维护 | [SK-820 现场联调](../device/aisino-sk820-agent-test.md)、[维护首期](../requirements/agent-maintenance-delivery.md)、[维护改版](../requirements/agent-maintenance-revision-delivery.md)、[连续纸软件交付](../requirements/continuous-paper-print-delivery.md)；A4 单张已确认，连续五联纸未验收 |
| 厂区与身份 | [多厂区隔离](../requirements/multi-site-delivery.md)、[配置交付](../requirements/site-agent-configuration-delivery.md)；后者含 fcc-com-01 打印 Agent 与维护服务升级 |
| 中心同步 | [Issue #4 交付](../requirements/local-first-deployment-delivery.md)记录当时双数据库及浏览器验证；中心暂缓，软件完成不代表生产切换 |
| D12 | [接入交付](../requirements/d12-agent-integration-delivery.md)记录零值／故障／恢复；非零带载、稳定灯及重启权限仍待现场 |
| 摄像头模拟源 | [部署博客](../device/ffmpeg-rtsp-camera-simulation-blog.md)、[开发备忘](../device/camera-simulation-dev-memo.md)；2026-10-23 完成双路模拟 RTSP 部署、跨主机截帧和故障恢复；后续配置本机 Agent 并修正首次采集入口，业务图片上传及完整闭环仍待验证，不代替真实摄像头验收 |
| D12 模拟及本地 Agent 联调 | [模拟器交付](../requirements/d12-simulator-delivery.md)、[启动说明](../../agent/README.md)；虚拟串口、可控报文、真实串口驱动及浏览器回归通过，保留独立 mock 供业务测试 |
| PostgreSQL 收敛 | [ADR 0024](../adr/0024-postgresql-only.md)、[交付记录](../requirements/postgresql-only-delivery.md)；历史 SQLite 测试事实保留，现行数据库范围已调整 |

早期需求、ADR 与 record/ 原文保留。旧文档内的历史授权不代表本轮自动执行部署、提交推送或远程发布；当前任务范围以用户指令及当前规格为准。
