# 串口扫码枪 Agent

`serial-agent` 是厂端部署机上的纯后台进程，用于把串口扫码枪读取到的手机二维码交给厂端称重业务。它没有界面、不登录用户账号、不访问数据库。

## 工作方式

```text
手机二维码 fc-handoff:...
        ↓ 扫码枪串口
serial-agent（本机后台）
        ↓ 本机 Unix socket
厂端 API /api/internal/weighing/mobile-handoffs/scan
        ↓ 复用现有称重流程
首磅任务 / 回磅正式磅单 / 云端同步
```

- 扫码枪参数在厂端 Web「本地管理 → 设备维护 → 扫码枪管理」中配置，保存在数据库。
- Agent 每 4 秒心跳并从厂端 API 获取最新配置版本；配置变更后自动重新打开串口。
- 串口结束符默认按 `\r\n` 组帧，可按扫码枪实际输出修改。
- 网络超时或 5xx 会按配置重试；二维码过期、已使用、厂区不匹配等业务拒绝不重试。
- 操作审计使用系统身份 `ActorID=0 / 串口扫码Agent`，不会创建可登录账号。
- 内部接口只监听本机 Unix socket，socket 文件权限为 `0600`。

## 构建

```bash
cd server
go build -trimpath -o ../bin/weighbridge-serial-agent ./cmd/serial-agent
```

## 运行

```bash
../bin/weighbridge-serial-agent \
  -socket /tmp/weighbridge-serial-agent.sock \
  -id site01-serial-agent
```

参数：

| 参数 | 说明 |
| --- | --- |
| `-socket` | 厂端 API 暴露的本机 Unix socket，默认 `/tmp/weighbridge-serial-agent.sock` |
| `-id` | Agent 标识，显示在设备维护页面 |
| `-stdin` | 从标准输入读取二维码，仅用于本地联调 |
| `-debug` | 输出调试日志 |

厂端 API 的 socket 路径由 `configs/config.{env}.yaml` 配置：

```yaml
serial_agent:
  socket_path: /tmp/weighbridge-serial-agent.sock
```

`socket_path` 留空可关闭内部监听。

## systemd 示例

```ini
[Unit]
Description=Fenchichun serial QR scanner agent
After=network.target

[Service]
Type=simple
User=weighbridge
ExecStart=/opt/weighbridge/bin/weighbridge-serial-agent -socket /run/weighbridge/serial-agent.sock -id site01-serial-agent
Restart=always
RestartSec=3
# 串口权限：将运行用户加入 dialout 组，或按现场 udev 规则授权。
SupplementaryGroups=dialout
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

如果使用 `/run/weighbridge/serial-agent.sock`，请确保厂端 API 与 Agent 使用同一路径，并且运行用户对目录有写权限。

## 本地联调

没有真实扫码枪时，可以用 `-stdin` 模拟扫码：

```bash
printf 'fc-handoff:xxxxx\r\n' | \
  ./bin/weighbridge-serial-agent -stdin -socket /tmp/weighbridge-serial-agent.sock
```

也可以用伪终端（pty）模拟串口；Agent 使用标准 Linux termios 配置，支持 `/dev/ttyUSB0`、`/dev/ttyS*` 和 `/dev/pts/*`。
