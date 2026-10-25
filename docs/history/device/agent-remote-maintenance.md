> **历史档案（2026-10-25）**：以下保留页面演进、旧版操作及当时验证事实；现行说明见[原路径的收敛版](../../device/agent-remote-maintenance.md)，实际交付见[当前状态](../../next.md)。

# Agent 持有配置与局域网维护

> **2026-10-25 效力：部分被替代。** [ADR 0030](../../adr/0030-local-device-responsibilities-and-configuration.md) 起 Agent 只持有串口与打印配置，相机、识别和视频服务归 Server；维护页改为单次“保存并应用”，不再保留草稿步骤。下文关于设备连接、主进程常驻、作业保护和结果核实的说明继续适用，草稿与相机配置仅描述旧版。当前操作见[设备维护规格](../../requirements/local-device-maintenance-spec.md)与[本厂部署](../../device/local-deployment.md)。

2026-10-23。决策见 [ADR 0026](../adr/0026-agent-owned-device-maintenance.md)。业务 API／数据库与 Agent 可同机，也可部署在同厂局域网不同设备。设备进程保持一个，既连接 API 又执行仪表、摄像头和打印，不新增维护常驻服务或 HTTP 端口。

## 配置归属与操作

API 配置 `maintenance.enabled: true` 启用统一维护入口。旧 `maintenance.data_dir` 只兼容为启用标记，API 不再读取或创建该目录中的设备参数。安装和开发初始化使用新开关；已有 API 本机草稿不自动覆盖 Agent 的真实配置。

Agent 按原命令启动：

```sh
weighbridge-agent-printer --env-file /path/to/agent.env --state-dir /path/to/state
```

`--env-file` 指定的文件是有效设备配置；`--state-dir` 的父目录保存 `maintenance.json`（草稿、有效记录、上一份）、`maintenance.jsonl`（维护记录）及状态文件。路径可以不同，维护应用始终写入真实 `--env-file`，不会另建一份同名环境文件。原打印账本及身份目录继续保留。

旧版页面曾把“保存草稿”和“保存并应用配置”分成两步。ADR 0030 后当前维护页只有一次“保存并应用”：参数经现有设备连接保存到 Agent 主机，并完成校验、重载、核实和必要回退；刷新从 Agent 重新读取实际生效参数。保存成功不等于设备正常，失败或结果未知后先读取实际状态核实，不自动重放维护动作。

“启动设备／停止设备／重载设备”控制 Agent 内设备执行，主进程和 WebSocket 保持。停止后仍能维护配置并恢复设备；Agent 整个进程离线时不能远程读写实际配置或启动进程，使用其宿主机 systemd／本机命令恢复。

仪表及摄像头参数变更不要求 API 主机安装 CUPS。没有改变打印参数时，Agent 等待并核实本进程设备执行后重载；当前有打印执行时拒绝重载。改变队列或打印版式时仍检查 Agent 主机的真实 CUPS 队列与未结束作业，不用模拟结果代替。已经排队的旧打印命令不会在新设备配置下自动执行，打印结果不明仍按原规则核实。

应用或设备重载与 API 中在途称重协调；切换后未确认采样需重新采集，已提交记录不改写。设备身份和业务归属仍由本厂业务服务管理，普通参数维护不改绑 Agent。

## 通信与权限

复用原认证 WebSocket。Agent 上报 `maintenance.state`（脱敏有效配置和设备执行状态），API 使用 `maintenance.request`／`maintenance.result` 关联一次请求和回复；不建立离线命令队列。配置与日志、作业、校准、维护记录都在 Agent 主机处理，API 保留既有 JWT、维护 RBAC 和业务审计。

原 `/api/system/agent-maintenance/*` 页面接口保留；新增协议详情见 [v1](../../../protocol/v1.md)。摄像头公开预览地址来自在线 Agent 当前生效配置，浏览器直接连接 MediaMTX WHEP，不增加媒体鉴权。

## 部署与恢复

1. Server 主机安装业务服务、PostgreSQL 和 FFmpeg（RTSP 抓拍需要），启用维护；Agent 主机准备真实串口、CUPS（需要打印时）及设备访问权限。FFmpeg 不再要求安装在 Agent 主机。
2. Agent 连接文件只含 Server 地址，由安装工具或 `agent-provision --server-url` 从 Agent 主机视角显式生成（同机可用回环地址，分机须用局域网 IP 或域名）；不再需要密钥、配对文件或人工身份。日常维护不再下载或复制配置文件。
3. 两个自研进程升级到同一版本。旧 Agent 可继续按既有称重协议接入，但未提供新维护能力时页面明确提示版本／连接状态，不在 API 主机伪造草稿。
4. Agent 设备使用 systemd 配置进程开机启动和故障恢复。API 不调用本机 systemd 控制另一台设备。
5. 备份分别覆盖业务数据库与图片，以及 Agent 设备的实际环境文件、维护记录、状态目录、身份和完整打印账本。恢复不自动补称、补打或重放维护命令。

## 软件验证

- 真实 WebSocket 测试：不同环境文件与状态目录、草稿与生效区分、共享摄像头应用、停止设备仍保持同一连接、停止后读取及恢复、操作者在 Agent 维护记录保留、离线拒绝实际配置读取。
- 打印执行期间拒绝设备重载；重载前排队的打印命令不在新配置下执行；浏览器取消请求后，已发送的应用操作完成前仍保持称重维护暂停；受影响模块运行 race 检查。
- 隔离 PostgreSQL 验证实际 API 启动、既有业务权限及厂区范围；无 Agent 时仅报告离线，不创建 API 本机设备文件。
- 维护页浏览器回归覆盖先保存当前表单再应用、刷新保留字段、设备执行语义和错误提示；按仓库入口完成后端及前端检查。
- 本机实际 API／Agent 与既有模拟服务器联调：摄像头配置保存应用后刷新保留，运行摘要与真实 `agent.env` 一致；前后 WHEP 视频均为 1280×720，两路 Agent RTSP 诊断抓拍成功，浏览器无页面异常。未执行正式称重确认或打印，未改模拟服务器配置。

验证入口：`task check`、`task build:check`、`task agent:printer:build`、`npm --prefix web run test:standalone-management`；Agent、通信和维护模块加跑 `go test -race`。隔离 PostgreSQL 使用 `TestPostgresDevelopmentMaintenance`、`TestPostgresCameraPreviewUses`、`TestPostgresBusinessMaintenance` 集成测试。

以上是软件与隔离环境验证。真实双机、串口非零带载、打印及完整现场闭环另行验收。
