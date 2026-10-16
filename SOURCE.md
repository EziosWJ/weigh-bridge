# 脚手架来源与许可记录

- 来源：https://github.com/EziosWJ/base-project-golang
- 分支：main
- 克隆基准提交：3dc5cbb3f3210547df1cf7a647ab14d30f887bdc
- 初始化日期：2026-10-16
- 目标项目：weighbridge（酒厂地磅管理系统）
- 发布仓库：https://github.com/EziosWJ/weigh-bridge（用户在 Phase 1 完成后指定；不是源仓库）。
- 项目目录和 Go Module 保留 Phase 1 确认的 weighbridge 命名。

保留源项目全部已跟踪代码、测试、migration、历史 ADR、record 和必要文档；目录迁移与工程引用按当前项目适配。历史 Issue URL 继续指向来源仓库，不视作目标仓库中的工作项。

克隆基准中没有已跟踪的 LICENSE 或 COPYING 文件，因此没有可复制的项目开源许可证文本。本次未新增或假定许可授权；发布前由维护者明确源代码许可条件。已有源码中的版权和第三方依赖声明保留，依赖许可不因项目重命名而改变。

兼容标识：JWT issuer=base-go-api、audience=react-admin 以及浏览器持久化 key react-admin-auth / react-admin-remembered-username 保留原值，以维持认证和本地状态行为；它们不是失效目录路径。

Prometheus 指标 base_go_api_* 保留原名，维持现有监控端点响应契约。
