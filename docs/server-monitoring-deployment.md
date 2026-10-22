# 宿主机监控部署

服务器监控默认关闭，配置 `monitoring.enabled: true` 或 `APP_MONITORING__ENABLED=true` 后重启 API 才开始采集。关闭时不初始化采集器、不启动监控任务、不维护历史，导航隐藏服务器监控；已授权 ADMIN 直接请求概览或历史得到 HTTP 503，响应 `data.reason` 为 `monitoring_disabled`。401/403 授权语义、`/health`、`/ready` 和应用 `/metrics` 不变。升级已有部署时请显式开启需要保留的资源监控。

监控范围是 API 所在的单台宿主机。Linux 和 Windows 原生 API 使用 `native`；Linux Docker API 使用 `unix` 并连接同宿主机采集进程。Windows Server 2019/2022 为本次原生部署目标，Windows Docker/WSL 监控外部 Windows 宿主机不在支持范围。不能用容器内 `/proc`、`/sys` 或容器挂载点代替宿主机资源，采集失败不会自动切换来源。

## 采集依赖和权限

Linux 的 CPU、内存、负载、进程和网卡累计计数继续使用 `prometheus/procfs`，块设备使用同库的 `blockdevice`，文件系统容量使用 `x/sys/unix.Statfs`。Windows 的 CPU、内存与网络计数使用 `gopsutil/v4`，主机及进程信息使用只读 WMI，卷容量、磁盘 I/O、网卡链路使用 Win32 查询。新增依赖仅在 Windows 采集边界使用；不导入会自动修改磁盘计数注册表或启用调试权限的采集包。NVIDIA 使用驱动自带的 `nvidia-smi` 只读查询，命令有超时；不安装驱动、不改变 GPU 配置。字段返回 `N/A` 时保留为空并说明不支持，GPU 身份使用 UUID。

使用专用用户 `base-monitor`，无需给予 API 容器 root、host 网络或特权权限。普通 Linux `/proc` 通常允许读取基础进程统计；启用 `hidepid`、安全策略或挂载访问限制时，页面会显示覆盖不完整或失败。按主机已有的 proc 读取组策略授权专用用户，避免为方便采集授予任意 sudo。NVIDIA 权限遵循宿主机驱动设备的现有访问组策略。

## 宿主机采集进程

在 Linux 宿主机构建发布二进制：

```bash
task host:build
sudo groupadd --system base-monitor
sudo useradd --system --gid base-monitor --no-create-home --shell /usr/sbin/nologin base-monitor
sudo install -o root -g root -m 0755 bin/weighbridge-host-collector /usr/local/bin/weighbridge-host-collector
```

已存在的组和用户无需重复创建。安装以下 `/etc/systemd/system/weighbridge-host-collector.service`，socket 目录由 systemd 创建，其用户和组均为 `base-monitor`，目录权限 `0750`，socket 权限 `0660`：

```ini
[Unit]
Description=Base Go Linux host resource collector
After=local-fs.target

[Service]
Type=simple
User=base-monitor
Group=base-monitor
RuntimeDirectory=server
RuntimeDirectoryMode=0750
ExecStart=/usr/local/bin/weighbridge-host-collector -socket /run/weighbridge/host-monitor.sock
Restart=on-failure
RestartSec=3
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

不要设置 `PrivateNetwork`、`PrivateMounts`、`PrivateDevices`、`PrivateUsers`、`ProtectSystem`、`ProtectHome`，否则可能改变待采集资源范围或阻止驱动访问。挂载命名空间沙箱会改变挂载点列表，因此采集服务应保持宿主机挂载范围。

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now weighbridge-host-collector
sudo systemctl status weighbridge-host-collector --no-pager
sudo journalctl -u weighbridge-host-collector -n 30 --no-pager
timeout 5 sudo -u base-monitor curl --fail --unix-socket /run/weighbridge/host-monitor.sock http://localhost/snapshot
```

