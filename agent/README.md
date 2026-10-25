# 现场 Agent 与开发 mock

> **版本范围**：ADR 0030 起 Agent 只需 Server 地址，相机与视频服务归 Server，串口与打印由 Agent 维护页管理。协议与所用二进制的完成程度以[当前状态](../docs/next.md)及实际交付核对。安装、组件选择与升级迁移见[本厂部署](../docs/device/local-deployment.md)。

真实 `agent-printer` 共用一条连接提供 D12 仪表、测量有效性核实与 SK-820 打印；相机、可选识别及视频由 Server 管理。原始帧、单位核验见[D12 参考](../docs/device/keli-d12-agent-integration-guide.md)，软件及现场完成程度统一见当前状态。开发 mock 替代整个 Agent，不能与同磅真实 Agent 同时在线。

## 本地启动真实 Agent

先启动本地 API，在网页「系统管理 → 配置管理」完成本厂身份配置。Agent 连接文件只需 Server 地址，不连接业务数据库：

```bash
task init
task dev
# Server 首次启动自动建立本厂身份，登录后在配置管理确认过磅单标题
cd server
go run ./cmd/agent-provision --agent-env-path "$PWD/../agent.env" --server-url http://127.0.0.1:8900
cd ..
# 可在维护页配置串口和打印，或编辑根目录 agent.env 后启动真实 Agent
task agent -- --env-file "$PWD/agent.env"
```

`agent-provision` 只写入 `WEIGHBRIDGE_SERVER_URL`（以及新文件的队列／串口默认值），权限 0600，不打印敏感内容。连接地址从 Agent 主机视角填写：同机可用回环地址，分机须为 Server 的局域网 IP 或域名。首次生成必须提供有效地址；更新已有文件时显式加 `--update-connection`，未传地址则保留已有有效值。更新会清理旧密钥、人工身份和相机字段，保留串口、打印校准、未知配置行及打印账本；旧相机源地址保存在同目录 `agent.env.legacy-camera.json`，请在 Server 摄像头页重新填写。`--config-dir` 仅为旧调用方兼容而保留，工具不再读取数据库。不要把真实 `agent.env` 提交到仓库。

`task agent` 自动编译并运行真实 `agent-printer`，默认读取根目录 `agent.env`；也可显式传入 `--env-file`。开发状态及打印账本默认保存在忽略的 `.task/agent/state`。修改配置后重启任务。真实 Agent 和同一地磅的 mock 不能同时连接。

可透传现有启动参数覆盖配置和状态目录，建议使用绝对路径；已有账本继续指定原状态目录：

```bash
task agent -- --env-file /绝对路径/agent.env --state-dir /绝对路径/agent-state
```

使用配置文件时，连接和队列参数写入文件，不与 `--server` 等兼容参数混用。

## 真实 Agent 的仪表输入模拟

日常仪表联调可运行 `task device:d12`，通过 <http://127.0.0.1:8091> 操控 D12 重量、动态位、停发和坏帧，把显示的 `/dev/pts/...` 写入根目录 `agent.env`，再在另一终端运行 `task agent`。无需新增 Agent 身份，也不与真实 Agent 抢占地磅连接。启动、配置和故障语义见 [D12 模拟器说明](../docs/device/d12-simulator.md)，软件验证见[交付记录](../docs/history/requirements/d12-simulator-delivery.md)。

摄像头输入由 Server 直接读取 RTSP，开发可参考[模拟源说明](../docs/device/camera-simulation-dev-memo.md)；Agent 不再配置或转流相机。仪表在 Agent 的设备输入端模拟；完整采集与业务闭环需另行联调。`agent-mock` 是轻量业务联调与自动化测试入口，同磅不能与真实 Agent 同时连接。

## mock 本机启动

先启动 Server，确认内部身份已自动建立，再按上文生成 `agent.env`。mock 可读取显式指定的 Agent 环境文件，或用 `--server` 指定地址；不从业务 API YAML 导入身份，不需要配对文件。

