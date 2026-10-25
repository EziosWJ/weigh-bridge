> **历史档案（2026-10-24 归档）**：以下保留当时的决定、计划或验证事实；其中“当前／待实施／已授权”均指原记录时点。开发与验收使用[当前规格](../../requirements/standalone-site-spec.md)，追溯入口见[历史索引](../README.md)。旧步骤不作为本轮部署或数据操作指令。

# Spec #64 实现与验证记录

对应 [Spec #64](https://github.com/EziosWJ/base-project-golang/issues/64) 与 #65–#71。功能代码已集成；验收状态以以下实际检查和硬件范围为准，不关闭缺少硬件证据的工作项。

## 实现范围

- 新增 ADMIN 专属“系统监控 → 服务器监控”菜单和 `/monitor/server` 页面，双数据库 seed 版本均为 `00004`。
- 接口为 `GET /api/v1/monitoring/overview` 和 `GET /api/v1/monitoring/history?resource=cpu&device=`。两者使用现有 Bearer 登录态及当前数据库中启用、未删除的 ADMIN 关系。
- CPU 总体/逐核、系统负载、内存/Swap、文件系统容量、块设备速率/IOPS、逐网卡计数/速率/链路、NVIDIA GPU 和 CPU/内存 Top 10 进程均使用真实采集。
- 原生 Linux 在 API 内部采集；Docker API 仅从指定宿主机 Unix socket 读取，不回退到容器指标。非 Linux 返回不支持，容器配置原生采集时拒绝启动并说明需要 Unix 来源。
- 每 5 秒后台采样，单区域超时或失败不会阻塞其他有效区域；同一采集任务不会重叠。Linux 区域预算最多 3.5 秒，为 API 默认 4 秒超时留出返回余量。
- 各区域独立保留有效时间、支持情况和局部失败说明。超过 15 秒标为过期，失败保留旧值时明确显示说明。新增设备、计数重置、中断后重新建立基线，缺失速率为 `null`。
- API 实例保留有界的最近 30 分钟内存历史，历史使用各区域实际采集时间，失败/重复旧样本记录缺口。API 重启清空，采集进程重启保留已有历史。
- CPU/内存 ≥90% 持续 60 秒触发，<85% 持续 30 秒恢复；文件系统 ≥90% 立即触发，<85% 恢复。中断重置持续时间判断；局部采集失败不视为容量异常恢复。
- 页面每 5 秒非重叠刷新，清理卸载后的轮询和请求；趋势按资源/设备切换，缺口断线，默认隐藏回环与 veth，可显示全部接口。
- CPU/内存使用紧凑容量条与等高卡片；下方文件系统、块设备、网卡、GPU、进程通过 Tab 切换，支持键盘操作。文件系统默认隐藏临时及运行时挂载、合并同源重复项，保留全部挂载点开关；趋势设备选择同步筛选。
- API Swagger 已重新生成；新增发布二进制 `weighbridge-host-collector`，启动和维护方法见 [部署说明](../../server-monitoring-deployment.md)。

## 自动化验证

| 检查 | 覆盖 |
| --- | --- |
| `task backend:check` | 后端测试和 go vet；真实 HTTP 下的旧值、过期、历史窗口、告警边界/持续时间/恢复/中断及设备查询 |
| `task db:integration:sqlite` | 真实 SQLite migration、菜单、普通角色和 ADMIN 撤销/禁用/软删后的访问 |
| `task db:integration:postgres` | 临时 PostgreSQL 容器中的相同 migration/HTTP 契约 |
| `task frontend:lint`、`task frontend:build` | 前端静态检查与生产构建 |
| `task frontend:monitoring` | Playwright：菜单/路由、真实权限拒绝、设备切换、状态/旧值/过期、逐核空值、趋势断线、390px 布局与卸载轮询清理 |
| `task build:check` | 内嵌前端的发布二进制及 embedweb 测试/go vet |
| `go test -race ./internal/monitoring ./cmd/host-collector` | 采样/缓存/socket/部分挂载超时的并发检查 |
| `task host:smoke` | 真实 Linux 主机身份、网卡和挂载范围，首样本、重复读取、socket 权限及重启基线 |
| `task monitoring:smoke:docker` | 临时数据中的原生/隔离 Docker API、socket 权限、断开/过期、采集重启与历史保留 |

以上检查均已通过。监控模块包含 16 个针对性测试，另有配置、双库集成与浏览器验证。

测试使用可推进时间，不依赖等待真实 30 分钟或告警完整持续时间。数据库测试只使用临时数据库，宿主机烟测不改变实际部署数据。WSL 的 9p mountinfo 选项含空格导致原采集库解析失败，已在 Linux 边界修复并加入真实格式回归测试；慢挂载点有独立互斥及最多 8 个并行 Statfs，其他挂载点仍可返回。

## 实际硬件验证范围

2026-10-04，Linux WSL 宿主机 `skyrim`：20 个逻辑核、10 个物理核，40 个文件系统、6 个块设备，网卡 `eth0`、`eth1`、`eth2`、`eth3`、`lo`、`loopback0`。宿主机 socket 验证时间保持、停止清理、重启来源变化及 CPU/磁盘/网卡首速率为空。

NVIDIA 实际单卡为 GeForce RTX 4060，驱动 610.88，显存 8188 MiB。利用率、显存和温度可读，功耗返回 `[N/A]`，接口正确保留 `null` 并说明字段不支持。多卡的 CSV/页面路径有确定输入验证，但多张 NVIDIA 实际硬件、完整原生 Linux NVIDIA 部署及实际可用功耗字段仍待硬件验收。模拟响应不能替代上述验收。

当前环境未安装 `golangci-lint`，因此未执行该额外工具；已使用项目统一测试/go vet 与 race 检查。项目既有 Vite chunk 大小与 Browserslist 提示未在本次功能中调整。

## Issue 验收结论

- #65–#69：已有功能、自动化检查和宿主机/Docker 对照记录，满足关闭条件。
- #70：进程排行功能和页面验证已实现，但尚缺进程排序、CPU 归一化、进程退出等针对性行为测试，以及 Docker 场景的宿主机进程 PID/名称身份对照记录；保留开放。
- #71：单卡 RTX 4060 已观察，尚缺多卡真机及完整原生 Linux NVIDIA 部署验收；保留开放。
- 父 Spec #64：待 #70、#71 的验收缺口补齐后关闭。

最新 UI 调整已再次通过 `task frontend:lint`、`task frontend:build` 和 `task frontend:monitoring`；浏览器验证覆盖 Tab 键盘切换、同源去重、全部挂载恢复、趋势同步筛选，以及保留 `/run/media` 下的独立存储。

Docker 实际验收同日通过：非 root、删除全部 Linux capabilities、独立网络的容器主机名为 `spec64-isolated-api`，API 概览的主机名仍为宿主机 `skyrim`，网卡集合与宿主机完全一致，43 个当时可读挂载点均属于宿主机挂载范围。临时 Docker 挂载使挂载数量与先前原生烟测不同。socket 权限错误连续两轮保留原有效时间；断开后采集失败并过期，重启采集进程后重建速率基线、API 保留已有历史和缺口。通过 Docker API 同样读到 RTX 4060，功耗为空。测试使用临时 SQLite 数据和临时 socket，退出时完成进程、容器和文件清理。证据由 `task monitoring:smoke:docker CLI_ARGS='--output /tmp/spec64-monitoring-docker.json'` 生成。
