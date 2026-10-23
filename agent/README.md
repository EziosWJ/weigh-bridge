# 现场 Agent 与开发 mock

> 当前目标：Agent 作为每厂本地系统的内部设备进程，连接本厂业务服务；维护入口和账号将统一，独立维护服务仅作过渡。自治软件已实现统一维护入口与发布包，现场整合尚待验收；新部署见[三服务安装说明](../docs/device/standalone-deployment.md)，下文命令保留已有实现用途，详见[当前规格](../docs/requirements/standalone-site-spec.md)与 [ADR 0023](../docs/adr/0023-standalone-site-baseline.md)。

现场 Linux Agent 已具备 SK-820 打印与柯力 D12 tF=05 重量采集：同一 `agent-printer` 进程共用一个 WSS 连接和地磅身份，串口读取与打印互不阻塞。2026-10-22 已在 `fcc-com-01` 部署并验证稳定零值、串口占用故障及恢复；真实非零带载、面板稳定灯对照、重启后串口权限及五联／连续走纸仍待现场验收。已新增 HTTP(S)/RTSP 前后抓拍与可选 HTTP 识别适配通道，现场型号、参数及实际识别引擎尚未确认或验收。未配置摄像头或无法取得新鲜证据时明确失败，不会伪造图片或采集成功。见[交付记录](../docs/requirements/d12-agent-integration-delivery.md)与[打印部署记录](../docs/device/aisino-sk820-agent-test.md)。开发和上线前测试使用独立 agent-mock，通过[同一通信契约](../protocol/v1.md)连接 Server。

D12 的现场依据、单位差异与验证清单见[RS232 现场实验记录／博客稿](../docs/device/keli-d12-rs232-blog.md)与[Agent 接入开发指导](../docs/device/keli-d12-agent-integration-guide.md)。驱动实现位于 `server/internal/agentscale`。本机 `tF=05` 共 868 帧带载样本验证了累加校验；现场确认数值单位为吨，因此显式配置源单位 `t`，不根据文档 kg 标志再次除以 1000。单位结论仅适用于已核对的现场仪表。

## 本地启动真实 Agent

首次使用可复制[开发配置示例](agent.env.example)，填写本地业务服务地址、匹配的身份／密钥、实际串口路径及需要的设备参数：

```bash
# 仅首次创建；不覆盖已有配置
cp -n agent/agent.env.example agent.env
# 编辑根目录 agent.env 后启动
task agent
```

`task agent` 自动编译并运行真实 `agent-printer`，显式读取仓库根目录 `agent.env`，开发状态及打印账本默认保存在忽略的 `.task/agent/state`。修改配置后重启任务。根目录 `agent.env` 已被 Git 忽略，示例中的密钥占位符须替换成本地业务服务的实际 Agent 密钥。

可透传现有启动参数覆盖配置和状态目录，建议使用绝对路径；已有账本继续指定原状态目录：

```bash
task agent -- --env-file /绝对路径/agent.env --state-dir /绝对路径/agent-state
```

使用配置文件时，连接和队列参数写入文件，不与 `--server` 等兼容参数混用。

## 真实 Agent 的仪表输入模拟

日常仪表联调可运行 `task device:d12`，通过 <http://127.0.0.1:8091> 操控 D12 重量、动态位、停发和坏帧，把显示的 `/dev/pts/...` 写入根目录 `agent.env`，再在另一终端运行 `task agent`。无需新增 Agent 身份，也不与真实 Agent 抢占地磅连接。启动、配置和故障语义见 [D12 模拟器说明](../docs/device/d12-simulator.md)，软件验证见[交付记录](../docs/requirements/d12-simulator-delivery.md)。

摄像头输入可使用[两路模拟 RTSP](../docs/device/camera-simulation-dev-memo.md)。仪表与摄像头均在 Agent 的设备输入端模拟；完整 Agent 采集与业务闭环需另行联调。下文 `agent-mock` 保留为轻量业务联调与自动化测试入口，同磅不能与真实 Agent 同时连接。

## mock 本机启动

先运行 `task init`。初始化会在忽略的本地配置中生成独立随机 Agent 密钥，API 与 mock 自动读取同一份配置。已有开发配置也可重新运行 `task init` 补齐缺失密钥，已有 JWT、数据库设置及已配置密钥不会改写。补齐后重启 API。

```bash
# 初始化或补齐已有本地配置
task init
# 终端一：API 与系统 Web
task dev
# 终端二：独立 mock
task agent:mock
```

也可用 `APP_WEIGHING__AGENT_KEY` 覆盖本地密钥；覆盖时 API 与 mock 必须使用同一值。独立可执行文件没有本地配置时需要此环境变量。真实密钥不得提交。

