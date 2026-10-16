# Spec：Linux 宿主机资源监控面板

范围依据：已确认的服务器监控需求与 ADR-0011。用户已确认测试边界、ticket 拆分与发布。目标 tracker 为本仓库 GitHub Issues，发布标签为 `ready-for-agent`。

已发布：[Spec #64](https://github.com/EziosWJ/base-project-golang/issues/64)。实现工作项与阻塞依赖见 [ticket 索引](server-monitoring-tickets.md)。

本文件保留首版 Linux 契约。2026-10-14 的[增量 Spec #77](server-monitoring-extension-spec.md) 将平台目标扩展至 Windows Server 2019/2022 原生 API 部署，增加 IPv4/IPv6 网卡 IP 弹窗，并将模块改为默认关闭；开关关闭时不执行本文件描述的持续采集和历史维护。涉及平台范围、默认启用及网卡地址时，以增量规格为准。

## Problem Statement

管理员需要在现有后台判断本项目所在服务器是否存在 CPU、内存、磁盘或网络瓶颈，并定位主要占用进程。现有应用存活、数据库就绪与 Prometheus 应用指标不能提供完整的宿主机资源面板。Docker 部署还存在将容器指标误认为整机指标的风险，多网卡、多磁盘与 GPU 的支持情况也需要明确展示。

## Solution

增加仅 ADMIN 可访问的“系统监控 → 服务器监控”，监控本项目 API 所在的一台 Linux 宿主机。提供资源概览、最近 30 分钟趋势、文件系统容量、块设备读写、逐网卡流量、NVIDIA 多卡和 CPU/内存进程 Top 10。每 5 秒采样，后台持续运行；异常在面板提示，数据缺失与过期明确标注。

API 直接部署时内部采集；Docker 部署时由同一宿主机上的采集进程通过本地 Unix socket 提供最新样本。历史由 API 实例保存在内存中。两种部署使用相同指标语义与访问规则。

## User Stories

1. As an ADMIN 管理员, I want to 在系统监控菜单打开服务器监控, so that I can 在现有后台排查主机资源问题。
2. As an ADMIN 管理员, I want to 查看主机名、系统版本和运行时长, so that I can 确认正在查看的服务器。
3. As an ADMIN 管理员, I want to 查看 CPU 型号、核心数、总体及逐核利用率, so that I can 判断整机与单核瓶颈。
4. As an ADMIN 管理员, I want to 查看 1、5、15 分钟系统负载, so that I can 了解近期任务压力。
5. As an ADMIN 管理员, I want to 查看内存总量、已用量、可用量和占用率, so that I can 判断剩余承载能力。
6. As an ADMIN 管理员, I want to 查看 Swap 使用情况, so that I can 补充判断内存压力。
7. As an ADMIN 管理员, I want to 按文件系统挂载点查看容量, so that I can 找到接近用满的存储空间。
8. As an ADMIN 管理员, I want to 按块设备查看读写速率和 IOPS, so that I can 区分容量不足与读写繁忙。
9. As an ADMIN 管理员, I want to 同时查看多张网卡的收发速率和累计流量, so that I can 判断每条网络通路的负载。
10. As an ADMIN 管理员, I want to 查看网卡链路状态、错误和丢包, so that I can 发现网络异常线索。
11. As an ADMIN 管理员, I want to 默认隐藏回环和 veth 并能显示全部接口, so that I can 在清晰概览与完整排查之间切换。
12. As an ADMIN 管理员, I want to 在链路速率可用时查看收发带宽占用率, so that I can 判断网卡是否接近带宽上限。
13. As an ADMIN 管理员, I want to 分别查看 NVIDIA 多卡的型号、利用率、显存、温度和功耗, so that I can 判断每张显卡的资源状况。
14. As an ADMIN 管理员, I want to 查看 CPU 和内存占用 Top 10 的进程名、PID 与占用量, so that I can 定位主要占用进程。
15. As an ADMIN 管理员, I want to 查看最近 30 分钟趋势并切换设备, so that I can 区分短时波动与持续压力。
16. As an ADMIN 管理员, I want to 看到达到持续条件的 CPU 与内存异常提示, so that I can 优先关注持续资源不足。
17. As an ADMIN 管理员, I want to 看到接近用满的文件系统立即提示, so that I can 及时处理空间不足。
18. As an ADMIN 管理员, I want to 看到异常按明确恢复规则消退, so that I can 理解提示状态的变化。
19. As an ADMIN 管理员, I want to 查看各区域的采集时间和过期状态, so that I can 判断数据是否仍代表当前情况。
20. As an ADMIN 管理员, I want to 区分无设备、不支持和采集失败, so that I can 避免将缺失数据误认为零占用。
21. As an ADMIN 管理员, I want to 在部分设备采集失败时继续查看其他资源, so that I can 继续排查问题。
22. As an ADMIN 管理员, I want to 在首次采样时看到采集中并查看实际已有历史, so that I can 正确理解尚未积累的数据。
23. As an ADMIN 管理员, I want to 在趋势中看到真实采集缺口, so that I can 避免将中断误认为连续正常。
24. As an ADMIN 管理员, I want to 在设备新增或计数重置时看到重新建立速率基线, so that I can 避免误读负速率和尖峰。
25. As an ADMIN 管理员, I want to 关闭页面后仍持续采样, so that I can 再次进入时查看已积累的趋势。
26. As an ADMIN 管理员, I want to API 重启后从空历史重新开始, so that I can 理解短期历史的生命周期。
27. As an ADMIN 管理员, I want to 采集进程重启后保留 API 已有历史并标注缺口, so that I can 继续分析重启前后的情况。
28. As a Docker 部署者, I want to 通过宿主机采集进程取得整机指标, so that I can 避免把容器资源当成宿主机资源。
29. As a Linux 部署者, I want to 以专用用户和必要读取权限运行采集并获得启动升级说明, so that I can 持续维护采集服务。
30. As a 系统维护者, I want to 菜单、页面与接口均检查 ADMIN 权限, so that I can 控制主机和进程信息的访问范围。
31. As a 系统维护者, I want to 采集限时且同一任务不重叠, so that I can 避免采集故障拖累 API。
32. As a 系统维护者, I want to 明确 Linux、NVIDIA 与实际硬件验证范围, so that I can 区分已支持、未支持与未验证能力。

## Implementation Decisions

- 遵循 ADR-0011：单台 Linux 宿主机资源监控；直接部署内部采集，Docker 部署通过本机采集进程和 Unix socket 读取最新样本。
- 新增独立的资源监控业务模块，接入现有 HTTP 依赖装配、认证、统一响应与管理后台路由。采集依赖在模块边界隔离，不引入全局任务框架。
- 提供版本化的只读概览与历史查询接口，同步 API 文档。快照包含主机身份、各资源数据、各区域有效采集时间和支持/失败状态；历史支持按资源与设备查看最近 30 分钟样本及缺口。
- 复用现有 ADMIN 业务授权模式，从当前数据库角色关系判断启用且未删除的 ADMIN；权限不依赖 JWT 中的角色或仅依赖菜单可见性。
- 在两种正式支持的数据库中增加版本锁步的目录、监控菜单及 ADMIN 关系种子 migration。API 启动不自动迁移；不为指标或历史新增数据库表。
- 每 5 秒采样并刷新页面数据，页面关闭后仍采样。同一任务不重叠，采集设超时，单一区域失败不阻止其他区域有效结果。
- 历史由当前 API 实例内存保存，按 30 分钟时间窗口淘汰，内存有界；API 重启清空，多个 API 实例历史独立。
- 采集进程重启保留 API 历史，中断形成缺口，恢复后重新建立速率基线。重复读取同一旧样本不能使其变为新鲜数据。
- 各区域分别展示正常、无设备、不支持、采集失败和过期；超过 15 秒无有效新样本时标为过期。字段不支持、权限不足与旧值保留均需说明。
- CPU 总体与进程占用以整机总算力为 100%，逐逻辑核以该核为 100%；系统负载展示 1、5、15 分钟原始负载值。
- 内存已用为总量减可用量，占用率为已用除以总量；进程内存以常驻物理内存量排序；未配置 Swap 明确展示状态。
- 文件系统总量、已用、普通用户可用量分别展示；占用率为已用除以已用与可用之和。块设备读写速率及 IOPS 单独展示。
- 磁盘与网络速率用相邻有效计数差值除以实际间隔；设备新增或计数重置时重新建立基线。首个速率样本显示采集中，不填零、不产生负速率。
- 网卡默认隐藏回环和 veth，可显示全部；收发速度使用 MiB/s，1 MiB 为 1,048,576 字节。链路速率已知时转换为 bit/s 后分别计算收发带宽占用率，不直接叠加所有接口流量。
- NVIDIA 多卡逐卡展示型号、利用率、显存、温度、功耗；各字段以设备支持情况为准，GPU 不可用不阻止其他区域展示。
- CPU、内存各提供 Top 10 进程名、PID 和占用量；读取权限不足时保留有效结果并标识覆盖不完整。
- CPU、内存 ≥90% 持续 60 秒后提示，<85% 持续 30 秒后恢复；文件系统 ≥90% 立即提示，<85% 恢复。固定规则在页面说明。
- GPU、网络显示状态与错误信息，高利用率或高流量本身不判为异常。采样中断中止对应持续时间判断，恢复后重新计时。
- 页面顶部显示主机信息、采集时间和资源概览，下方显示趋势、磁盘、网卡、GPU 和进程排行；多设备可切换趋势，历史不足照实展示，缺口不连接为连续曲线。
- 新运行配置沿用现有 YAML 和 APP 环境变量覆盖机制；Docker 部署说明包含采集进程启动、升级、Unix socket 目录与权限、状态检查方法。
- 具体采集库在实现时按真实需求选择并说明理由；不预先加入时序数据库、Redis、远端管理或分布式基础设施。

## Testing Decisions

- 主要测试边界沿用现有真实 HTTP 路由：组装真实监控业务服务，以可控采集输入和可推进时间验证概览、历史、权限与状态。断言外部结果，不断言私有缓存结构、采集库调用顺序或具体 goroutine 数量。
- 现有应用装配 HTTP 测试、通知模块非 ADMIN 返回 403 的路由测试、认证中 Bearer 缺失返回 401 的测试提供先例；新增监控场景需覆盖真正普通角色，以及 ADMIN 撤销或禁用后的拒绝访问。
- 时间测试沿用登录防护可推进时钟的方式，验证 15 秒过期、30 分钟淘汰、60 秒触发、30 秒恢复、阈值边界与中断重置，不靠真实长时间睡眠。
- 用确定的累积计数和不同实际间隔验证 CPU 归一化、内存与文件系统比例、磁盘和网络速率、IOPS、计数重置与设备新增。
- 在后台采样入口验证取消退出、超时和不重叠；可控输入产生部分失败时，其他有效区域继续返回，旧样本不被重新标为新鲜。
- 菜单、角色关联和访问契约复用现有 PostgreSQL 临时容器与 SQLite 临时文件的真实 migration/HTTP 集成测试，覆盖重复 migration、双库菜单版本一致、ADMIN 可见及普通角色拒绝。
- 前端沿用现有 Playwright 浏览器脚本，以可控监控 HTTP 响应验证菜单、资源展示、设备切换、无设备/不支持/失败/过期、首屏和趋势缺口、页面卸载后的轮询清理；不新建前端单元测试框架。
- Docker 部署通过真实 Linux 宿主机上的 Unix socket 采集场景，对照主机身份、网卡及挂载点，验证宿主机范围、权限错误、断开与采集进程重启。应用保持现有隔离方式。
- GPU 支持需要 NVIDIA 真机记录型号、驱动、支持字段和观察结果；模拟响应验证不能代替真机验收，没有硬件时明确列为未验证。
- 后端执行项目统一检查；前端执行 lint、构建和新增监控浏览器验证。菜单 migration 覆盖 PostgreSQL 与 SQLite；长时间联调具备超时、健康检查与清理。
- 无需前置重构：现有 HTTP、认证、业务授权、可控时间和浏览器测试入口足够承载本功能；只增加监控模块必需的采集输入边界。

## Out of Scope

- 多台服务器、远端采集注册、跨实例共享历史。
- Ping、端口、HTTP 主动可用性探测。
- Windows/macOS 正式硬件采集支持，AMD/Intel GPU 正式支持。
- 超过 30 分钟的持久历史、时序数据库、Prometheus 平台部署。
- 站内通知、邮件或外部告警通道，用户编辑异常阈值。
- 终止进程、修改设备配置、自动修复、硬件管理操作。
- SMART 全面磁盘健康、CPU 温度与风扇等未确认硬件指标。
- 通用 permissionCode 拦截框架、权限引擎或全局后台任务平台。

## Further Notes

- 本 Spec 将四轮已确认需求整理为实现契约，遵守 ADR-0011；术语使用资源监控、可用性探测、宿主机采集进程。
- 用户已确认测试边界与 ticket 拆分，发布标签为 ready-for-agent；各实现切片引用本 Spec，并声明真实阻塞依赖。
- 首版实现完成需要实际验收证据。未提供 NVIDIA 真机验证时，不能宣称 GPU 验收通过。
