# Database migrations

Server supports PostgreSQL only. `schema/` contains tables, indexes and constraints;
`seed/` contains built-in data. Keep both historical paths and version numbers.
Run schema before seed using `task db:migrate`; each stream uses its own Goose
version table (`goose_schema_db_version` and `goose_seed_db_version`).

The API never runs migrations or AutoMigrate. Add a new migration for schema
changes and validate with `task db:check` against isolated PostgreSQL containers.
Retired SQLite SQL is preserved in [the historical archive](../../docs/archive/sqlite-migrations/README.md),
not a supported migration stream and not subject to version lockstep.
