# Agent 本地打印维护服务部署

日期：2026-10-21。本文描述现场反馈改版后的部署方式；本次改版尚未部署到真实工控机。首期历史见[首期交付记录](../requirements/agent-maintenance-delivery.md)，改版实现及验证见[改版交付记录](../requirements/agent-maintenance-revision-delivery.md)。

改版依据为[改版需求](../requirements/agent-maintenance-revision-spec.md)及 [ADR 0017](../adr/0017-agent-maintenance-config-and-lan.md)。默认监听、配置来源及页面布局已调整，部署前按下方升级步骤迁移旧参数。

## 组成和数据

`weighbridge-agent-maintenance` 是独立于 `weighbridge-agent-printer` 的 Go 服务，页面与静态资源嵌入单一二进制，不访问云端资源。它默认监听 `0.0.0.0:18901`，通过 HTTP 供本机及可信维护局域网访问。浏览器使用 `http://127.0.0.1:18901` 或 `http://<工控机局域网 IP>:18901`，不把 `0.0.0.0` 当作访问地址。TLS 为可选配置；程序允许通配、回环及私网 IP，拒绝显式公网 IP。必须使用防火墙把端口来源限制在地磅房管理网段，不得开放或转发公网端口。HTTP 不加密维护密码和会话，此可信内网边界已经用户确认。

维护密码首次启动时随机生成，明文只写入数据目录 `bootstrap-password.txt`（权限 0600）；首次成功登录后删除，首次登录前应通过本地受控方式保存密码。密码哈希、配置草稿、当前生效配置、上份配置和追加式维护记录存放在 `~/weighbridge-agent/`，目录应保持 0700 并纳入工控机受控备份。配置文件 `maintenance.json` 与打印 Agent 读取的 `agent.env` 均含连接密钥，均以权限 0600 保存，页面、日志和维护记录均不返回明文。Agent 心跳文件表达到 Server 的连接状态及当前运行配置摘要；维护页可据此核对运行配置与 `agent.env` 是否一致。打印 Agent 另以 0600 原子写入 `scale-status.json`，维护服务只读该文件展示仪表重量、稳定、最后有效时间、校验与故障，不打开串口；Agent 停止或文件过期时明确显示过期／未运行，不把旧 `stable:true` 当作当前重量。停止、过期或旧版本没有摘要时显示未知，不代表仪表、摄像头或正式称重可用。

打印尝试账本继续使用既有 `state` 目录，不由维护动作清除。首次启动会用 `.attempt-identity.json` 将无命名空间的历史尝试绑定到当时配置的 Server／Agent／地磅身份；后续身份使用独立子目录，切回原身份时回到原账本。Server 地址统一主机名大小写、默认端口与末尾斜线，拒绝查询参数、用户信息和片段。升级后第一次启动须沿用原身份，避免错误绑定历史账本。任何身份切换都不迁移、删除或重新提交既有尝试。

## 构建和安装

在仓库根目录执行：

```sh
task agent:printer:build
task agent:maintenance:build
```

将两个二进制放入 `~/weighbridge-agent/bin/`，将 `agent/deploy/weighbridge-agent-printer.service` 和 `agent/deploy/weighbridge-agent-maintenance.service` 复制到 `~/.config/systemd/user/`。为真实 Server、Agent 编号、地磅编号和密钥设置 `agent.env`，文件权限 0600；不要把真实密钥写入仓库或安装说明。维护 UI 启动后，在配置页选择已安装的 CUPS 队列并检查尺寸，再显式保存和应用。

以下命令在目标工控机由部署人员执行，源文件路径对应已复制的发布包；本次开发未执行这些现场操作：

```sh
mkdir -p "$HOME/weighbridge-agent/bin" "$HOME/.config/systemd/user"
chmod 700 "$HOME/weighbridge-agent"
install -m 755 bin/weighbridge-agent-printer bin/weighbridge-agent-maintenance "$HOME/weighbridge-agent/bin/"
install -m 644 agent/deploy/weighbridge-agent-printer.service agent/deploy/weighbridge-agent-maintenance.service "$HOME/.config/systemd/user/"
systemctl --user daemon-reload
systemctl --user enable --now weighbridge-agent-maintenance.service
systemctl --user enable weighbridge-agent-printer.service
```

维护页启动及配置／状态查询时重新核对 `agent.env`，不执行 shell。`maintenance.json` 保留草稿、上次配置及密码；外部修改环境文件时保留尚未应用的草稿。缺少 Server／Agent／地磅／队列项时显示导入错误，不把默认身份当作生效值。首次应用有效配置后才启动打印 Agent；现场按既有方式确认用户 linger 已启用。