```bash
# 终端一：API 与系统 Web
task dev
# 终端二：独立 mock 控制台
task agent:mock -- --env-file "$PWD/agent.env"
```

`--config` 可继续读取便携 JSON 连接配置；它与 `--env-file` 互斥，且二者都不能与 `--server`、`--agent-id`、`--scale-id` 混用。ADR 0030 起只需 Server 地址，mock 通过 `agent.hello` 接收 Server 分配的地磅；`--agent-id`、`--scale-id` 与旧 key 仅供排障，不再是接入门槛。`agent.env` 中的旧身份、密钥与相机字段会在升级时清理，不影响 mock 从该文件读取连接地址。

本地 API 默认端口 8900，mock 控制页默认监听 `0.0.0.0:8090`；本机访问 http://127.0.0.1:8090。也可运行 `task agent:mock -- --server http://127.0.0.1:8900`，不与 `--env-file` 混用；旧编号和 key 兼容参数不成为接入门槛。Agent 离线时网页明确显示实际配置不可读，不在 API 主机伪造设备文件。

## mock 操作与当前采集

1. 设置目标吨重并模拟上磅，观察动态转稳定；可强制失稳、故障、断连或下磅，验证测量依据失效。
2. 发送识别候选或模拟失败，由操作员核对；候选不自动绑定任务或提交称重。
3. 在工作台采集并人工确认。v2 图片由 Server 的相机列表提供；需要图像时在 Server 配置可控模拟源，关闭强制抓拍时也仍需有效仪表依据。
4. 发起打印后 mock 展示任务，可反馈成功、失败或不回报以保留未知。

mock 控制页仍保留 front／rear 示例图上传及 `camera-disabled`、`camera-failure`、`upload-failure`、`camera-ready` 控制，供 v1 兼容链路测试；这些动作不改变 v2 Server 摄像头配置或抓拍策略。开发控制页用于受信环境，模拟不替代现场验收；重连不补办称重或重打。

`task agent:build` 构建 mock；真实打印／仪表 Agent 使用 `task agent:printer:build`。host-collector 是另一个可选宿主机监控工具。

## 证据保留

Server 将抓拍保存于 `weighing.image_dir`，默认 `server/data/agent-images`（相对 Server 工作目录）。该目录已忽略；备份、迁移和恢复需与业务数据库一起保留。历史内置测试图片继续用于读取旧快照，不参与新采集。

## 仪表与打印维护

日常使用业务系统设备维护页，串口及打印一次“保存并应用”，CUPS 队列从 Agent 主机读取，成功参数在 Server 留恢复副本。主进程在线而设备执行停止时仍可维护；整个 Agent 离线时使用其宿主机 systemd／日志恢复，详见[维护说明](../docs/device/agent-remote-maintenance.md)。旧独立维护服务仅用于已安装旧部署的过渡排障，资料见[本机恢复](../docs/device/agent-local-maintenance.md)。

串口示例：

```dotenv
WEIGHBRIDGE_SERVER_URL="http://127.0.0.1:8900"
WEIGHBRIDGE_SERIAL_PORT="/dev/ttyS0"
WEIGHBRIDGE_SERIAL_BAUD=9600
```

D12 协议、吨制及 1 秒有效帧阈值固定；未配置串口仍保持维护连接。按 Agent 实际服务账号授予持久串口权限并验证重启，不以 root 运行整个 Agent。

## 五联三等分连续纸

小数毫米表长、统一缩放及校准样张的参数含义与现场验收顺序见[SK-820 连续纸校准](../docs/device/sk820-continuous-paper-calibration.md)。本次软件实现与验证记录见[交付记录](../docs/history/requirements/continuous-paper-print-delivery.md)；真实走纸、撕纸停靠和第五联清晰度仍需现场验收。

## 抓拍策略联调

在配置管理切换“称重强制抓拍”，规则见[业务规格](../docs/requirements/weighing-business-spec.md#4-抓拍策略)。v2 的失败、空集合及部分图片场景通过 Server 相机源模拟，mock 继续负责有效仪表条件；不能用 mock 旧相机控制代替 Server 取图失败测试。
