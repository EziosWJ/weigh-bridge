> **历史档案（2026-10-24 归档）**：以下保留当时的决定、计划或验证事实；其中“当前／待实施／已授权”均指原记录时点。开发与验收使用[当前规格](../../requirements/standalone-site-spec.md)，追溯入口见[历史索引](../README.md)。旧步骤不作为本轮部署或数据操作指令。

# 柯力 D12 仪表与现场 Agent 对接交付记录

日期：2026-10-22。用户授权按[开发指导](../../device/keli-d12-agent-integration-guide.md)直接推进实现、测试、部署和联调。本文区分软件验证、已完成的现场验证、待现场操作项和明确未实现能力；真实重量预览可用不等于完整正式称重闭环已完成。

## 1. 实现范围

### 1.1 独立串口模块

新增 `server/internal/agentscale`：

- `frame.go`：按 18 字节 `tF=05` 固定结构解析 STX、状态字 A/B/C、六位显示重量、六位皮重、CR 和 CKS；保留原始帧、全部状态位、小数位置、毛净类型和文档单位标志。
- `stream.go`：字节流拆包／粘包、垃圾字节、坏帧逐字节重同步和有界缓存；CKS 为二进制字节时仍按固定长度处理。
- `serial_linux.go`：Linux termios raw 模式、8N1、关闭软硬件流控、9600（可配置标准波特率）、`TIOCEXCL` 独占、读超时、关闭时恢复原 termios 并释放描述符；非 Linux 构建明确返回未实现。
- `monitor.go`：串口打开／读取／重连、取消、有界退避、有效帧保鲜、连续错误故障、恢复和状态快照。串口成功打开后必须由新完整有效帧恢复可用；旧稳定值不会因重开、心跳或重复读取复活。
- `status.go`：以 0600 原子写入 `scale-status.json`，供维护服务只读展示。

校验采用本机 868 帧带载验证过的 `CKS = sum(frame[0:16]) & 0xff`，不套用托利多补码算法，也不使用“总是忽略校验”的降级。

### 1.2 数值与单位

- 小数位置取状态字 A 低三位 `d`；源读数按 `raw × 10^(2-d)` 解释。
- 业务重量精确换算为 `0.001 t`：源单位 `t` 时指数为 `5-d`，源单位 `kg` 时指数为 `2-d`；负指数仅在整除时接受，否则返回精度不兼容，不做四舍五入或截断。
- 本机现场确认源单位为 `t`：`1.550` 解码为 `agentproto.Weight(1550)`，即 `"1.55"` 吨。状态字 B bit4 在抓包中均置位、按文档表示 kg，但现场确认实际为吨，因此保留该标志用于诊断，不据此自动换算或再次除以 1000。
- 负值、超载／低于零异常位、净重模式、未知单位、精度不兼容均表达为不可采集；上报 Server 的重量置 0、`stable:false`、`deviceOk:false` 并附原因，不伪造正常值。

### 1.3 稳定、保鲜与故障

- `stable` 只取仪表动态位：B bit3 为 0 时仪表稳定，为 1 时动态；不用连续相同重量推断稳定。
- 默认 1000 ms 无新有效帧即过期；阈值可配置为 200～2999 ms，必须早于 Server 的 3000 ms 采集门限。
- 连续 3 帧校验／结构错误立即故障；单帧坏帧后收到有效帧可恢复。
- 串口打开失败、读取中断、设备忙、重连等待、无有效帧、校验错误、异常位和精度问题均有明确故障文本。
- 旧读数只保留在诊断快照中；过期后 `deviceOk:false`、`stable:false`，不会重复上报旧稳定值刷新 Server 新鲜度。

### 1.4 现有 Agent 集成

