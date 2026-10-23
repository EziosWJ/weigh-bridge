# 模拟摄像头：开发接入与维护备忘

记录日期：2026-10-23。本文保留实际部署地址，供项目内部开发使用；公开发布使用[博客稿](ffmpeg-rtsp-camera-simulation-blog.md)，其中已替换主机地址。

## 当前状态

已在指定服务器部署并保持两路模拟 RTSP 流运行，完成跨主机截帧和故障恢复检查。首次部署仅验证模拟源；后续本机联调已配置真实 Agent 的前后地址，并修正首次采集的页面门槛，见下文联调记录。**尚未完成当前运行 Agent 的图片上传及完整称重闭环验收**。

| 项目 | 实际值 |
| --- | --- |
| SSH | `ssh wangjian@192.168.1.10` |
| 远端目录 | `/data/camera` |
| Compose 项目 | `weighbridge-camera-sim` |
| 服务名 | `mediamtx`、`front`、`rear` |
| 车前地址 | `rtsp://192.168.1.10:8554/front` |
| 车后地址 | `rtsp://192.168.1.10:8554/rear` |
| 对外监听 | `192.168.1.10:8554`，TCP RTSP |
| 视频 | H.264、1280×720、15 fps，约一秒关键帧间隔 |
| 内容 | 动态测试图、FRONT/REAR、北京时间、帧号、模拟标记；无车辆或车牌素材 |
| 版本 | MediaMTX `v1.21.2`，FFmpeg `8.1.2` |
| 运行恢复 | Docker 服务已启用开机启动，容器使用 `unless-stopped`；宿主机重启未实测 |

实际固定镜像：

```text
registry-1.docker.io/bluenviron/mediamtx@sha256:c0feb4acc548d5f410d57d9046f62dfa355e2abc4098f5243de9e86430c77a95
```

远端文件如下：

| 文件／目录 | 用途 |
| --- | --- |
| `.env` | 固定镜像摘要；实际 Compose 直接绑定 `192.168.1.10` |
| `compose.yaml` | 三个容器、目录挂载、重启和日志轮转 |
| `mediamtx.yml` | TCP RTSP 与 front/rear 两个发布 path |
| `publish.sh` | 实时生成两路测试画面 |
| `fonts/DejaVuSans.ttf` | 从宿主机复制的字体 |
| `verify.py` | 并发抓拍两路、各两次，验证 JPEG 标记、耗时和摘要变化 |
| `verification/report.json` | 最新一次 `verify.py` 结果；重复执行会覆盖 |
| `verification/recovery.json` | 本次单路故障与恢复记录 |
| `verification/front-*.jpg`、`rear-*.jpg` | 容器内验证及开发机跨主机截帧样图 |
| `README.md`、`DEPLOYMENT.md` | 远端维护说明及实际部署记录 |

博客为便于复现，把 Compose 的绑定地址参数化为 `CAMERA_BIND_IP`；实际部署文件使用上表中的固定 IP。除这一点外，博客中的 Compose、推流脚本和 MediaMTX 配置来自本次实际部署。

## 日常查看与启停

登录后在指定目录操作，仅作用于该 Compose 项目：

```bash
ssh wangjian@192.168.1.10
cd /data/camera

docker compose ps
docker compose logs --tail=80 mediamtx front rear
```

根据需要选择以下命令：

| 操作 | 命令 |
| --- | --- |
| 创建／启动全部服务 | `docker compose up -d` |
| 重启画面生成器 | `docker compose restart front rear` |
| 重启 RTSP 服务 | `docker compose restart mediamtx` |
| 模拟后路断开 | `docker compose stop rear` |
| 恢复后路 | `docker compose start rear` |
| 停止全部容器并保留 | `docker compose stop` |
| 删除本项目容器和网络 | `docker compose down`；目录内配置和验证文件保留 |

重启前先确认无人正在进行依赖这两路流的联调。修改文件后，推流脚本的变更需要重启对应容器；Compose 配置变更用 `docker compose up -d` 应用。MediaMTX 配置变更后检查其日志及两个 path 状态。

