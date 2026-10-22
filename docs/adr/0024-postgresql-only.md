---
status: accepted
---

# Server 统一使用 PostgreSQL

2026-10-22，用户确认生产目标使用 PostgreSQL，并授权收敛实现、测试和文档。Server 的开发、集成测试及生产运行统一使用 PostgreSQL，取消 SQLite 运行支持、双库迁移锁步和兼容测试要求。该决定取代 ADR 0010 的双库生产支持，以及 ADR 0004 中长期多数据库兼容目标；同时替代旧规格中的 SQLite 开发与验收条款。

保留现有 PostgreSQL schema/seed 路径和版本历史。SQLite 旧 SQL 移至 `docs/archive/sqlite-migrations/`，仅作历史档案，不被 migration 命令、发布镜像或测试使用，后续业务变更无需补齐。已有 SQLite 数据不会自动转换或删除；需要保留的旧库应连同旧版本封存，切换到 PostgreSQL 前另行核对需迁移的数据。

纯业务单元测试不要求数据库；涉及持久化、迁移、约束、事务和 API 的集成测试使用隔离的临时 PostgreSQL。原 SQLite 测试中的业务行为覆盖迁移到 PostgreSQL，SQLite 专有的 PRAGMA、文件备份和锁等待测试随支持退役。浏览器与监控验收也使用临时 PostgreSQL，测试结束清理容器。

取舍是本地验证需要 PostgreSQL 服务，换取开发、测试与生产行为一致，以及一套迁移、备份运维和构建路径。Server 不再因 SQLite 要求 CGO 或 C 编译器。Agent 本地存储按独立设备需求决定，当前没有新增 SQLite 的要求；本次不改动现有 Agent 账本。

这是数据库支持范围的调整，不改变业务规则、现场验收边界或本厂自治改造的范围。当前开发基线见 [基线索引](../development-baseline.md)，实际改动和验证见 [交付记录](../requirements/postgresql-only-delivery.md)。