- 复用 `server/cmd/agent-printer` 的单一 WSS 连接、身份、密钥、打印账本、配置摘要和 kick 生命周期；没有另起重量 Agent。
- 设备状态默认每 500 ms 上报，健康状态变化即时触发；心跳每 5 s；网络重连只上报当前快照。
- 打印请求进入独立串行工作协程，维护暂停、渲染和 `lp` 提交均不阻塞串口读取、状态上报或 kick。
- 收到 `capture.request` 时以相同 `requestId` 返回 `capture.result`、`success:false`、明确“摄像头未接入，无法取得前后抓拍”的原因，不伪造图片。
- 配置关闭或进程被 kick 时取消串口读取循环并释放端口；原打印和 CUPS 未决作业语义不变。

### 1.5 共享配置与本地维护

`agent.env` 新增字段，旧文件缺少时默认关闭仪表并保持原打印能力：

```dotenv
WEIGHBRIDGE_SERIAL_ENABLED=true
WEIGHBRIDGE_SERIAL_PORT="/dev/ttyS0"
WEIGHBRIDGE_SERIAL_BAUD=9600
WEIGHBRIDGE_SERIAL_PROTOCOL="keli-d12-tf05"
WEIGHBRIDGE_SERIAL_WEIGHT_UNIT="t"
WEIGHBRIDGE_SERIAL_STALE_MS=1000
```

- 字段贯通读取、校验、配置摘要、草稿保存、应用、上一份回退和厂区配置导入；导入厂区配置保留本机串口、打印和校准参数。
- 维护页面“设备”页显示状态、实际重量、源读数、皮重、毛净类型、稳定／动态、最后有效时间、校验状态、连续错误、收发计数和故障原因。
- 维护服务只读 `scale-status.json`，不打开串口；Agent 停止、文件过期或无新有效帧时明确显示“Agent 未运行／状态已过期／无新鲜数据”，不把文件中的旧 `stable:true` 当作当前重量。
- 摄像头仍显示未接入；采集失败不改变正式称重需操作员确认与 Server 校验的规则。

## 2. 软件验证

| 检查 | 结果 |
| --- | --- |
| `task backend:check` | 通过：`go test ./...` 与 `go vet ./...` 全部成功 |
| `task agent:maintenance:smoke` | 通过：真实浏览器与隔离 CUPS／systemd 替身验证维护页、登录、配置应用／回退、导入、队列、校准、过期状态和移动端；设备页新增仪表状态且无编辑输入 |
| 真实抓包解析 | `testdata/d12-tf05-20261022.bin` 共 15,624 字节／868 帧，0 坏帧、0 丢弃；稳定 787、动态 81、非零 409、52 种数值；全部 CKS 通过 |
| 拆包／粘包 | 按 1、2、3、7、8、17、18、19、257 字节切片输入均得到 868 帧；8＋8＋2 分段不提前发帧 |
| 校验与结构 | 868 帧原始数据全通过；单字节篡改 18 个位置全部被拒；坏 CR、非法数字、短帧、坏 CKS 分类明确；CKS 等于 CR 时仍能按固定长度解析 |
| 状态与数值 | 覆盖全部小数位置、正负号、毛净、动态、异常、单位标志、精度不兼容和单位不支持；`1550` 吨制解码为 `"1.55"` |
| 串口生命周期 | Linux PTY 验证 raw 8N1、无校验／流控、9600、VMIN/VTIME、独占打开、关闭恢复 termios、重新打开；伪 reader 验证无新帧过期、重连需新帧、连续坏帧故障、取消关闭端口和有界缓存 |
| 配置兼容 | 旧 `agent.env` 默认禁用串口并保留打印；新字段读写、摘要、校验、`BuildAgentEnvironment` 往返和非法值拒绝通过 |
| 同进程集成 | 打印待处理时仍可更新重量并接受 kick；缺摄像头时 `capture.result` 明确失败且无图片；打印暂停期间不预留打印尝试 |
| 并发检查 | `go test -race ./internal/agentscale ./cmd/agent-printer` 通过，未发现数据竞争 |

## 3. 现场部署与验证

目标主机 `fcc-com-01`（`hp-HP-ProDesk-400-G2-MINI`，Linux x86_64），用户级 systemd 服务。

