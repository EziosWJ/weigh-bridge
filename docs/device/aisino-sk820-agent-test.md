# SK-820 最小打印 Agent 现场联调

日期：2026-10-19。范围：真实打印提交链路及 A4 单张版面。用户已确认最终 175 mm 版面可用；仪表和摄像头未接入，五联复写和连续走纸未验收。

## 程序与状态边界

`server/cmd/agent-printer` 使用现有 Agent v1 协议连接 Server，从 `ticket.printSheet` 读取精简纸单。只支持 `weighing-compact-v1`、210 × 99 mm，首版使用 GB18030 文字，现场反馈缺少表格后改为 Pillow＋Noto Sans CJK 将文字与实线表格统一渲染为 180 dpi 黑白点阵，再用已验证的 ESC/P `ESC * 39`（24 点分带，每次最多 128 列）输出，通过 `lp -d Aisino-SK820-raw -o raw -n 1` 提交一份。五联表示复写层数，不生成五份任务。

表格画布 1240 × 600 点（约 175 × 84.7 mm），五行字段按各行单元格数量等分，长字段最多折为两行，超出则拒绝截断。控制字符替换为空格，纸面包含外框及行列实线。初始化指定 99 mm 表长、单向图形打印、24/180 英寸分带走纸；打印机对表长指令的支持、实际边距和五联效果必须由现场纸面验证。

任务按打印尝试编号创建独占文件并同步落盘后才调用 `lp`；重连、重启及重复请求不重提旧尝试。提交前后的崩溃或 `lp` 错误均保留该编号，需人工核查，不能删除记录后盲目重打。CUPS 作业信息保存在同目录的 `.cups` 文件中。

此版本不能可靠判定纸面成功，不自动发送 `printed`，也不把无法确定的提交错误发送为明确未出纸的 `failed`。系统沿用待核实与人工确认流程。提交结果不确定时必须核查 CUPS 剩余任务，设备恢复可能使旧作业继续出纸。

Agent 每秒上报零重量、`stable=false`、`deviceOk=false` 及未接入说明，保持连接状态但不能用于称重确认。同一身份的 mock 与真实 Agent 不能同时运行，新连接会替换旧连接。不得为了打印恢复而伪造可称重状态。

## 部署

构建：`task agent:printer:build`，产物 `bin/weighbridge-agent-printer`，Linux amd64、无 CGO。目标主机不需要安装 Go；运行时需要 Python 3、Pillow、`/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc`（可用环境变量 `WEIGHBRIDGE_PRINT_FONT` 指定其他完整中文字体）。本次目标主机已具备这些依赖，没有执行安装。

HP 主机文件：

- 程序：`~/weighbridge-agent/bin/weighbridge-agent-printer`
- 密钥：`~/weighbridge-agent/agent.env`，权限 600，不纳入仓库
- 尝试记录：`~/weighbridge-agent/state/`，目录权限 700
- 用户服务：`~/.config/systemd/user/weighbridge-agent-printer.service`，仓库[模板](../../agent/deploy/weighbridge-agent-printer.service)使用本次测试隧道地址

用户服务通过 `flock` 防止重复启动，已启用 `enabled`；`loginctl show-user hp -p Linger` 为 `yes`。CUPS 保持既有已启动、开机自启配置。

本次 Server 是开发电脑上的 `127.0.0.1:8900`。通过 SSH 加密反向隧道映射至 HP 的 `127.0.0.1:18900`，Agent 连接 `http://127.0.0.1:18900`。不开放公网 CUPS。

隧道由开发电脑维护，**不是正式部署的永久 Server 地址**，开发电脑重启、隧道退出或断网均会中断接入；Agent 会重连但不会重打。正式部署需要改为稳定可达的 HTTPS/WSS Server。

本次隧道控制 socket：开发电脑 `/tmp/weighbridge-printer-tunnel`。检查：

```bash
ssh -F ~/.ssh/config -S /tmp/weighbridge-printer-tunnel -O check weighbridge-hp
```

需要恢复隧道时，先确认旧隧道及远端端口状态，再运行：

```bash
ssh -F ~/.ssh/config -o BatchMode=yes -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=15 -o ServerAliveCountMax=3 \
  -M -S /tmp/weighbridge-printer-tunnel -fNT \
  -R 127.0.0.1:18900:127.0.0.1:8900 weighbridge-hp
```

远端检查与停止：

```bash
systemctl --user status weighbridge-agent-printer
journalctl --user -u weighbridge-agent-printer -n 30 --no-pager
lpstat -p -a
lpstat -o
# 需要停止现场接入时
systemctl --user stop weighbridge-agent-printer
```

