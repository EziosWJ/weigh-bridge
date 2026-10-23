# 文档索引

当前开发从[根上下文](../CONTEXT.md)进入[本厂自治规格](requirements/standalone-site-spec.md)，通过[文档效力索引](development-baseline.md)判断旧材料适用范围。

| 目的 | 入口 |
| --- | --- |
| 当前架构及数据库决策 | [ADR 0023：每厂自治](adr/0023-standalone-site-baseline.md)、[ADR 0024：PostgreSQL](adr/0024-postgresql-only.md) |
| 下一步与验收 | [开发顺序](next.md)、[当前规格](requirements/standalone-site-spec.md) |
| 代码、软件验证及现场事实 | [历史与交付索引](history/README.md) |
| 阶段需求 | [需求索引](requirements/README.md) |
| 架构和既有设计 | [架构索引](architecture/README.md) |
| 开发与运行 | [根 README](../README.md)、[开发指导](../CLAUDE.md)、[Server 说明](../server/README.md) |
| 设备与契约 | [Agent 说明](../agent/README.md)、[协议 v1](../protocol/v1.md)、[D12 接入](device/keli-d12-agent-integration-guide.md) |
| 模拟摄像头部署与接入 | [FFmpeg／RTSP 博客稿](device/ffmpeg-rtsp-camera-simulation-blog.md)、[开发维护备忘](device/camera-simulation-dev-memo.md) |
| D12 虚拟串口与可控报文 | [模拟器使用说明](device/d12-simulator.md)、[交付及软件验证](requirements/d12-simulator-delivery.md) |

原有初始化、通知与监控文档保留脚手架背景，其验收不代替地磅业务验收。adr/、server-monitoring-tickets/ 与 record/ 中历史 Issue 属于原仓库，保留来源身份；旧目录名不作为当前可执行路径。

设备侧实验、软件 mock、单张实物打印与完整生产闭环是不同的验证结果；具体已验证范围和限制以对应交付记录为准。
