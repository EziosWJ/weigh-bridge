# SQLite migration 历史归档

2026-10-22 按 [ADR 0024](../../adr/0024-postgresql-only.md) 退役。此处保留原 `server/migrations/sqlite` 的 schema/seed SQL，供历史追溯；不参与运行、镜像发布或测试，后续 PostgreSQL 版本无需同步补齐。

该目录不是数据转换工具。现有 SQLite 库应与支持它的旧应用版本一起封存；当前 Server 只连接 PostgreSQL。
