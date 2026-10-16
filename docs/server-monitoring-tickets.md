# 服务器监控 ticket 拆分

状态：用户已确认并完成发布。正式 [Spec #64](https://github.com/EziosWJ/base-project-golang/issues/64) 和 7 个实现 tickets 已发布到本仓库 GitHub Issues，均使用 ready-for-agent。远端正文、标签与 9 条原生阻塞关系已核对。

ADR-0011 已接受。正式 Spec 内容见 [服务器监控 Spec](server-monitoring-spec.md)，需求依据见 [服务器监控需求与设计](server-monitoring.md)。无需前置重构。

主要测试边界为真实 HTTP 路由加可控采集输入和时间；沿用双数据库 migration/HTTP 集成测试与 Playwright 页面验证，Docker 与 NVIDIA 支持补真实部署及硬件验证。

| 序号 | ticket | GitHub | Blocked by | 完成后可演示的行为 |
| --- | --- | --- | --- | --- |
| 01 | [主机 CPU/内存监控与 ADMIN 入口](server-monitoring-tickets/01-host-overview.md) | [#65](https://github.com/EziosWJ/base-project-golang/issues/65) | 无 | 原生 Linux 部署中，ADMIN 打开真实菜单，看到持续采样的主机、CPU、内存与采集状态 |
| 02 | [30 分钟趋势与资源异常提示](server-monitoring-tickets/02-history-and-warnings.md) | [#66](https://github.com/EziosWJ/base-project-golang/issues/66) | #65 | 查看 CPU/内存趋势、过期与缺口，观察固定阈值触发和恢复 |
| 03 | [Docker 宿主机采集端到端接入](server-monitoring-tickets/03-docker-host-collector.md) | [#67](https://github.com/EziosWJ/base-project-golang/issues/67) | #65 | Docker 内 API 通过宿主机采集进程读取真实主机 CPU/内存，断开时明确提示 |
| 04 | [多文件系统容量与块设备 I/O](server-monitoring-tickets/04-disk-monitoring.md) | [#68](https://github.com/EziosWJ/base-project-golang/issues/68) | #66、#67 | 两种部署中查看磁盘容量、读写趋势、IOPS 和空间异常提示 |
| 05 | [多网卡流量与链路状态](server-monitoring-tickets/05-network-monitoring.md) | [#69](https://github.com/EziosWJ/base-project-golang/issues/69) | #66、#67 | 两种部署中逐卡查看速率、趋势、错误、丢包和接口筛选 |
| 06 | [CPU/内存进程 Top 10](server-monitoring-tickets/06-process-ranking.md) | [#70](https://github.com/EziosWJ/base-project-golang/issues/70) | #67 | 两种部署中定位主机上的主要占用进程，并说明权限导致的覆盖不足 |
| 07 | [NVIDIA 多卡监控与支持状态](server-monitoring-tickets/07-nvidia-monitoring.md) | [#71](https://github.com/EziosWJ/base-project-golang/issues/71) | #66、#67 | 两种部署中逐卡查看 GPU 指标与趋势，准确区分不支持和采集失败 |

01 完成后，02 与 03 可以并行；03 完成后，06 不必等待 02；02、03 均完成后，04、05、07 可以并行。仅列直接依赖，不重复传递依赖。

02 提供后续设备趋势与异常判断的公共行为；03 提供后续指标在 Docker 下取得宿主机数据的端到端路径。每个后续切片交付原生与 Docker 的同一指标契约，并自行携带 API、页面、采集、相关测试及说明。

## 发布记录

- 已先发布完整 Spec，再按依赖顺序发布 7 个实现 issues。
- 各 ticket 的 Parent 引用 Spec #64，Blocked by 使用真实 GitHub issue 引用。
- 已创建并回读核对全部 9 条 GitHub 原生 blocking 关系，同时保留 Blocked by 文本。
- 父 Spec issue 保持开放且发布后未修改；链接已回填本索引和各 ticket 文档。
