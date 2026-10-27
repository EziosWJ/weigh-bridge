# Agent 通信协议

[通信契约](v1.md)按版本描述接入、测量、配置和打印；沿用文件路径以保护引用。当前 Server／Agent／mock 使用 v4（`agent.hello.ack` 下发身份编码和安装实例标识）；v2 的数字身份与 v1 的密钥配对保留为历史，Server 拒绝旧版本握手。先读 v4 的不兼容变化，不将旧配对或 Agent 图片上传作为新采集前提。

产品职责见[总规格](../docs/requirements/standalone-site-spec.md)，设备行为见[设备规格](../docs/requirements/local-device-maintenance-spec.md)，实际软件与现场验收见[当前状态](../docs/next.md)。模拟和真实 Agent 使用统一业务链路，输入模拟不代替现场证据。