## Agent 接入步骤

1. 确认使用包含 `server/internal/agentcamera` 的新版 `agent-printer`。现场旧版 D12 二进制不能仅凭重量正常就认定具备摄像头能力。
2. 在 **Agent 实际运行环境**检查 `ffmpeg -version`。模拟服务器的 FFmpeg 在容器内，不会自动为 Agent 所在主机提供可执行文件。注意 systemd 服务的 PATH 可能与交互式 shell 不同。
3. 从 Agent 所在机器分别读取两个 RTSP 地址，确认网络可达、能截取 JPEG、耗时满足预算。
4. 在统一设备维护页填写下列配置，保存并明确应用，核对已生效参数和进程运行摘要。维护页只显示摄像头配置状态，没有视频预览或独立测试抓拍按钮。
5. 在隔离联调环境准备健康、稳定且业务允许的仪表数据，再从称重工作台触发采集，检查新图上传、两个视角、摘要和称重关联。采集期间仪表失稳、重量变化或链路异常应拒绝。

```dotenv
WEIGHBRIDGE_CAMERA_FRONT_URL="rtsp://192.168.1.10:8554/front"
WEIGHBRIDGE_CAMERA_REAR_URL="rtsp://192.168.1.10:8554/rear"
WEIGHBRIDGE_CAMERA_TIMEOUT_MS=6000
```

摄像头键通过 `ReadEnvironment` 从受维护的 `agent.env` 读取。`printerOptions` 的独立参数兼容分支未读取这些摄像头环境变量；不要仅在 shell 中 `export` 后就认定参数已加载。通过既有服务使用 `--env-file`，保留原身份、密钥、地磅和打印账本。

前后地址须同时配置。默认 2000 ms 在本次模拟 RTSP 条件下不足；6000 ms 是当前联调起点。两路抓拍并发，随后两图依次上传，可选识别也共用该预算；外层上限为 8 秒。识别失败允许保留有效图片供人工核对车牌，但任一路抓拍失败不能用旧图补齐。

当前没有车牌识别服务；不凭空填写识别地址。与真实 Agent 同磅的独立 `agent-mock` 必须互斥，不能用 mock 替真实 Agent 单独补重量。

## 验证入口与已经测得的结果

远端执行：

```bash
cd /data/camera
python3 verify.py
```

该脚本依赖远端 Python 3，通过运行中的推流容器调用 FFmpeg，以主机实际发布的地址读取。每次并发读取两路，执行两轮；每个子进程设 8 秒超时，并要求实际返回时间小于 6 秒。会重写样图和 `report.json`。

| 已执行检查 | 结果 |
| --- | --- |
| Compose 配置与三个容器状态 | 通过 |
| 同参数截帧：两路各两次 | JPEG 标记正常，各路两次 SHA-256 不同；约 2.3～3.1 秒／次 |
| 开发机经局域网直接截帧 | 两路成功，约 2.6 秒／次；人工查看了方向、时间与帧号 |
| 停后路时读前路 | 正常，约 2.6 秒 |
| 停后路时读后路 | 明确失败且无图片，约 0.18 秒 |
| 后路恢复 | 成功，约 2.5 秒 |
| MediaMTX 重启 | 两路推流自动恢复，重新截帧通过 |

截图已归档到本仓库： [车前](assets/camera-sim/front.jpg)、[车后](assets/camera-sim/rear.jpg)。故障恢复检查是在部署时单独执行并保存的，`verify.py` 本身不会停止／启动服务。

资源占用仅采样一次：每个生成器约 20%～23% 单个逻辑 CPU、约 44 MiB 内存；MediaMTX 约 23 MiB。没有做长期容量或宿主机重启验收。

## 常见问题

