# ADR-0010: SQLite 作为正式生产数据库

## Status

Accepted

## Context

ADR-0004 将 PostgreSQL、MySQL 和 SQLite 定为长期兼容目标，但当前只承诺 PostgreSQL。项目由 Coding Agent 持续开发；SQLite 能降低小规模单机部署的基础设施成本，但其文件型存储、单写入者和部署拓扑与 PostgreSQL 不同。仅能建立连接或完成少量 CRUD，不足以构成生产支持。

本 ADR 决定将 SQLite 提升为正式生产数据库。在与 ADR-0004 冲突之处，以本 ADR 为准；MySQL 仍是尚未实现的后续兼容目标。

## Decision

PostgreSQL 与 SQLite 都是正式支持的生产数据库。PostgreSQL 保持默认选择；SQLite 必须由配置显式选择。

两者在现有 API、数据约束、认证会话、权限、事务、审计日志、通知、文件元数据、schema migration 和 seed migration 上提供一致的可观察业务行为，但不承诺相同的部署能力、吞吐量、底层字段类型或亚秒时间精度。业务时间统一按 UTC 读写。

SQLite 生产模式只支持单个 API 实例访问本地持久卷中的文件数据库，适用于小规模、低写并发部署。不支持以内存数据库作为生产存储，不支持多个 API 实例或网络文件系统共享同一个 SQLite 文件。需要多副本、持续出现写锁超时、备份恢复窗口不可接受，或性能不能满足业务目标时，应迁移到 PostgreSQL。

项目采用 GORM 官方 SQLite 驱动并允许 CGO。应用统一启用外键、WAL、忙等待超时和偏向耐久性的同步策略。超过忙等待超时后，API 返回统一的暂时不可用错误，不向客户端暴露 SQLite 驱动错误，也不自动重放整个业务事务。

`database.driver` 为 `sqlite` 时，`database.url` 表示数据库文件路径，用户名和密码不需要且不得配置；连接参数由应用管理。PostgreSQL 的既有配置行为保持不变。

PostgreSQL 与 SQLite 使用各自显式的 Goose schema/seed migration 树。两套 migration 版本号锁步，同一编号表示同一个逻辑变更，只允许方言实现不同。API 进程不执行 migration，也不使用 GORM `AutoMigrate`。生产回退不承诺自动执行 migration `down`，而是恢复发布前备份和旧应用版本。

每次数据库相关改动必须同时通过 PostgreSQL 与 SQLite 集成测试。首次声明 SQLite 正式支持前，必须验证：PostgreSQL 从当前版本升级及现有集成测试、SQLite 空库 schema/seed、同等 API/Repository/事务行为、文件库重启持久性、两套 migration 版本一致，以及 SQLite 在线备份、恢复、恢复后 migration 与核心读取。

项目提供自包含的 SQLite 备份命令，并自动验证恢复结果。备份调度、保留策略、数据库和备份文件的访问控制由部署方负责。本次不提供 SQLCipher、跨数据库存量数据转换或 PostgreSQL 与 SQLite 双向迁移工具。

## Consequences

- 数据库改动的 CI 和 migration 维护成本增加，但兼容性退化会在合并前暴露。
- SQLite 可以用于正式的低成本单机部署，但其生产支持边界必须随部署文档明确呈现。
- SQLite 与 PostgreSQL 可以独立新建和升级数据库，但使用方不能假定存量数据可在两者之间直接切换。
- MySQL 不进入当前测试和正式支持矩阵；实现前仍不得宣称支持。

## Considered Options

- **SQLite 仅用于开发和测试**：维护成本更低，但不能满足将其用于正式部署的目标。
- **SQLite 与 PostgreSQL 运维能力完全等同**：文件型数据库无法合理提供共享多副本等 PostgreSQL 部署能力，因此拒绝。
- **共用一套 DDL 或对 SQLite 使用 `AutoMigrate`**：会隐藏方言差异或绕过版本化 schema 管理，因此拒绝。
- **本次同时支持 MySQL**：会扩大 migration 与集成测试矩阵，且没有当前需求，因此延期。