开发启动入口为 `task host:collector CLI_ARGS='-socket /tmp/weighbridge-host-collector.sock'`；正式部署使用上述 `/run` 路径。采样周期固定为 5 秒，单次 Linux 采集预算最多 3.5 秒，为 API 的默认 4 秒超时留出返回余量，同一采集任务不会重叠。socket 只返回当前样本，不储存历史。

## API 配置和 Docker 映射

在既有 YAML 配置内设置：

```yaml
monitoring:
  enabled: true
  source: unix
  socket_path: /run/weighbridge/host-monitor.sock
  timeout: 4s
```

环境变量覆盖名称沿用现有双下划线嵌套规则：`APP_MONITORING__ENABLED`、`APP_MONITORING__SOURCE`、`APP_MONITORING__SOCKET_PATH`、`APP_MONITORING__TIMEOUT`。原生 API 设置 `enabled: true` 和 `source: native`，无需启动额外进程。采样周期仍为 5 秒，历史仍保存在当前 API 实例的内存中，窗口为 30 分钟。

现有 `server/docker-compose.dev.yml` 已接入目录映射及访问组，但默认关闭监控。需要监控时，在 Compose 环境中显式设置 `APP_MONITORING__ENABLED=true`，并将 `HOST_MONITOR_SOCKET_DIR=/run/weighbridge` 与 `HOST_MONITOR_GID` 写入部署环境；后者为 `getent group base-monitor` 显示的数值 GID。启用后的等价 API 服务配置如下：

```yaml
services:
  api:
    environment:
      APP_MONITORING__ENABLED: "true"
      APP_MONITORING__SOURCE: unix
      APP_MONITORING__SOCKET_PATH: /run/weighbridge/host-monitor.sock
    volumes:
      - "${HOST_MONITOR_SOCKET_DIR:-/run/weighbridge}:/run/weighbridge:ro"
    group_add:
      - "${HOST_MONITOR_GID}"
```

挂载目录而不是单个 socket 文件，可让采集进程重启后新建的 socket 继续被容器看到。API 容器用户需要该目录的执行权限与 socket 的组读写权限；Unix socket 只允许获授权的本机用户访问，不暴露 TCP 端口。

## 升级和范围验收

### Windows 原生部署

目标为 Windows Server 2019/2022 amd64 原生运行 API。使用现有配置文件和数据库/JWT 配置机制，启用：

```yaml
monitoring:
  enabled: true
  source: native
  timeout: 4s
```

也可在当前 PowerShell 会话设置 `$env:APP_MONITORING__ENABLED = "true"`、`$env:APP_MONITORING__SOURCE = "native"` 后启动 API。无需独立采集进程或 Unix socket。使用专用运行账号，按现有 Windows/WMI 权限策略授权；读取不完整时明确展示失败或权限不足，不自动提权、不启用调试权限、不修改性能计数器设置。NVIDIA 需要驱动提供的 `nvidia-smi.exe` 可从 API 进程 PATH 找到。

Windows CPU 与进程 CPU 按整机总算力归一化；物理内存已用为总量减可用量。Linux 1/5/15 分钟系统负载返回 `null` 并说明不支持；Swap 标记 `swapUnsupported`，不把 Windows 分页文件伪装成 Linux Swap。卷容量使用 Win32 提供的总量、实际空闲和调用账号可用量；存在账号配额时按系统返回口径读取并提示。磁盘性能计数器未启用或不支持时明确显示原因，首次速率样本重新建立计数基线。

Linux 与 Windows 网卡均返回 `addresses`（`family`、`ip`、`prefixLength`）和可选 `addressError`。在网卡表格点击“查看 IP”，弹窗随每 5 秒快照更新全部 IPv4/IPv6 CIDR 地址。空集合显示“未分配 IP”；地址读取失败或旧采集端未提供字段时显示不可用原因；接口消失时显示已不可用。Docker 场景的地址来自宿主机采集进程，不能由 API 容器替代。