| 项目 | 实际结果 |
| --- | --- |
| 部署前 | 两个用户服务停止；CUPS 无未结束作业；`/dev/ttyS0` 为 `root:dialout 660` 且当前有 `hp` 临时 ACL |
| 备份 | `/home/hp/weighbridge-agent-backups/d12-20261022-161903/`，含旧二进制、两个 unit、`agent.env`、`maintenance.json`、`maintenance-status.json`、完整 `state/` 和 30 个账本文件逐项 SHA-256 |
| 二进制 | 最终打印 Agent `5ba7c350c9c3a1157a4bb868868b91faa892edb3a389867b1c893ee3b4383f85`；维护服务 `ec2963b4312ff3510c15e3229a4e87153c82c7cd96bb4ed21a059e0f7d1487c5`。首次 D12 构建 `70e21cc3…` 保留在备份目录，供回退对比 |
| 配置 | 保留原身份、密钥、Server、队列和打印校准；仅追加 `WEIGHBRIDGE_SERIAL_*` 六项，身份行逐行比对无变化 |
| 服务 | `weighbridge-agent-printer.service`、`weighbridge-agent-maintenance.service` 均 active；最终构建重新部署后日志显示 `serial=true`，串口重新读取 10 帧后恢复稳定零值 |
| 账本 | 部署后 `state/` 30 个文件 SHA-256 与备份清单完全一致；未发起打印、未重提历史尝试 |
| 零值读取 | `scale-status.json`：`deviceOk:true`、`stable:true`、`weight:"0"`、`unit:"t"`、`checksum:"ok"`、`framesInvalid:0`、原始帧 `02 2d 30 20 30 30 30 30 30 30 30 30 30 30 30 30 0d bf`，与实际 `0.000` 稳定零值一致 |
| Server 状态 | 管理员查询 `/api/weighing/scales/1/device`：`online:true`、`deviceOk:true`、`stable:true`、`weight:"0"`、`unit:"t"` |
| 故障验证 | 停止 Agent 后由本机诊断进程仅打开并独占 `/dev/ttyS0`（不发任何字节），再启动 Agent；Server 收到 `deviceOk:false`、`stable:false`、`weight:"0"`、`error:"串口打开失败：device or resource busy"` |
| 恢复验证 | 释放端口后 Agent 自动重连；Server 恢复 `deviceOk:true`、`stable:true`、`weight:"0"`，本地状态 `fault:""`、`checksum:"ok"` |
| 维护页 | `http://127.0.0.1:18901/` 返回 200；`/api/status` 显示 Agent 运行中、云端已连接、仪表“稳定”、单位 t、校验通过；页面资源包含新的仪表状态渲染 |
| 连续运行 | 约 19 分钟内累计 5,424 个有效帧，`framesInvalid:0`、`discardedBytes:0`、`fault:""`，重量维持真实稳定零值 |

## 4. 待验证项与明确限制

- **真实非零重量现场验证待做**：部署后现场无车辆上磅，Server 观测到的是真实稳定零值；开发测试用本机 868 帧带载样本覆盖 0～1.560 t，但不能替代部署后的非零现场读数。需要现场人员驾驶车辆或放置已知载荷后复核。
- **重启后串口权限待持久化**：当前 `hp` 仍依赖 `/dev/ttyS0` 的临时 ACL。需要现场管理员执行本文第 6 节命令安装 udev ACL 规则并重启验证；不得让 Agent 以 root 运行，也不开放全员写权限。
- **稳定灯逐时对照未做**：抓包与软件均使用仪表动态位，但尚未在本次部署后与面板稳定灯逐时核对。
- **未覆盖模式**：负值、净重、非零皮重、异常位、其他固件／协议、长期运行和真实断线重插仍只有构造／伪终端测试，未在真机覆盖。
- **摄像头未接入**：`capture.request` 明确失败，正式采集闭环仍不完整；真实重量预览可用不等于正式称重完成。
- **打印未做实物验证**：本轮未发起任何打印，未验证纸张、五联或连续走纸；打印账本保持原状。
- 本地验证 Server 使用 SQLite 开发 profile 临时启动，验证结束后已停止；现场 Agent 配置仍指向原 `http://skyrim:8900`，Server 恢复后会自动重连。正式本地部署切换不属于本轮。

