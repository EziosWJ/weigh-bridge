# 现场 Agent 与开发 mock

现场 Linux Agent 已具备 SK-820 打印与柯力 D12 tF=05 重量采集：同一 `agent-printer` 进程共用一个 WSS 连接和地磅身份，串口读取与打印互不阻塞。2026-10-22 已在 `fcc-com-01` 部署并验证稳定零值、串口占用故障及恢复；真实非零带载、面板稳定灯对照、重启后串口权限及五联／连续走纸仍待现场验收。RTSP/HyperLPR3 识别与摄像头尚未实现，收到采集请求会明确失败，不会伪造图片或采集成功。见[交付记录](../docs/requirements/d12-agent-integration-delivery.md)与[打印部署记录](../docs/device/aisino-sk820-agent-test.md)。开发和上线前测试使用独立 agent-mock，通过[同一通信契约](../protocol/v1.md)连接 Server。

D12 的现场依据、单位差异与验证清单见[RS232 现场实验记录／博客稿](../docs/device/keli-d12-rs232-blog.md)与[Agent 接入开发指导](../docs/device/keli-d12-agent-integration-guide.md)。驱动实现位于 `server/internal/agentscale`。本机 `tF=05` 共 868 帧带载样本验证了累加校验；现场确认数值单位为吨，因此显式配置源单位 `t`，不根据文档 kg 标志再次除以 1000。单位结论仅适用于已核对的现场仪表。

## 本机启动

先运行 `task init -- --database sqlite`。初始化会在忽略的本地配置中生成独立随机 Agent 密钥，API 与 mock 自动读取同一份配置。已有开发配置也可重新运行 `task init` 补齐缺失密钥，已有 JWT、数据库设置及已配置密钥不会改写。补齐后重启 API。

```bash
# 初始化或补齐已有本地配置
task init -- --database sqlite
# 终端一：API 与系统 Web
task dev:sqlite
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

SK-820 打印 Agent 的离线维护页面是独立进程，即使打印 Agent 无法启动仍可使用。维护服务默认监听 `0.0.0.0:18901`，资源随 Go 二进制嵌入；可信维护局域网可通过 HTTP 访问，TLS 可选，防火墙须限制来源且不得开放公网。打印 Agent 的受维护运行配置统一从 `agent.env` 读取，旧 unit／drop-in 的连接参数需按部署说明先迁入文件。页面通过顶部菜单提供总览、打印与队列、设备、配置和记录；设备页显示 Agent 写入的实时重量、单位、稳定状态、最后有效时间、校验与故障原因，维护服务只读状态文件、不打开串口。摄像头仍显示未接入。构建入口为 `task agent:maintenance:build`。部署、CUPS 最小授权、维护密码首次设置／恢复、队列操作边界和未现场验证项见[本地维护部署说明](../docs/device/agent-local-maintenance.md)。串口字段示例：

```dotenv
WEIGHBRIDGE_SERIAL_ENABLED=true
WEIGHBRIDGE_SERIAL_PORT="/dev/ttyS0"
WEIGHBRIDGE_SERIAL_BAUD=9600
WEIGHBRIDGE_SERIAL_PROTOCOL="keli-d12-tf05"
WEIGHBRIDGE_SERIAL_WEIGHT_UNIT="t"
WEIGHBRIDGE_SERIAL_STALE_MS=1000
```

旧 `agent.env` 没有这些字段时按仅打印运行。现场串口权限须按实际服务账号持久配置，不能以 root 运行 Agent。