执行 `task backend:windows` 生成 Windows amd64 API、迁移程序与监控测试二进制，采用 `CGO_ENABLED=0`，适用于 PostgreSQL。Server 统一使用 PostgreSQL，不要求 CGO 或 C 编译器。该任务仅构建 API，不包含内嵌前端资源；完整发布仍使用项目既有前端与发布流程。

在 Windows 或启用 Windows 互操作的 WSL 中执行 `task monitoring:smoke:windows`，运行 Windows 专属计数测试及只读真机采集。对照系统工具验收 CPU、内存、卷容量、磁盘、网卡/IP 和进程，并记录账号权限及 NVIDIA 支持字段。Windows 11 上的验证只作为额外证据，不能替代 Server 2019/2022 验收。

### Linux 升级与验收

重新构建二进制，使用 `install` 替换 `/usr/local/bin/weighbridge-host-collector`，再执行 `sudo systemctl restart weighbridge-host-collector`。不要删除 API 历史：采集进程重启仅重新建立 CPU、块设备、网络和进程基线，API 的已有历史保留，中断会形成缺口。API 重启才清空其自身的 30 分钟内存历史。

真实 Linux Docker 验收需要记录：宿主机 `hostname`、`cat /etc/os-release`、`ip -brief link`、`findmnt`，与 socket 样本和 API 概览的主机名、网卡和挂载点对照。再以带超时的请求验证错误组权限、停止采集进程、重启采集进程后首个速率样本的采集中状态和历史缺口。每项验证完成后恢复服务和原有权限，清理临时 socket 与测试进程。

执行 `task host:smoke`，使用临时 socket 验证真实宿主机身份、网卡、挂载点、首个速率样本、重复读取时间与采集进程重启后的新来源和基线。脚本自动清理进程及 socket，每次等待最多 12 秒，可用 `task host:smoke CLI_ARGS='--output /tmp/host-monitor-evidence.json'` 留存证据。

执行 `task build` 后运行 `task monitoring:smoke:docker CLI_ARGS='--output /tmp/monitoring-docker-evidence.json'`，可验证真实原生 API 和使用临时 PostgreSQL 数据、非 root 用户、独立网络及只读 socket 挂载的 Docker API。脚本依赖 `golang:1.26.5-bookworm` 和 `postgres:17-alpine` 镜像，验证 socket 权限失败、断开过期、重启基线、已有历史和缺口，并自动清理临时进程、容器和数据库。页面浏览器验证入口为 `task frontend:monitoring`。

2026-10-04 在 Linux WSL 宿主机 `skyrim` 上运行该脚本通过：20 个逻辑核、10 个物理核、40 个可读取文件系统、6 个块设备，以及 `eth0/eth1/eth2/eth3/lo/loopback0` 接口均与主机范围一致。socket 权限 `0660`、重复拉取的有效时间保留、采集进程退出后 socket 清理、重启后来源身份更新和速率基线重建均通过。WSL 的 9p 挂载选项含空格，采集器按固定分隔符解析已作真实及回归验证。

NVIDIA 真机验收必须记录 GPU 型号、驱动版本、UUID、各字段支持情况和观察结果，至少覆盖声明支持的多卡场景与 Docker API 读取宿主机样本的场景。模拟 CSV、无 GPU 机器上的测试和编译通过均不等于 GPU 真机验收通过。当前 Linux WSL 环境只读查询确认单张 NVIDIA GeForce RTX 4060，驱动 610.88，显存 8188 MiB，温度字段可用，功耗返回 `[N/A]`；多卡与完整 Linux NVIDIA 部署仍待真机验收。

指标口径依据 [Linux I/O statistics](https://docs.kernel.org/admin-guide/iostats.html) 和 [NVIDIA nvidia-smi](https://docs.nvidia.com/deploy/nvidia-smi/index.html)：块设备扇区累计数转换采用内核统计的 512 字节单位，NVIDIA 不支持的查询字段不能解释为零。
