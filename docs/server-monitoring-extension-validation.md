# Spec #77 实现与验证记录

日期：2026-10-14。对应[增量规格](server-monitoring-extension-spec.md)，启用方式与部署限制见[部署说明](server-monitoring-deployment.md)。

## 实现结果

- 增加默认关闭的 `monitoring.enabled`，遵循既有 YAML/环境变量覆盖机制；关闭时跳过采集器构造和后台任务、禁止历史写入，按实例过滤导航，保留认证/ADMIN 授权后返回 503 和 `data.reason: monitoring_disabled`。前端清除监控旧数据并停止轮询。
- Linux 与 Windows 网卡采集全部 IPv4/IPv6 地址及前缀。网卡操作按钮复用现有详情弹窗，随快照更新，区分无地址、读取失败、旧采集端缺字段、旧值/过期和接口消失。Linux socket 传输保留宿主机地址及原采集时间。
- Windows 原生采集主机、总体/逐核 CPU、物理内存、卷容量、物理磁盘 I/O、网卡流量/IP/链路、进程排行和 NVIDIA GPU。原生系统读取隔离在采集边界，Linux 系统负载返回 null，Swap 明确不支持；系统调用及 WMI 查询不修改注册表、磁盘配置或运行账号权限。
- 隔离现有 SQLite CGO 专属备份和错误映射，解决它们阻止 Windows PostgreSQL API 构建的问题。SQLite 功能仍依赖 CGO，没有换数据库驱动。
- 同步 Swagger、部署说明、原始规格与 ADR-0011 的增量边界，增加统一 Windows 构建与只读验收任务。

## 验证结果

| 验证 | 结果及范围 |
| --- | --- |
| `task backend:check` | 通过：全部后端单元测试及 go vet，包含默认开关、503/401/403、IP 解析、快照更新/失败旧值、Unix socket 地址传递与菜单过滤 |
| `task frontend:lint` / `task frontend:build` | 通过；构建仍有现有 Browserslist 数据及 bundle 大小提示 |
| `task frontend:monitoring` | 通过：IP 弹窗、多地址/CIDR、动态更新、读取失败、空地址、旧字段、接口消失、Windows 不支持字段、关闭清数据和停止轮询、隐藏菜单/直接路由及权限回归 |
| `task db:integration:sqlite` | 通过：完整 SQLite 集成集；监控契约同时验证启用/关闭实例共享菜单记录，且仍执行动态 ADMIN 授权 |
| `task backend:windows` | 通过：Windows amd64 API/迁移构建、监控测试编译及目标平台 go vet；关闭 CGO，使用 PostgreSQL 构建路径 |
| `task monitoring:smoke:windows` | 通过：当前 Windows 11 宿主机运行真实 Windows 二进制，验证主机、CPU、内存、卷容量、网卡/IP、进程及第二次 CPU/进程/磁盘速率样本；磁盘 I/O 与 NVIDIA GPU 返回正常状态 |
| `task host:smoke` | 通过：当前 Linux WSL 宿主机范围、socket 权限/时间、重启基线，以及全部 IP/前缀与宿主机 `ip -json address show` 逐接口对照 |
| `task backend:swagger` | 通过：生成并同步接口契约 |
| `task backend:integration` | 受环境阻碍：WSL 当前 Docker 命令为不可用的互操作入口，无法启动临时 PostgreSQL；没有改用实际部署数据库 |

Windows 真机验证先暴露了 WMI 卷/磁盘查询超过采集预算的问题，最终改用只读 Win32 卷枚举、容量及物理磁盘计数查询。最终 Windows 测试在 3.5 秒单次采集预算内通过，整个两次采样验证约 2–3 秒；这只是本机冒烟结果，不构成通用性能保证。

## 尚未验收

- Windows Server 2019/2022 真机与正式服务账号权限矩阵尚未验证；Windows 11 实测不能替代目标 Server 系统验收。
- Windows SQLite/CGO 发布、多 NVIDIA 卡及 Server 驱动矩阵尚未验证。
- Docker 宿主机端到端 API 场景与 PostgreSQL 集成测试需要恢复 Docker 后重跑；Linux 宿主机采集及 socket 往返已通过。
- 当前环境没有 `golangci-lint` 可执行文件，未运行该检查；后端测试和 go vet 已执行。

本次未修改数据库 schema、未执行实际部署迁移、未推送或发布运行服务。当前实际开发配置保留原状，基础配置默认关闭；需要监控时按部署说明显式开启并重启 API。
