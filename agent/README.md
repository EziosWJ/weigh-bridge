# Agent（预留）

计划部署于酒厂地磅房 Linux 工控机，负责柯力 D12、RTSP 摄像机配合 HyperLPR3、Linux 针式打印机，以及通过 WSS 与远程 Server 通信；计划使用 SQLite 保存现场状态。

Phase 1 只预留此目录，未实现程序、驱动、识别、通信或打印。正式称重必须由操作员确认，断网不得擅自生成正式电子过磅单。领域约束见 ../CONTEXT.md。现有 Server host-collector 仅负责宿主机监控，不是本 Agent。
