# ADR-0011：宿主机资源采集边界

## Status

Accepted

实现规格已发布为 [Spec #64](https://github.com/EziosWJ/base-project-golang/issues/64)，7 个实现工作项与阻塞依赖见 [ticket 索引](../server-monitoring-tickets.md)。

## Decision

服务器监控的对象是本项目 API 所在宿主机。API 直接部署时在进程内采集；API 通过 Docker 部署时，由同一宿主机上的独立采集进程通过本地 Unix socket 提供主机资源指标。第一版正式支持 Linux，不扩展为远端主机管理或分布式监控系统。

2026-10-14 增量决策（[Spec #77](https://github.com/EziosWJ/base-project-golang/issues/77)）：平台目标扩展至原生运行 API 的 Windows Server 2019/2022，保持单台宿主机和既有 Linux Docker 采集边界。考虑到通用脚手架的运行与维护负担，资源监控改为默认关闭的可选能力：关闭时不初始化采集器、不启动采样、不维护历史，按当前 API 实例配置过滤导航，接口保留认证及 ADMIN 授权后明确返回模块未启用。Windows Docker 宿主机采集和新通信协议不在此次范围内；Windows 编译和真机验证分别记录。

## Context

现有 API 容器使用独立网络与挂载命名空间，以非 root 用户运行，未配置宿主机文件系统或 GPU 采集条件。仅挂载宿主机 `/proc` 与 `/sys`，不能保证宿主机网卡、挂载点容量和 GPU 指标均可正确采集；误用容器指标会违背用户确认的监控范围。

Linux 的 `/proc/net` 指向 `/proc/self/net`，对应读取进程的网络命名空间；文件系统容量也需要对正确的宿主机路径查询，见 [Linux 内核 proc 文档](https://docs.kernel.org/filesystems/proc.html)。GPU 采集还涉及设备访问与驱动工具或库，见 [NVIDIA 容器配置](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/docker-specialized.html)。

## Considered Options

- 容器直接采集宿主机：需要额外的主机路径、网络和 GPU 配置。使用 host 网络还会改变现有端口映射与网络隔离，见 [Docker host 网络说明](https://docs.docker.com/engine/network/drivers/host/)。
- 宿主机采集进程：在主机的网络和挂载范围内采集，通过本地连接向 API 提供指标，保留现有 API 容器隔离方式。

选择宿主机采集进程，接受 Docker 部署需额外管理一个进程的成本，以明确主机观测范围并减少 API 对主机路径和设备的直接访问需求。用户已在需求访谈 Q13 中接受此取舍。

## Consequences

- 需要提供采集进程的发布、启动和升级说明，以及 Unix socket 权限配置。
- API 与采集进程必须共用资源指标的语义，避免直接部署与 Docker 部署产生口径差异。
- 采集进程故障不应拖垮 API；面板按区域报告采集失败与过期，避免将旧值视为实时数据。
- 最近 30 分钟历史归属 API 实例内存；API 重启后清空，采集进程重启时 API 保留历史并留下中断缺口。
- 采集使用专用用户及必要读取权限，设置超时，同一采集任务不重叠运行；具体采集依赖在实现时选择。
- 指标口径、异常规则与验收要求见 [服务器监控模块需求与设计](../server-monitoring.md)。
- 正式实现契约见 [Spec](../server-monitoring-spec.md)，端到端切片和阻塞依赖见 [ticket 拆分](../server-monitoring-tickets.md)。