用户级 systemd 服务分别管理打印 Agent 与维护服务。维护进程以普通专用用户运行，不使用 root。默认不需要证书即可在维护网段访问；如只需本机访问，通过维护服务 drop-in 设置 `--listen 127.0.0.1:18901`。启用 HTTPS 时同时提供 `--tls-cert` 和 `--tls-key`，只提供其中一个会拒绝启动。

打印 Agent 服务通过 `--env-file` 显式读取同一份 `agent.env`，文件内容优先于旧环境值，且不能再混用 `--server/--agent-id/--scale-id/--queue`。文件缺失时不会启动。部署时应先以管理员安全方式创建该文件，或先从本地维护页保存并应用有效连接配置，再启动 Agent。连接密钥不能通过维护页回显确认；可查看“已设置”提示与 Agent 状态。

## 从旧启动参数升级

先查看现有 unit 与 drop-in，记录 Server、Agent、地磅及打印目标的有效值。维护页不自动解析任意启动命令：

```sh
systemctl --user cat weighbridge-agent-printer.service
```

| 旧参数或来源 | agent.env 项 |
| --- | --- |
| `--server` | `WEIGHBRIDGE_SERVER_URL` |
| `--agent-id` | `WEIGHBRIDGE_AGENT_ID` |
| `--scale-id` | `WEIGHBRIDGE_SCALE_ID` |
| `--queue` | `WEIGHBRIDGE_PRINT_QUEUE` |
| 已有连接密钥 | `APP_WEIGHING__AGENT_KEY`，保留原值 |
| 已有打印校准参数 | 保留对应宽度、偏移与表长值 |

以下为字段示例，手工编辑已有文件并保留真实密钥，不能用占位符覆盖现场值：

```dotenv
WEIGHBRIDGE_SERVER_URL="https://server.example"
WEIGHBRIDGE_AGENT_ID="onsite-agent-1"
WEIGHBRIDGE_SCALE_ID=1
WEIGHBRIDGE_PRINT_QUEUE="Aisino-SK820-raw"
APP_WEIGHING__AGENT_KEY="REPLACE_WITH_EXISTING_AGENT_KEY"
WEIGHBRIDGE_PRINT_WIDTH_MM=175
WEIGHBRIDGE_PRINT_OFFSET_X_MM=0
WEIGHBRIDGE_PRINT_OFFSET_Y_MM=0
WEIGHBRIDGE_FORM_LENGTH_MM=99
```

先核对并处置未结束作业，再停止打印 Agent 和维护服务，备份原 unit、配置、维护数据及完整 `state` 账本。迁入有效值后安装新二进制与 unit，保持原有数据／账本目录及身份不变。不要自动删除或重提旧作业。

