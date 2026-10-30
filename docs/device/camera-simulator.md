# 摄像头车牌识别模拟器

2026-10-30。独立开发工具，用于摄像头不在身边时联调 Server 的车牌主动上报、心跳、候选车牌和工作台人工核对。模拟器按厂家主动上报协议向 Server 的 `POST /api/v1/event/pull` 发送请求，不直接写数据库，也不绕过 Server 的设备绑定、去重、有效期和业务校验。

本期只模拟车牌识别与心跳，不提供 RTSP 视频或抓拍图片，不能代替真实摄像头的现场验收。

## 启动与接入

在仓库根目录先启动 Server（本机开发默认 `http://127.0.0.1:8900`）：

```bash
task dev
# 另一终端
task device:camera
```

默认控制页 <http://127.0.0.1:8092>，默认 Server 地址 `http://127.0.0.1:8900`，默认设备序列号 `camera-sim-01`，心跳间隔 30 秒。默认只监听本机回环地址；控制页没有独立账号体系，只用于受信开发环境。

按摄像头管理页绑定的实际序列号启动：

```bash
task device:camera -- --server http://192.168.1.10:8900 --serial camera-sim-01
```

参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--listen` | `127.0.0.1:8092` | 控制页监听地址；局域网直接访问可显式设置 `0.0.0.0:8092` |
| `--server` | `http://127.0.0.1:8900` | 本厂 Server 根地址，只填协议、主机和端口；不要带 `/api` 或事件路径 |
| `--serial` | `camera-sim-01` | 与摄像头管理页绑定的设备序列号完全一致 |
| `--heartbeat` | `30s` | 心跳间隔，需大于零；联调可缩短为 `1s` |

需要局域网访问控制页时：

```bash
task device:camera -- --listen 0.0.0.0:8092
# 或构建独立二进制
task device:camera:build
./bin/weighbridge-camera-simulator --server http://192.168.1.10:8900 --serial camera-sim-01
```

也可用 SSH 转发保持默认的本机监听：`ssh -L 8092:127.0.0.1:8092 user@server`。

## 使用前提

1. 在「摄像头管理」为接收上报的摄像头启用上报，填写与 `--serial` 完全一致的设备序列号并保存；未绑定、停用或序列号不一致会收到 HTTP 403。离线阈值应大于心跳间隔。
2. Server 根地址必须从运行模拟器的主机可达。设备上报入口不要求用户 JWT，但必须满足部署的网络限制：可选 `WEIGHBRIDGE_CAMERA_EVENT_ALLOWED_IPS` 只接受逗号分隔的直接连接 IP，且不信任转发头；未配置时由防火墙或网关把入口限制在本厂可信网段。反向代理部署时同时核对代理本身的网络规则。
3. 模拟器与 Server 时钟保持同步。Server 用接收时间减设备时间判断车牌事件有效期（默认 5 分钟，可在配置管理调整）；时间在未来不会被判为过期，但会失去自动预填所需的时效。
4. 在工作台取得操作权后再触发新识别。模拟器只产生候选，不自动查找任务、不提交称重，也不替代人工确认。

## 页面与操作

页面顶部明确标注「模拟器 · 非真实摄像头」，展示当前随机车牌、事件号、设备序列号、Server 地址、上报地址、心跳状态和最近一次上报的 HTTP／ACK 结果。

| 页面操作 | 控制动作 | 行为 |
| --- | --- | --- |
| 刷新车牌并上报 | `recognize` | 随机生成符合现有车牌校验规则的新车牌，立即以新事件号上报 |
| 再次上报当前车牌 | `report_current` | 车牌不变，以新的识别事件再次上报，用于测试同车候选合并 |
| 重试上次上报 | `retry` | 仅在最近一次上报失败时可用；原样重发同一事件号和设备时间，用于验证 Server 去重与恢复 |

每次主动识别都生成更大的十进制事件号（19 位，避免 JavaScript 精度截断），设备时间使用当前本地时间的 RFC 3339 表示。心跳按 `--heartbeat` 周期自动发送，页面刷新不影响后台心跳。

失败时页面显示可理解原因，例如：

- HTTP 403：序列号未绑定／未启用，或本机 IP 不在允许来源。
- HTTP 400：事件字段、车牌或设备时间格式无效。
- HTTP 503：数据库／设备存储故障或并发过多。
- HTTP 404：`--server` 不是 Server 根地址。
- 连接失败或超时：Server 不可达或 10 秒内无响应。
- HTTP 200 但 ACK 操作类型不符、缺少 `errorNo`、`errorNo` 非 0、事件号不一致或响应不是 JSON：明确显示 ACK 问题及厂家描述。

