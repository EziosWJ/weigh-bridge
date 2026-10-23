# 摄像头共享配置、实时监控与维护

日期：2026-10-23。依据：[ADR 0025](../adr/0025-camera-stream-maintenance.md)。范围：统一维护、运行时预览配置和独立维护测试。按用户最终修订，沿用现有局域网 MediaMTX 直连方式，不增加视频鉴权层。17:09 模拟服务器启用记录保留于下文。真实摄像头及长期运行仍待验收。

## 播放与称重抓拍

左侧大区域通过 WebRTC 播放所选视角，默认车头，支持车头／车尾切换、小窗／标准／大窗及放大。放大复用同一 MediaStream；下方“本次抓拍／已保存抓拍”继续展示 Agent 获取并随称重保存的静态证据。预览与 Agent 在线状态分别表达，预览故障不会改变现有采集／确认条件，也不会自动提交、采集或补办业务。

视频由浏览器原生 `<video>` 解码显示，不逐帧写入 React 状态或业务缓存。官方 MediaMTX v1.21.2 `reader.js` 随 Web 打包，来源与生命周期修订见 [vendor 说明](../../web/src/lib/vendor/mediamtx/README.md)，MIT 许可随构建输出到 `third-party/mediamtx-LICENSE.txt`。

连接期间 15 秒没有画面进展，或播放后约 10 秒没有新显示帧，释放当前连接再重建；连续失败按 1、2、4、8、16、30 秒退避，最多间隔 30 秒。只有实际显示帧进展才标记“实时画面”，不能以连接成功替代。页面隐藏时暂停并释放连接，返回时重新连接；切视角、切地磅和卸载时取消请求、定时器并释放流，关闭已建立的 WHEP 会话。恢复只处理视频预览。

## 日常维护

进入“设备维护 → 配置 → 摄像头共享配置”。旧 RTSP／HTTP 抓拍配置在明确切换前继续有效；点击“切换为 MediaMTX 配置”后填写服务入口、前后流名称及超时，保存仅形成草稿。模拟环境可填写：

| 字段 | 模拟环境示例 |
| --- | --- |
| RTSP 服务入口 | `rtsp://192.168.1.10:8554` |
| 实时预览服务入口 | `http://192.168.1.10:8889` |
| 车前／车后流名称 | `front` / `rear` |
| 抓拍超时 | `6000 ms` |

两个服务入口允许不同主机、端口和协议；WHEP 可包含反向代理路径前缀。入口不含用户名密码、查询参数或末尾流名称；前后流名称须不同。页面展示不含凭据的生成地址供核对。正式 HTTPS 页面使用浏览器信任的 HTTPS 预览入口。

按 [ADR 0026](../adr/0026-agent-owned-device-maintenance.md)，配置归 Agent 主机。点击“保存并应用配置”先将当前表单保存到 Agent，再暂停新的采集及确认、等待在途处理完成，由 Agent 重载设备执行，主进程和维护连接保留。仅改变仪表或摄像头时不要求 API 主机安装 CUPS；改变打印参数仍核实 Agent 主机队列和作业。MediaMTX 模式核实运行配置摘要，失败恢复原环境。已提交但客户端结果不明的业务仍通过原请求查询核实，不自动重放。

切换成功后，未确认的旧采集结果因设备连接版本变化不能继续确认，须重新采集；既有正式称重记录不改写。整组停用保留连接信息，关闭预览、阻止新正式采集，重新启用也须保存并应用。配置生效与摄像头可连接是两种状态：摄像头离线不阻止保存／应用，但缺少当次前后抓拍不能正式称重。

配置由 Agent 管理，沿用其私有 `maintenance.json` 草稿和回退记录、真实 `--env-file` 指定的有效环境，不增加数据库表。`WEIGHBRIDGE_CAMERA_CONFIG` 保存共享映射；执行地址由服务入口和流名称生成。`GET /api/weighing/scales/{id}/camera-preview` 返回该地磅当前已核实的公开预览地址，复用现有称重查询权限及厂区范围，浏览器约每 2 秒复查，读取失败立即释放画面，不继续展示旧连接。已移除 `VITE_CAMERA_*` 入口，不需要重建 Web 才能更新摄像头；`.env*.local` 中遗留变量也不再生效。