本地测试 API 默认监听 `0.0.0.0:8900`，mock 控制台默认监听 `0.0.0.0:8090`；本机访问 http://127.0.0.1:8090，局域网访问 `http://<本机局域网 IP>:8090`。默认连接地址跟随本地 API 的 `http.address` 配置，例如 API 为 `0.0.0.0:8900`，mock 自动连接 http://127.0.0.1:8900；通配监听地址会转换为本机回环地址。启动日志显示实际连接目标。Agent 编号默认 `scale-agent-1`，地磅 ID 默认 `1`。Server 与 mock 的编号、地磅和密钥必须一致。远程地址仍可显式覆盖：`task agent:mock -- --server https://your-server.example --listen 0.0.0.0:8090`。独立程序没有可用 Server 配置时回退到 http://127.0.0.1:8080。

## 页面操作

1. 设置目标吨重，点击“模拟上磅”；系统可看到连续 4 秒的不稳定波动，再转为稳定。
2. 发送识别候选，或模拟识别失败；系统由操作员核对车牌。
3. 默认有车头、车尾示例图，也可上传 PNG/JPEG 替换；采集时取得独立图片引用。
4. 在系统工作台采集并人工确认。mock 可强制失稳、故障、断连或下磅，验证系统的失效处理。
5. 在系统发起打印，mock 展示任务内容，可回报成功、失败或暂不回报以保留未知结果。

控制台只服务于开发测试，默认监听所有 IPv4 网卡。无需 SQLite、本地业务库或设备驱动；重连只发送当前状态，不补办旧称重、不自动重打。真实设备以后实现同一协议，不需要将模拟代码放回系统。

`task agent:build` 生成独立可执行文件。实现沿用 Server 的 Go module 和既有 WebSocket 依赖，页面及示例图嵌入该程序。Server host-collector 仍只负责宿主机监控，与现场 Agent 无关。

## 证据保留

Server 将抓拍保存于 `weighing.image_dir`，默认 `server/data/agent-images`（相对 Server 工作目录）。该目录已忽略；备份、迁移和恢复需与业务数据库一起保留。历史内置测试图片继续用于读取旧快照，不参与新采集。

## 本地打印维护

SK-820 打印 Agent 的离线维护页面是独立进程，即使打印 Agent 无法启动仍可使用。维护服务默认监听 `0.0.0.0:18901`，资源随 Go 二进制嵌入；可信维护局域网可通过 HTTP 访问，TLS 可选，防火墙须限制来源且不得开放公网。打印 Agent 的受维护运行配置统一从 `agent.env` 读取，旧 unit／drop-in 的连接参数需按部署说明先迁入文件。页面通过顶部菜单提供总览、打印与队列、设备、配置和记录；设备页显示 Agent 写入的实时重量、单位、稳定状态、最后有效时间、校验与故障原因，维护服务只读状态文件、不打开串口。摄像头显示配置状态，真实连接在称重采集时确认。构建入口为 `task agent:maintenance:build`。部署、CUPS 最小授权、维护密码首次设置／恢复、队列操作边界和未现场验证项见[本地维护部署说明](../docs/device/agent-local-maintenance.md)。串口字段示例：

```dotenv
WEIGHBRIDGE_SERIAL_ENABLED=true
WEIGHBRIDGE_SERIAL_PORT="/dev/ttyS0"
WEIGHBRIDGE_SERIAL_BAUD=9600
WEIGHBRIDGE_SERIAL_PROTOCOL="keli-d12-tf05"
WEIGHBRIDGE_SERIAL_WEIGHT_UNIT="t"
WEIGHBRIDGE_SERIAL_STALE_MS=1000
```

旧 `agent.env` 没有这些字段时按仅打印运行。现场串口权限须按实际服务账号持久配置，不能以 root 运行 Agent。

## 五联三等分连续纸

小数毫米表长、统一缩放及校准样张的参数含义与现场验收顺序见[SK-820 连续纸校准](../docs/device/sk820-continuous-paper-calibration.md)。本次软件实现与验证记录见[交付记录](../docs/requirements/continuous-paper-print-delivery.md)；真实走纸、撕纸停靠和第五联清晰度仍需现场验收。

### 可选抓拍联调（2026-10-23）

业务配置管理的“称重强制抓拍”默认开启，关闭后的正式称重规则见 [功能规格](../docs/requirements/optional-weighing-capture-spec.md)。真实 Agent 也须升级至支持 `captureRequired` 的版本；未施工厂区可关闭设备摄像头及业务强制抓拍，重量和人工确认仍必需。

独立 mock 的 `POST /api/control` 新增动作 `camera-disabled`、`camera-failure`、`upload-failure` 和 `camera-ready`，分别模拟无摄像头、抓拍失败、上传失败和恢复抓拍；仅影响下一次采集返回，不修改业务策略。业务强制时图片异常仍失败，关闭时保留相应状态。
