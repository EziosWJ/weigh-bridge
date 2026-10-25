# Agent 本机恢复与旧维护服务

日常维护使用业务系统[设备维护页](agent-remote-maintenance.md)，安装和升级见[本厂部署](local-deployment.md)。独立 `weighbridge-agent-maintenance` 是旧部署过渡服务，完整安装、密码恢复、队列动作和历史验证见[原部署档案](../history/device/agent-local-maintenance.md)；不作为新安装必需进程。现场确认统一维护可用后再退役旧服务。

## 主进程离线恢复

Agent 主进程保留时，通过网页检查、保存应用和恢复设备执行；整个进程离线时使用其宿主机 systemd、journal 和实际配置恢复。检查有效 unit、drop-in、环境文件、运行账号和状态目录，避免更新模板后仍加载旧身份或相机参数。

串口只由 Agent 打开。维护状态中的最后读数、磁盘快照或文件保存成功不代表设备当前可用；过期、未运行或无法读取时显示实际状态。保留原打印账本及未决操作，重启不推定未出纸。

## CUPS 与服务权限

`lpstat` 只读查询由 Agent 维护模块在本机执行。取消作业、`cupsenable` 及打印提交遵守 CUPS 用户／组权限；按现场系统策略给实际服务账号所需的最小授权。系统级 CUPS restart 单独授权，不以 root 运行整个维护 Web 或授予通用 sudo／systemctl。

使用 polkit 的发行版，可由系统管理员配置仅允许专用账号 restart `cups.service` 的本机规则，限制 unit、动作和用户并实际复核。串口组／ACL 按 Agent 账号持久配置，组变更可能需重启服务会话；实验临时 ACL 不作为重启后权限已通过的证据。

维护命令与参数由实现限定，页面字段不用于指定可执行文件或附加任意参数。CUPS 输出按 `LC_ALL=C` 解析，`lpstat -o` 尾部只作提交信息，不推定为可靠标题。未决或无法排除出纸的作业先人工核实，不通过清队列或重建账本绕过。

## 旧服务资料

仅排障既有旧部署时，按[历史安装与升级](../history/device/agent-local-maintenance.md#构建和安装)、[旧密码恢复](../history/device/agent-local-maintenance.md#密码恢复)及对应版本配置操作。历史中的草稿、连接导入、Agent 相机和独立密码不是当前产品要求。实际完成度与现场未验收项见[当前状态](../next.md)。