## 5. 数据位置

| 内容 | 位置 |
| --- | --- |
| 现场原始抓包 | `tmp/weighbridge-d12-capture-20261022-153008-607c67/`（原始 `raw.bin` 等，未提交） |
| 解析测试样本 | `server/internal/agentscale/testdata/d12-tf05-20261022.bin`，SHA-256 `911c2231b35436751d7258ceda2fe0bcb2d9e684101baf06d42337a4b5f4eff5` |
| 现场仪表状态 | `/home/hp/weighbridge-agent/scale-status.json`（0600，原子替换） |
| 现场连接／配置摘要 | `/home/hp/weighbridge-agent/maintenance-status.json`、`maintenance.json`、`agent.env` |
| 现场维护记录 | `/home/hp/weighbridge-agent/maintenance.jsonl` |
| 现场打印账本 | `/home/hp/weighbridge-agent/state/`（未改变） |
| 部署前备份 | `/home/hp/weighbridge-agent-backups/d12-20261022-161903/` |
| 现场服务日志 | `journalctl --user -u weighbridge-agent-printer.service` |

## 6. 现场必要操作

### 6.1 持久化串口权限（需 sudo）

当前临时 ACL 在重启后可能失效。建议按实际服务账号安装 udev ACL 规则：

```bash
echo 'SUBSYSTEM=="tty", KERNEL=="ttyS0", RUN+="/usr/bin/setfacl -m u:hp:rw /dev/ttyS0"' \
  | sudo tee /etc/udev/rules.d/70-weighbridge-serial.rules
sudo udevadm control --reload-rules
sudo udevadm trigger --sysname-match=ttyS0
getfacl -p /dev/ttyS0 | grep 'user:hp:rw'
sudo systemctl reboot   # 重启后再次确认服务 active 与 getfacl 输出
```

不要用 root 运行整个 Agent，也不要执行 `chmod o+rw /dev/ttyS0`。

### 6.2 非零重量复核（需现场车辆或已知载荷）

车辆上磅并稳定后，在维护页或以下命令核对：

```bash
curl -sS http://127.0.0.1:18901/api/status | python3 -m json.tool
```

确认 `scale.weight`、`scale.stable`、`scale.sourceWeight`、`scale.checksum`、`lastValidAt` 与仪表面板一致；同时由管理员在 Web 工作台核对 Server 设备状态。不要把该预览用于未经操作员确认的正式称重。

## 7. 回退方法

回退前先停止两个用户服务并确认 CUPS 无未结束作业；回退不重发、不删除历史打印尝试。

```bash
BK=/home/hp/weighbridge-agent-backups/d12-20261022-161903
systemctl --user stop weighbridge-agent-printer.service weighbridge-agent-maintenance.service
cd /home/hp/weighbridge-agent
cp -a "$BK/weighbridge-agent-printer" "$BK/weighbridge-agent-maintenance" bin/
cp -a "$BK/agent.env" "$BK/maintenance.json" ./
# 可选：恢复 unit（本轮未修改，保留作核对）
# cp -a "$BK"/*.service /home/hp/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user start weighbridge-agent-maintenance.service
systemctl --user start weighbridge-agent-printer.service
systemctl --user is-active weighbridge-agent-printer.service weighbridge-agent-maintenance.service
```

旧版本忽略新增的 `WEIGHBRIDGE_SERIAL_*` 环境变量；如需彻底清理可删除这些行。`state/` 目录无需回退，除非账本本身受损；本轮确认账本前后一致。回退后维护页“设备”恢复未接入占位，打印能力保持。
