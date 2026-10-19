# 文档索引

当前工程入口为根 README.md、CONTEXT.md、CLAUDE.md、Taskfile.yml 和 server/README.md；阶段范围与实际验收见 requirements/，架构边界见 architecture/。

本目录原有开发初始化、通知与监控说明保留脚手架技术背景，使用说明中的工程路径已适配 server/、web/。这些模块的历史验收不替代当前项目的验收记录。

adr/、server-monitoring-tickets/ 和 ../record/ 保留来源历史正文；其中旧目录 base-go-api、react-admin 分别对应当前 server、web，旧 bin/base-go-api 对应 bin/weighbridge-server，其他 base-go-* 工具前缀对应 weighbridge-*。历史 Issue 链接仍属于源项目，不表示目标仓库已有同号工作项，也不表示地磅业务已实现。

设备侧硬件验证见 [Aisino SK-820 针式打印机 Linux 适配与打印验证记录](device/aisino-sk820-linux-record.md)；面向外部发布的调试笔记见 [把二维码打上 24 针针式打印机](device/aisino-sk820-linux-blog.md)。两者只记录驱动与打印链路，不代表 Agent 或正式业务打印已实现。

Agent 接口及独立 Web mock 见 [Spec](requirements/agent-mock-issue-spec.md)、[协议 v1](../protocol/v1.md)、[运行说明](../agent/README.md)与[交付及验证记录](requirements/agent-mock-delivery.md)。mock 通过不替代真实设备验收。