**仅替换仓库 unit 不能覆盖旧 drop-in。** 若旧 drop-in 仍带固定 `--server` 等参数，须编辑相应覆盖项，移除重复参数并采用以下启动行；不要删除与本次迁移无关的权限、字体等设置：

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/flock -n %h/weighbridge-agent/agent.lock %h/weighbridge-agent/bin/weighbridge-agent-printer --env-file %h/weighbridge-agent/agent.env --state-dir %h/weighbridge-agent/state
```

旧维护服务的 drop-in 如保留回环地址或 TLS 参数，也会覆盖新默认行为，按实际需要调整。完成后 `systemctl --user daemon-reload`，先启动维护服务并核对配置页，再明确应用打印配置。页面显示环境文件值、待应用差异及运行进程反馈；无摘要或反馈过期时保持未知，不将文件值直接等同于正在运行的进程值。

维护服务监听与 TLS 仍由启动参数管理，配置页显示当前访问方式。修改启动参数会使维护服务重启；原会话失效后重新登录。同一地址的状态刷新会在服务恢复后恢复读取，地址或协议改变时须用新地址重新访问。

## CUPS 与服务权限

`lpstat` 只读查询由维护服务直接运行。队列取消、`cupsenable` 和打印提交仍受 CUPS 用户/组授权约束；按现场系统策略把服务用户加入最小必要 CUPS 管理组。重启系统级 CUPS 需要单独授权。不要以 root 运行整个维护 Web 服务，也不要授予通用 `sudo systemctl` 权限。若发行版使用 polkit，可由系统管理员评估仅允许该专用用户对 `cups.service` 执行 restart 的本机规则；规则应拒绝其他 unit、动作和用户，并在现场复核授权效果。

维护 API 的命令固定为代码内列出的 `systemctl`、`lpstat`、`cupsenable`、`cancel`、`journalctl` 和 `lp` 动作及参数。页面字段不能指定可执行文件或附加任意参数。CUPS 输出按 `LC_ALL=C` 解析；`lpstat -o` 不提供可靠的作业标题，页面将尾部原文标为提交信息。

## 配置与队列动作

默认打印校准参数沿用 A4 已确认版面：基准宽 175 mm、整体缩放 1 倍、水平及垂直偏移 0 mm、连续纸表长 99 mm。连续纸升级版增加 0.5～1.5 倍整体缩放及小数毫米表长，维护台显示实际输出尺寸和分带走纸；当前纸张必须按实测重新校准，详见[SK-820 连续纸校准](sk820-continuous-paper-calibration.md)。宽度可设 100～210 mm，向右／向下偏移可设 0～30 mm，表长可设 85～300 mm；缩放后的宽度加右偏移不超过 210 mm，整幅图像及末尾 24 点分带补齐后的走纸不超过表长。不支持负偏移；越界会明确拒绝。偏移通过增加空白平移整幅内容，不缩窄表格或裁掉标题。测试样张与业务单据共用同一栅格渲染参数，但样张固定标记“测试／校准”，每次一份，不载入业务历史、不改变 Server 打印次数。CUPS 接收和作业完成均不证明纸面结果。

配置页面区分草稿与当前生效配置。应用或回退前先核查旧队列及新目标队列的未结束作业：保留作业意味着暂不应用；取消部分作业后仍有剩余作业，同样不能应用。应用前先停止打印 Agent，再检查队列变化，只有确认快照未变化后才写入受限环境文件并启动 Agent。失败时恢复原环境和生效项；恢复原 Agent 失败会明确报错，不隐藏在重新预览响应后。暂停标记让重启后的 Agent 在释放新打印请求前等待应用结束。维护中断遗留的暂停状态会在页面显示，需核查后应用或恢复配置，不自动放行未知配置。Agent 停止时已送达但尚未持久预留的请求仍可能处于 Server 未决状态，按既有未知结果流程核实，不会自动重提。

重启 CUPS 会影响本机全部队列；恢复暂停目标队列会影响其中保留的旧作业。重启 Agent、重启 CUPS 和恢复队列都先展示作业，停止 Agent 后比较确认前后快照，恢复队列／CUPS 后沿用 Agent 原启停状态；CUPS 查询失败或输出无法完整解析时只能在明确接受“状态未知”后继续恢复。取消作业须明确选中，并在执行后重新查询队列，每项取消独立记录。系统命令返回成功后还会查询 Agent、CUPS 或队列状态；核实失败会显示失败/未知，不把命令发出视为成功。

维护记录支持按日期、操作和结果在页面查询，筛选后分页，每页 100 条，历史记录不受最新 500 条截断限制，跨进程重启保留；日期按工控机本地时区筛选。不提供日志或记录导出。会话字段是本地维护会话 token 的不可逆摘要；共用密码不能表示实际操作者身份。

## 密码恢复

有本机文件系统权限的管理员停止维护用户服务后，运行安装的维护二进制并指定同一数据目录：

```sh
weighbridge-agent-maintenance --data-dir "$HOME/weighbridge-agent" --reset-password
```

命令轮换密码并在数据目录写入一次性明文文件。管理员应通过本地受控方式读取后删除该文件，再启动维护服务。该恢复依赖现场 OS 账号，不依赖云端登录。

部署前须在备用环境先完成权限与恢复演练。本文档不代表真实工控机的安装、CUPS 管理授权、局域网 HTTP／TLS 或纸面校准已验收。

## 开发验证

`task agent:maintenance:smoke` 使用真实 Chromium 浏览器与生产命令执行器，但通过临时 PATH 注入假 systemd／CUPS 工具，使用独立临时数据目录；结束后清理进程和端口，不操作真实服务。需要既有 `web/node_modules` 中的 Playwright、Python/Pillow/中文字体和 Chromium，可用 `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` 指定缓存浏览器。有私有 IPv4 网卡时同时验证通配绑定下的局域网 HTTP 登录及可选 TLS。沙箱禁止监听端口时，须在允许临时服务的执行环境运行。

## 从后台导入厂区连接配置（2026-10-21）

在后台“基础资料 → 厂区配置”下载目标厂区 JSON，打开本地维护台“配置 → 导入厂区启动配置”，核对 Server 地址、厂区、地磅及 Agent，确认后保存为连接草稿。导入不重启进程，不覆盖已保存的本机 CUPS 队列、打印尺寸或账本；仍需明确“应用配置”，按既有未结束作业检查流程启动。取消预览不保存。文件可重复使用，连接参数沿用既有方式，不增加一次性签发或凭据轮换流程。

`agent.env` 新增可选的厂区及地磅名称／编号展示项，旧文件无需补齐即可兼容运行；本次 `fcc-com-01` 已在保留原连接和打印参数的基础上补齐归属。准确部署与验证结果见[交付记录](../requirements/site-agent-configuration-delivery.md)。