| 现象 | 先检查 |
| --- | --- |
| 镜像拉取 403 | 原加速站曾拒绝请求；本次使用显式 `registry-1.docker.io` 成功，不需要改 Docker 全局配置 |
| 容器运行，但 RTSP 返回找不到流 | 看 MediaMTX 日志是否有该 path 的在线发布者，再看对应 front/rear 容器日志 |
| 另一台机器连不上 | 地址应使用 `192.168.1.10`；检查 TCP 8554、路由和防火墙；该部署只提供 TCP RTSP |
| Agent 抓拍失败，播放器却能播放 | 用 Agent 同一组 FFmpeg 参数和超时截帧；检查它实际加载的地址、二进制版本、FFmpeg PATH |
| 默认 2 秒超时 | 本次取图约 2.3～3.1 秒；从 6000 ms 联调，完整预算包含上传与可选识别 |
| 显示已保存，运行仍用旧地址 | 区分草稿、保存和已生效；执行维护页应用并核对运行摘要 |
| shell 导出了参数但无效 | 摄像头配置需从实际 `--env-file` 读取 |
| FFmpeg 报字体或滤镜错误 | 检查字体挂载、`drawtext`、`testsrc2` 和 `libx264`；固定镜像已验证这些能力 |
| Agent 报仪表未稳定／不可用 | 检查 D12 状态；模拟流只替代摄像头，不会生成 Agent 的重量数据 |
| 摄像头状态显示已配置 | 只表示有参数；成功取图和上传需通过实际采集确认 |
| 旧维护页提示 RTSP 未实现 | 旧独立维护页残留过时文案；以新版代码和实际运行版本为准 |

## 代码入口与下一步

### 2026-10-23 本机工作台联调修正

工作台曾要求已有前后图片才启用采集，但真实 Agent 只在收到 `capture.request` 后抓拍，导致首次采集互相等待。已移除发起采集前的预览图片门槛，保留本次抓拍结果和人工确认要求；采集返回的两路图片也可显示在现场画面中。首次无图不表示重量链路未接通，当前画面仍为按需抓拍，非实时视频。

本机实际 FFmpeg 单帧验证：`/front` 和 `/rear` 均在约 3 秒内返回 JPEG，误填的 `/near` 返回 400 且无图片。本地忽略的 `agent.env` 已改用 `/front`、`/rear`，超时 6000 ms；现有 Agent 需重启读取修改。

`npm run test:weighing:agent-capture`、`task frontend:lint` 和 `task frontend:build` 通过。新增浏览器回归只使用隔离请求夹具，覆盖真实工作台与 store，不连接现有业务库；不能据此宣称现有 Agent 到业务服务的两张图片上传或完整称重闭环已验收。下列后续项继续保留。

| 位置 | 用途 |
| --- | --- |
| [agentcamera/camera.go](../../server/internal/agentcamera/camera.go) | HTTP/RTSP 截帧、图片校验、上传与可选识别 |
| [agent-printer/main.go](../../server/cmd/agent-printer/main.go) | `captureEvidence`、采集中仪表状态检查及外层 8 秒上限 |
| [agent-printer/config.go](../../server/cmd/agent-printer/config.go) | `--env-file` 选择与独立参数兼容分支 |
| [agentprinter/config.go](../../server/internal/agentprinter/config.go) | 字段、默认值、校验与 `ReadEnvironment` |
| [维护页面](../../web/src/pages/agent-maintenance.tsx) | URL 配置、脱敏与应用入口 |
| [本厂部署说明](standalone-deployment.md) | 正式部署、维护与恢复边界 |

后续尚待完成：

- 确认目标 Agent 版本和 FFmpeg 依赖，实际应用这两路地址。
- 验证 Agent 到业务服务的两张新图上传、SHA-256 校验及工作台显示。
- 结合稳定仪表数据验证采集、失稳拒绝和单路失败拒绝。
- 按需要准备车辆素材并接入实际车牌识别服务。
- 用真实摄像头、D12 带载及实物打印完成现场闭环验收。

模拟截图只用于联调。本次记录不改变 [Issue #6 交付](../requirements/standalone-site-delivery.md)中完整真实生产闭环仍待验收的结论。
