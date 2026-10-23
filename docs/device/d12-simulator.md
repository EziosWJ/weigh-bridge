# D12 虚拟串口模拟器

2026-10-23。独立开发工具，为真实 Agent 提供现场已验证的柯力 D12 `tF=05` 字节流；不连接业务服务、不占用 Agent 身份。本次只实现仪表输入模拟，未执行摄像头接入、完整称重或实物打印验收。

## 启动与接入

要求 Linux／WSL，模拟器和 Agent 在同机且可见同一 `/dev/pts`。沿用项目 Go 工具链，无需 socat、Docker、数据库或 JWT。

```bash
task device:d12
```

默认控制页：<http://127.0.0.1:8091>。启动日志及页面显示实际虚拟串口，例如 `/dev/pts/3`。启动后按 200 ms 周期发送稳定零值；读取端未打开时丢弃报文，不积压后再发送。

把页面列出的串口字段合入开发 Agent **实际使用的** `agent.env`，其余身份、连接、摄像头及打印参数继续沿用原配置：

```dotenv
WEIGHBRIDGE_SERIAL_ENABLED=true
WEIGHBRIDGE_SERIAL_PORT="/dev/pts/3"
WEIGHBRIDGE_SERIAL_BAUD=9600
WEIGHBRIDGE_SERIAL_PROTOCOL="keli-d12-tf05"
WEIGHBRIDGE_SERIAL_WEIGHT_UNIT="t"
WEIGHBRIDGE_SERIAL_STALE_MS=1000
```

路径须替换成这次启动的实际值。通过既有维护入口应用配置，或以原参数和 `--env-file` 重启开发 Agent。仅在 shell 中导出串口参数不等于 Agent 已读取它们。建议模拟器与 Agent 使用同一系统用户；跨用户时须具备该 PTY 的读写权限，模拟器不会更改宿主机权限。

本机开发可在仓库根目录准备 `agent.env`，填写完整连接配置及上述串口字段，再在另一终端运行 `task agent`，自动编译并启动真实 Agent。首次配置示例、默认开发状态目录及自定义文件／账本路径见 [Agent 启动说明](../../agent/README.md#本地启动真实-agent)。

真实 Agent 与同磅 `agent-mock` 仍互斥；D12 模拟器没有 Agent 连接，应与真实 Agent 同时运行。业务服务和维护页不会另开串口。

## 控制行为

| 页面操作 | HTTP action | 行为 |
| --- | --- | --- |
| 设置稳定重量 | `set_weight` | 立即设置重量和稳定状态，取消上磅波动 |
| 模拟上磅 | `up` | 正目标重量，波动约 4 秒后稳定并保持；不自动下磅 |
| 下磅归零 | `down` | 取消波动，设置稳定零值 |
| 强制失稳 | `unstable` | 保持当前重量，仅置动态位；取消自动稳定 |
| 恢复稳定 | `stable` | 清动态位；若正在上磅则立即到达目标重量 |
| 暂停发送 | `pause` | 保留重量、状态和上磅进度，不发送报文，不销毁 PTY |
| 恢复发送 | `resume` | 从保留状态继续发送，上磅过程继续剩余时间 |
| 发送一个坏帧 | `bad_once` | 下一次实际发送仅破坏 CKS；发送成功后自动恢复正常校验 |
| 连续坏帧 | `bad_start` | 持续破坏 CKS，清除待发送的单个坏帧 |
| 恢复正常校验 | `bad_stop` | 取消连续坏帧和待发送的单个坏帧，不解除暂停或改变重量 |

重量字符串单位为吨，范围 `0`～`999.999`，最多三位小数；不接受负值、指数或需要四舍五入的值。只有 `set_weight` 和 `up` 接受 `weight`。上磅目标须大于零。

重量、暂停、校验故障分别控制：例如下磅不会解除暂停或连续坏帧。暂停或连续坏帧期间不能安排单个坏帧；读取端未打开时，单帧请求保持等待，直到实际发送或被校验操作取消。

页面显示模拟器当前状态及最近发送帧，并不证明 Agent 已接受该重量。恢复发送只恢复设备输入，不自动重放业务采集、确认或打印。

## HTTP 接口

`GET /api/state` 返回串口路径、重量、目标重量、动态／波动／暂停状态、校验控制、读取端是否打开、发送及坏帧计数、未送出计数、最后发送帧和错误信息。未送出包含没有读取端或写入失败的帧，不计暂停期间。

`POST /api/control` 必须使用 `application/json`，成功返回当前状态；非法操作或参数返回 400。示例：

```bash
curl http://127.0.0.1:8091/api/state

curl -X POST http://127.0.0.1:8091/api/control \
  -H 'Content-Type: application/json' \
  -d '{"action":"set_weight","weight":"30.125"}'

curl -X POST http://127.0.0.1:8091/api/control \
  -H 'Content-Type: application/json' \
  -d '{"action":"up","weight":"12.5"}'

curl -X POST http://127.0.0.1:8091/api/control \
  -H 'Content-Type: application/json' \
  -d '{"action":"pause"}'
```

页面断开不停止串口输出；Ctrl+C／SIGTERM 停止程序并释放虚拟串口。重启不保留重量、暂停或故障状态，重新从稳定零值开始；PTY 路径可能变化，须核对并重新应用 Agent 配置。

## 在服务器运行

```bash
task device:d12:build
./bin/weighbridge-d12-simulator
```

生成单个内嵌控制页的二进制，不依赖工作目录。将 Linux 二进制复制至服务器后，与该服务器的 Agent 同机运行即可。默认仅本机监听，可用 SSH 转发控制页：

```bash
ssh -L 8091:127.0.0.1:8091 user@server
```

需要开发局域网直接操控时，显式指定监听地址：

```bash
task device:d12 -- --listen 0.0.0.0:8091
# 或
./bin/weighbridge-d12-simulator --listen 0.0.0.0:8091
```

控制页与接口没有独立账号体系，只用于受信开发环境。浏览器可以远程访问，PTY 本身仍是 Agent 所在主机的本地设备，不通过 HTTP 提供远程串口。

## 协议及验证边界

固定 18 字节：`STX A B C 显示重量[6] 皮重[6] CR CKS`。A=`0x2d`，B=`0x30`（稳定）／`0x38`（动态），C=`0x20`，皮重固定零。CKS 为前 16 字节累加取低 8 位，不含 CR。遵循[现场 D12 配置](keli-d12-agent-integration-guide.md)，源单位显式设为 `t`；B4 保留抓包值，不能依其文档 kg 含义再次换算。

报文生成器与生产解析器分开实现，以现场 `1.560 t` 的稳定／动态原始帧作独立断言。行为测试经过 Linux PTY 和生产串口驱动，检查重量、动态位、单帧容错、连续坏帧、停发过期、恢复、无读取端不积压、退出释放及重启归零。

PTY 验证覆盖软件终端和协议行为，不模拟 RS232 电气、真实波特率传输、接线、仪表固件或现场串口权限。负数、超载、净重、非零皮重、原始抓包回放和摄像头／完整业务联调不在本次范围。软件验证记录见[交付记录](../requirements/d12-simulator-delivery.md)。

复验入口：`task backend:check`；已安装项目 Web 开发依赖及 Playwright Chromium 时，运行 `task device:d12:smoke` 验证独立二进制、控制页操作、桌面／窄屏布局及退出清理。冒烟使用临时回环端口和临时目录，不连接已有 Agent 或业务库，结束后自动停止进程、关闭浏览器并移除临时目录；截图保存在忽略的 `.task/d12-simulator-*.png`。
