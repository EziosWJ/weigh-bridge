# 模拟摄像头接入与维护

2026-10-25 收敛。以下主机及服务是 2026-10-23 的实际部署记录，使用前核对当前运行状态；实验验证与配置备份明细见[原始记录](../history/device/camera-simulation-dev-memo.md)。公开复现过程见[博客稿](ffmpeg-rtsp-camera-simulation-blog.md)。本页不宣称服务在任何时刻持续可用。

## 已记录的源与服务

| 项目 | 记录值 |
| --- | --- |
| 主机及目录 | `ssh wangjian@192.168.1.10`，`/data/camera` |
| Compose 项目／服务 | `weighbridge-camera-sim`；`mediamtx`、`front`、`rear` |
| RTSP | `rtsp://192.168.1.10:8554/front`、`rtsp://192.168.1.10:8554/rear` |
| 浏览器播放入口 | `http://192.168.1.10:8889/front`、`http://192.168.1.10:8889/rear` |
| 网络 | TCP 8554／8889，UDP 8189 |
| 视频 | H.264、1280×720、15 fps、约一秒关键帧；画面含模拟标记、北京时间和帧号，不含实际车辆 |
| 已记录版本 | MediaMTX v1.21.2、FFmpeg 8.1.2；固定镜像摘要及远端文件清单见原始记录 |

## 当前 Server 接入

从 **Server 的实际运行环境**确认源可达并具备 FFmpeg，再到摄像头管理新增两台，名称自行填写，源地址使用上表。它们只是两个可选择的模拟源，不形成产品固定前后两图约束；按测试场景选择是否用于称重抓拍。

逐台测试取新图，配置与预览分别核实。整次抓拍默认 6 秒，模拟源当时单帧约 2.3～3.1 秒，耗时只是实验结果，不代表真实设备保证。相机配置保存于 Server，不向 Agent 写源地址、流名称或识别参数。

旧实验 MediaMTX 的 front／rear 为发布者路径。若测试新视频同步，应使用可管理的实例或明确归属的路径；不把托管路径改写到共享的其他应用流。需要准备管理接口时由部署人员核对现有配置，原先的播放成功不证明新同步功能已验证。Agent 仍须提供仪表依据，模拟视频不补重量或替代人工确认。

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

## 诊断与证据

优先区分源不可达、Server 取帧失败、MediaMTX 同步失败及浏览器媒体连接失败，参照[摄像头接入](camera-live-preview.md#预览抓拍与故障)。不再通过 Agent 草稿或重载修复网络相机参数。

原始记录保留 `verify.py` 用法、JPEG 摘要、跨主机抓帧、单路故障／恢复、WebRTC 开启、历史图片及资源采样；重复执行远端验证会覆盖对应报告，须按当次任务范围操作。曾验证的运行恢复不等于宿主机重启或长时间现场验收。最新完成边界见[当前状态](../next.md)。