## 本次验证

- `task backend:check`：Go 测试、vet 通过；新增快照字段及控制字符清理、字段超限拒绝、提交成功和提交错误后的重复拦截测试。
- `python3 -m unittest discover -s server/internal/agentprinter -p '*_test.py'`：4 项通过，覆盖实线表格边框及分隔线、单元格溢出拒绝、ESC/P 数据逐点解码与图像一致性，以及实际图形宽度不超过 175 mm。
- HP 用户服务启动成功，Server API 可见 Agent 在线，仪表状态保持不可称重。
- 使用已有有效单据 `GB202610190001`（晋A65432），最终毛重 50、皮重 20、净重 30 吨，通过现有鉴权业务 API 补打，保留单号、原因和次数。
- Agent 重启后没有重提旧任务，尝试记录仍保留；目标主机服务及 CUPS 持续运行。

| 打印尝试 | CUPS 作业 | 现场反馈与处理 |
| --- | --- | --- |
| 3 | `Aisino-SK820-raw-4` | 仅文字，缺少表格；据实记录已出纸及版面问题，改为文字与实线表格统一点阵输出 |
| 4 | `Aisino-SK820-raw-5` | 有清晰表格，但右侧超纸；照片及现场确认使用 A4 竖向进纸，将图形宽度从约 197.6 mm 收窄至约 175 mm |
| 5 | `Aisino-SK820-raw-6` | 纸单标记“补打第4次”，用户确认“这一版可以了”，A4 单张版面通过现场确认 |

最终作业 95232 字节，22:25:47 提交，22:26:21 CUPS 完成、队列恢复空闲。现场确认后才将系统尝试 5 核实为已打印；此前不因 CUPS 完成而自动认定纸面合格。

## 本次经验

1. **把纸张尺寸和可打印尺寸分别确定。** 模板 210 × 99 mm 是纸单设计尺寸，不是可以从打印起点开始铺满的宽度。A4 竖向进纸纸宽 210 mm，实际打印起点、进纸位置和边距需留余量；本机约 198 mm 图形实际超纸，175 mm 图形经现场确认可用。该值是当前设备和装纸方式的验证值，更换纸型或设备应重新校准，不保证对其他设备通用。
2. **有字段不等于有表格。** 纯 GB18030 文本可以打印中文，但不能自动产生票面边框。将表格线与文字一同渲染为黑白点阵，能固定单元格位置、换行和边框关系；每个字段最多两行，超限拒绝截断。物理位置仍以实际出纸为准。
3. **选同时覆盖中文、英文、数字和标点的字体。** 本地预览曾发现只用 DroidSansFallback 会缺少部分西文字符；最终使用 NotoSansCJK-Regular.ttc。部署前既检查文件存在，也用包含中文、单号、重量、日期及签字线的真实样例预览，不能只检查中文能否显示。
4. **分别检查接收任务、设备输出和纸面效果。** `lp` 返回作业号只代表提交，CUPS 完成也不能证明纸面清晰、未越界。本版依靠操作员现场核实。约 95 KB 的图形作业本次处理约 34 秒，不能因比纯文字慢就立即补发；先查队列和实际出纸，结果未知不自动重打。
5. **每次复验有业务留痕，只改一个关键变量。** 先补表格，再收窄宽度；已出纸的失败版面也记录为实际出纸并注明问题，后续按补打流程记录原因和次数。任务先落盘再提交，重启或重复消息不自动再次出纸。提交不确定时先查旧 CUPS 作业，设备恢复可能继续执行旧作业。
6. **在线不等于可以称重，服务自启不等于链路永久可用。** 仅打印 Agent 持续上报未接入状态，不能假装仪表正常；同身份 mock 必须退出。用户服务和 CUPS 已自启，但本次连接仍依赖开发电脑维护的 SSH 隧道，正式部署需稳定的 WSS 地址。
7. **把物理缺陷固化为必要检查。** 新增宽度断言先复现失败（197.6 > 175），收窄后通过；同时检查表格边框、字段溢出和 ESC/P 数据逐点一致性。这些检查帮助防止软件回归，仍不能替代换纸后的现场验收。

## 后续验收边界

本次通过的是 A4 单张纸上的真实打印链路和当前样例版面。五联纸第五联清晰度、210 × 99 mm 连续纸首张定位、连续走纸与累积偏移，仍需取得实际纸张后单独验收；不能将本次 A4 结果描述为五联票据已完成验收。仪表、摄像头及完整真实称重接入仍未实现。
