# 摄像头接入、预览与维护

适用 ADR 0030 的 Server 摄像头模块。业务与失败规则见[设备规格第 6～7 节](../requirements/local-device-maintenance-spec.md#6-独立摄像头模块)，软件及现场状态见[当前状态](../next.md)。旧共享配置、Agent 取图及 2026-10-23 实验结果保存在[接入档案](../history/device/camera-live-preview.md)。

## 接入步骤

1. 在 Server 主机准备 RTSP 取帧需要的 FFmpeg，确认实际服务 PATH 和相机网络可达。相机数量可变，不要求固定前后两路。
2. 在“摄像头管理”新增名称、源 RTSP 地址、独立凭据及称重抓拍用途；保存后用于后续采集，不需要重载 Agent。配置变更保留历史证据，并使受影响未确认采样重新采集。
3. 逐台执行测试抓拍，核对新图、时间、名称和具体失败原因。测试不创建称重样本，不进入正式证据，历史测试成功不代表当前持续在线。
4. 需要实时预览时，准备本厂可达的 MediaMTX 管理接口和浏览器可达的公开 WHEP 入口，在视频服务设置填写。Server 生成托管流映射并报告同步结果，管理凭据不下发浏览器。
5. 如需车牌候选，在识别设置选择一个用于抓拍的相机及外部接口；复用本次图片，不额外抓图。未部署识别服务时保持关闭，候选始终由操作员核对。

Agent 离线不影响网络相机配置、测试和预览；正式称重仍需 Agent 提供有效仪表依据。相机及视频设置不放入 `agent.env`，已迁出的旧源参数需在 Server 核实导入，不能从旧转流地址猜测源凭据。

## 预览、抓拍与故障

Server 正式抓拍直接读取源相机，MediaMTX 只承载浏览器预览；两种结果分别反馈。强制抓拍、缺图策略、统一并发截止及配置版本规则统一见设备规格。预览不可用不等于直连抓拍失败，预览画面不能替代当次证据。

浏览器原生 video 解码，不逐帧写入 React 业务状态。沿用仓库内 MediaMTX v1.21.2 `reader.js`，来源、生命周期调整和 MIT 许可见[vendor 说明](../../web/src/lib/vendor/mediamtx/README.md)。连接、播放、冻结、重连、隐藏及离开时的资源管理见[工作台页面规则](../../web/design-system/pages/weighing.md#预览与证据)。

| 现象 | 检查 |
| --- | --- |
| 测试抓拍失败 | Server 实际源地址／凭据、FFmpeg PATH、相机网络及整次超时；不要只用 Agent 主机连通性代替检查 |
| 抓拍成功但预览失败 | MediaMTX 托管路径同步结果、公开 WHEP 地址、浏览器网络／origin、媒体端口及证书 |
| 配置保存成功但映射待同步 | 保留数据库相机配置，检查管理接口并按当前版本重新同步；不显示旧映射为新配置成功 |
| MediaMTX 重启或文件重载后路径消失 | Server 按数据库期望核对、恢复本系统路径，保留离线删除清理信息；不删除其他应用流 |
| 有图但候选失败 | 检查外部识别接口及指定抓拍源；识别失败本身不阻断满足其他条件的称重 |

## 部署与验证入口

MediaMTX 可独立部署，安装器不负责远端进程生命周期；样例见[mediamtx.yml.example](../../agent/deploy/mediamtx.yml.example)。核对 Server 到相机／管理接口、浏览器到 WHEP／媒体端口的不同网络路径。正式 HTTPS 页面使用浏览器信任的预览入口，不增加业务播放鉴权层。

开发可用[模拟源](camera-simulation-dev-memo.md)。软件回归复用 `test:camera-management`、`test:weighing:camera-live`、`test:weighing:agent-capture` 及业务后端冒烟；真实 MediaMTX 集成入口为 `MEDIAMTX_BIN=/path/to/mediamtx task camera:integration`，使用临时端口和进程并清理。入口存在不代表本次已执行，实际结果只记在交付记录。
