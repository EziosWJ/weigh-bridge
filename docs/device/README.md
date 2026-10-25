# 设备与部署资料

最终设备职责见[当前设备规格](../requirements/local-device-maintenance-spec.md)，实际完成度见[当前状态](../next.md)。本目录保留可操作资料及现场证据；旧版实验及配对资料集中于历史目录，不作为新安装步骤。

| 用途 | 入口与适用范围 |
| --- | --- |
| 已交付版本安装、启停及恢复 | [本厂部署](local-deployment.md)、[Agent 运行](../../agent/README.md)；核对各页版本说明 |
| 设备维护与本机恢复 | [局域网维护](agent-remote-maintenance.md)、[本机恢复与权限](agent-local-maintenance.md)；旧独立维护服务仅用于已有旧部署 |
| 协议 | [Agent—Server 契约](../../protocol/v1.md)；v1 历史事实与 v2 当前契约分开阅读 |
| D12 解析及现场参数 | [接入指导](keli-d12-agent-integration-guide.md)、[串口联调及带载原始报文](keli-d12-rs232-blog.md)；原始帧核验不替代完整业务验收 |
| D12 软件联调 | [虚拟串口模拟器](d12-simulator.md)；当前固定仪表参数按设备规格 |
| 打印与校准 | [连续纸校准](sk820-continuous-paper-calibration.md)、[现场测试](aisino-sk820-agent-test.md)、[Linux 记录](aisino-sk820-linux-record.md)、[经验整理](aisino-sk820-linux-blog.md) |
| 摄像头及模拟源 | [视频接入](camera-live-preview.md)、[模拟源备忘](camera-simulation-dev-memo.md)、[模拟 RTSP 部署实录](ffmpeg-rtsp-camera-simulation-blog.md)；Server 直连取图与视频映射分别核实，实验验证按原时点追溯 |

现场主机地址、当时命令和软件测试结果均有时间与设备范围。部署前核对实际版本、账号、目录及参数；正式数据库、图片、配置和打印账本按当前总规格保留。