“设备与打印 → 摄像头 → 打开摄像头测试”可检查当前生效配置的前后实时预览及 Agent 实际抓拍。测试期间暂停新的采集和确认；图片明确标为维护测试，经独立消息返回小图，不上传业务证据、不做车牌识别、不创建 sample。测试入口需要既有 `agent:maintenance:manage` 权限。

## MediaMTX 配套部署

浏览器直接连接配置中的 WHEP 地址，Agent 通过对应 RTSP 地址抓拍。MediaMTX 沿用现有局域网转流方式，不增加播放令牌、鉴权回调、管理接口或额外密钥；不把业务 JWT 传给 MediaMTX。

配置读取走现有业务登录及 `weighing:read`、厂区查询范围；维护配置和测试使用现有维护权限。维护页预览配置接口是 `GET /api/system/agent-maintenance/camera-preview`。这些是业务接口原有权限的复用，不改变媒体服务的播放方式。

MediaMTX v1.21.2 配置样例见 [mediamtx.yml.example](../../agent/deploy/mediamtx.yml.example)。只需按现场配置摄像头 source、RTSP／WebRTC 监听和浏览器 origin，开放相应媒体端口。当前模拟服务器已开启两路 RTSP 和 WHEP，无需为本次共享配置另加鉴权配置。

MediaMTX 由部署负责安装、上游摄像头接入、启动和恢复，系统不生成它的上游配置或控制其重启。升级业务服务时同步升级 Agent 二进制，备份共享配置及身份；配置回退继续使用维护页现有流程。MediaMTX 二进制和摄像头 source 配置同样纳入本厂备份恢复清单。

## 历史：模拟服务器 WebRTC 配置（17:09 已启用）

[模拟服务器](camera-simulation-dev-memo.md)首次部署只开放 TCP RTSP 8554，配置为 `webrtc: false`；2026-10-23 17:09 已启用以下配置并映射端口。仅重建现有 MediaMTX 容器，原镜像摘要、front/rear 推流、RTSP 地址和重启策略保持。两路生成器在连接中断后由原有容器重启策略恢复，三个容器均已运行正常。

`/data/camera/mediamtx.yml` 的实际 WebRTC 配置如下。`webrtcAllowOrigins` 应列出**浏览器实际访问的页面 origin**（协议、主机及端口）；本次已允许本机、开发机局域网地址及 MediaMTX 自带播放页，换地址或端口时需相应更新：

```yaml
webrtc: true
webrtcAddress: :8889
webrtcEncryption: false
webrtcAllowOrigins:
  - http://localhost:5173
  - http://127.0.0.1:5173
  - http://192.168.1.32:5173
  - http://192.168.1.10:8889
webrtcLocalUDPAddress: :8189
webrtcIPsFromInterfaces: false
webrtcAdditionalHosts: [192.168.1.10]
webrtcICEServers2: []
```

`/data/camera/compose.yaml` 的 **mediamtx 服务**已在原 TCP 8554 映射之外增加：

```yaml
- "192.168.1.10:8889:8889/tcp"
- "192.168.1.10:8189:8189/udp"
```

局域网需要同时可达 TCP 8889（HTTP 协商）和 UDP 8189（视频媒体）；只代理 HTTP 协商并不能自动代理 WebRTC 视频。此配置无需公共 STUN／TURN，互联网中断不影响局域网播放。如果现场阻断 UDP，可另行启用 `webrtcLocalTCPAddress: :8189` 并映射 TCP 8189，配置后验证延迟。