重试会复用原请求体和事件号，因此不会产生新候选；若该事件已超过有效期，Server 仍可能返回成功 ACK 但不新增车牌记录，这是协议规定的补传行为。需要新的候选请使用前两个操作。

## 上报内容

心跳：

```json
{"operator":"heartbeat","deviceInfo":{"serialNumber":"camera-sim-01"},"info":{"eventId":1793324089364665560,"time":"2026-10-30T09:00:00+08:00"}}
```

车牌事件：

```json
{"operator":"CarLicenseSnapshot","deviceInfo":{"serialNumber":"camera-sim-01"},"info":{"eventId":1793324089364665561,"time":"2026-10-30T09:00:01+08:00","carLicenseAttriInfo":{"Number":"晋A12345","CarLicenseAttriColor":1},"carAttriInfo":{"Score":99}}}
```

不包含 `CaptureImage`、`BackgroundImage` 或其他图片字段。Server 按绑定摄像头和完整请求体 SHA-256 去重；同一事件失败后原样重试可恢复，已保存事件重复投递仍返回成功 ACK。成功响应的 `CarLicenseSnapshot-Ack.info.eventID` 必须回填本次事件号，模拟器以此确认成功。

Server 只在车牌事件成功持久化后返回 ACK；超过有效期的旧事件不新增记录，但绑定有效时仍返回成功 ACK。候选由本厂启用且绑定的摄像头事件构建：同一车牌按服务端接收时间 15 秒近邻窗口合并来源，自首次接收起 60 秒有效，最多展示 5 个不同车牌；旧补传、异常时间和无效车牌不自动预填。候选只用于人工核对，不形成正式称重证据。

## 控制页 HTTP 接口

```bash
curl http://127.0.0.1:8092/api/state

curl -X POST http://127.0.0.1:8092/api/control \
  -H 'Content-Type: application/json' \
  -d '{"action":"recognize"}'

curl -X POST http://127.0.0.1:8092/api/control \
  -H 'Content-Type: application/json' \
  -d '{"action":"report_current"}'

curl -X POST http://127.0.0.1:8092/api/control \
  -H 'Content-Type: application/json' \
  -d '{"action":"retry"}'
```

控制接口只接受 `application/json`、单个 JSON 对象和上述动作；非法请求或没有可重试的上报返回 400。`GET /api/state` 返回页面展示的状态，不包含请求体。

## 联调与验证步骤

1. 启动 Server，登录并确认本厂身份等基础配置；在摄像头管理页建立测试摄像头并绑定序列号。
2. 启动 `task device:camera`，确认页面心跳正常且「摄像头管理 → 上报」显示在线、最近记录更新。
3. 打开称重工作台并取得操作权，点击「刷新车牌并上报」，检查候选是否自动出现或提示冲突；按现有规则人工核对，再继续原有采集和称重流程。
4. 点击「再次上报当前车牌」，检查候选按同车合并来源，而不是占用新的候选位。
5. 临时停用绑定或改错序列号验证 403 原因；恢复后点击「重试上次上报」，检查失败原因清除且没有产生新事件。

软件验证：

- `task backend:check`：模拟器单元测试覆盖车牌规则、事件号、心跳、ACK 校验、HTTP 状态原因、连接失败、原样重试、控制页和参数校验。
- `task device:camera:smoke`：隔离 Chromium 验证独立二进制、心跳、两种上报、HTTP 503、厂家 ACK 错误、手动重试、移动端布局和退出清理；不需要 Server 或数据库。
- 隔离 PostgreSQL 集成：`cd server && go test -tags=integration ./integration -run '^TestCameraSimulatorPlateCandidateChain$' -count=1`。使用临时数据库和真实 API 路由，覆盖未绑定 403、绑定后原样重试、同车候选合并和未登录拒绝。测试结束自动清理容器。

以上均为软件和模拟验证，不连接真实相机、业务数据库或现场网络，不产生现场验收结论。

## 边界

- 不实现 RTSP 视频、抓拍图片、预览或识别引擎；需要图像时按[模拟源说明](camera-simulation-dev-memo.md)配置可控视频源，正式证据仍由 Server 直连相机取得。
- 不直接写数据库、不调用内部业务接口、不修改 Server 权限或校验规则；所有事件都经过设备上报入口。
- 模拟成功不能证明真实设备的报文大小、图片、昼夜识别、时钟漂移、重传和长期稳定性；真实摄像头仍需按[上报配置说明](camera-event-reporting.md)现场验收。
- 控制页默认只监听本机且无独立认证，不得直接暴露到非受信网络。