配置和应用前备份位于服务器 `/data/camera/backups/webrtc-20261023T090906Z/`，包含原 `mediamtx.yml`、`compose.yaml` 及本次文档更新前的 README／部署记录。应用时使用 `docker compose config --quiet` 校验，随后仅对 `mediamtx` 执行 `up -d --no-deps --no-build --pull never`，没有拉取或升级镜像。以后修改按[维护说明](camera-simulation-dev-memo.md#日常查看与启停)执行。

该阶段可直接打开 `http://192.168.1.10:8889/front` 或 `/rear` 使用 MediaMTX 自带播放页；称重台通过同路径下的 `/whep` 协商。HTTP 播放页用 GET 验证，MediaMTX 对 HEAD 可能返回 404，不能据此判定流不可用。

配置依据：[v1.21.2 配置](https://github.com/bluenviron/mediamtx/blob/v1.21.2/mediamtx.yml)、[官方网页接入](https://mediamtx.org/docs/read/web-browsers)、[WebRTC 网络与编码要求](https://mediamtx.org/docs/features/webrtc-specific-features)。本次模拟流的 H.264 已通过下面的跨主机浏览器验证，不需要另加转码进程。

## 2026-10-23 17:10 跨主机验证

从开发机 Chromium 打开 `http://127.0.0.1:5173/tests/weighing-camera-live.html`，加载实际播放组件及开发配置，直接连接远端 MediaMTX，无媒体请求夹具、不连接业务数据库。

| 检查 | 结果 |
| --- | --- |
| 车前／车后 WHEP | 两路 POST 均返回 201，远端日志确认 H.264 与 UDP peer connection 建立 |
| 持续解码 | 两路均为 1280×720，每路 2.5 秒新增 39 帧，track 为 live |
| 视角切换及卸载 | 原会话 DELETE 返回 200，远端日志确认关闭 |
| 放大 | 与主画面引用同一 MediaStream，没有新增 WHEP POST |
| 浏览器异常 | 无 pageerror，实际查看截图中的 FRONT／REAR、模拟标记、时间及帧号 |
| 原 RTSP 抓拍 | `verify.py` 两路各两次 JPEG 正常、各自摘要变化；车前约 2.98～3.00 秒，车后约 2.58～2.60 秒 |

远端浏览器报告：`/data/camera/verification/webrtc-report.json`；RTSP 报告：同目录 `report.json`。本机截图为 `/tmp/weighbridge-remote-camera-front.png`、`/tmp/weighbridge-remote-camera-rear.png`。测试浏览器结束后已关闭；没有停止用户现有前端或播放会话，没有触发正式称重或修改 Agent 配置。

## 本次统一配置的软件验证

以下检查已按移除额外鉴权后的最终代码通过：

- `task check`、`task build:check`（含后端 test/vet、前端 lint/build 及内嵌 Web）：共享映射、旧配置保留、环境往返、运行摘要核实、维护排空与取消恢复、测试抓拍不上传证据等。
- `task frontend:lint`、`task frontend:build`。
- `npm run test:weighing:camera-live`：运行时版本切换、停用、配置读取失败释放连接，以及直接 WHEP 播放。
- `npm run test:weighing:agent-capture`、`npm run test:standalone-management`。
- 隔离 PostgreSQL 集成：预览配置使用原有称重查询权限及厂区范围，原有维护／开发进程启动回归。不连接业务数据库。
- `MEDIAMTX_BIN=/path/to/mediamtx task camera:integration`：临时本机端口运行官方 v1.21.2、FFmpeg 双路推流和 Chromium，检查前后直接播放、实际解码和生成的 RTSP 地址截帧，结束清理子进程。不修改远端模拟配置。

这些属于软件／隔离模拟验证，不代表真实摄像头或长期运行已验收。

## 先前预览验证与未验收项

- `npm run test:weighing:camera-live`：隔离 Vite 与 Chromium，通过本机 WHEP 请求夹具及真实 RTCPeerConnection／动态 canvas 视频验证协商解码、双视角、放大复用、冻结／失败恢复、后台暂停恢复、地磅匹配、卸载与窄屏；另用实际工作台验证预览失败时仍能向 Agent 发起采集且不自动确认。不连接模拟服务器或业务数据库。
- `npm run test:weighing:agent-capture`：原工作台与 store 的首次抓拍及采集限制回归，使用旧配置 API 夹具隔离外部依赖。
- 后端 `task backend:check`：共享映射、Agent 端环境往返与重载、维护排空与取消恢复、测试抓拍不上传证据等。
- `npm run test:standalone-management`：原有维护权限及秘密不回显、新增共享配置、停用和草稿提交。
- 前端改动执行 `task frontend:lint`、`task frontend:build`。

仍待实际工控机 24～72 小时资源观察、真实摄像头、Agent 抓拍上传及完整现场称重闭环验收。不能将本地软件回归或模拟源播放通过当成生产或长期稳定性结论。

## Agent 持有配置补充

API 已改为经现有 WebSocket 转发，Agent 主进程常驻，配置与设备重载都在 Agent 主机完成；部署和离线恢复见[远程维护说明](agent-remote-maintenance.md)。原文中的同机 systemd／CUPS 维护流程属于之前实现，不作为本次分机维护方式。
