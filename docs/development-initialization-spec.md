# 本地开发初始化入口

## Problem Statement

使用脚手架启动新项目时，开发者仍需手动复制后端配置、生成 JWT 密钥、调整上传目录并逐项排查工具与数据库环境。失败通常在迁移或启动阶段才显现，增加首次使用成本；已有配置还需要避免被重复初始化覆盖。

## Solution

提供统一的 `task init` 与 `task doctor`。前者生成本地开发配置，默认使用 PostgreSQL，也允许选择 SQLite；后者检查开发工具、配置与数据库准备情况，给出修复步骤。用户随后通过已有的开发入口执行迁移并启动前后端。

## User Stories

1. As a new project developer, I want one initialization command, so that I can prepare development configuration without manually copying a template.
2. As a new project developer, I want PostgreSQL selected by default, so that initialization follows the existing database convention.
3. As a new project developer, I want to select SQLite explicitly, so that I can start without an external database.
4. As a new project developer, I want a randomly generated JWT secret, so that I do not need to invent or reuse a template secret.
5. As a new project developer, I want the upload directory resolved for my checkout, so that configuration works on my machine.
6. As a new project developer, I want SQLite configuration without PostgreSQL credentials, so that the existing configuration validation passes.
7. As a returning developer, I want initialization to preserve my existing configuration and JWT, so that rerunning it does not disrupt my environment or sessions.
8. As a developer, I want an independent environment check, so that I can diagnose failures without regenerating configuration.
9. As a developer, I want missing Go, Node.js, npm, Task and C compilation tools identified, so that I know which prerequisites to install.
10. As a developer, I want missing frontend dependencies identified, so that I install them before starting the frontend.
11. As a developer, I want configuration checked using the application's existing rules, so that doctor and startup agree about valid configuration.
12. As a PostgreSQL user, I want connection checks to have a deadline, so that an unavailable database does not hang environment diagnosis.
13. As a PostgreSQL user, I want database preparation guidance when connection fails, so that I can prepare the database and credentials myself.
14. As a SQLite user, I want an absent database reported as awaiting explicit migration, so that initialization does not create database contents implicitly.
15. As a developer, I want credentials omitted from diagnostics, so that I can share diagnostic output safely.
16. As a developer, I want checks to report failure through exit status, so that automation can detect blocking problems.
17. As a developer, I want initialization to show the matching existing startup command, so that I can complete migration and startup without learning a second workflow.
18. As a developer, I want PostgreSQL and SQLite profile overrides respected, so that diagnosis checks the configuration my chosen startup command uses.
19. As a developer, I want existing database contents protected during diagnosis, so that checking an environment does not run migrations or seed data.
20. As a maintainer, I want documented first-start steps and command-level tests, so that future changes preserve the onboarding behavior.

## Implementation Decisions

- Extend the root Task development entry points with `init` and `doctor`, passing explicit command arguments through the existing Task convention.
- Provide an explicit database selection argument for initialization; the default is PostgreSQL. This keeps initialization repeatable in both terminal and automated use.
- Follow the existing Node maintenance-script pattern for prerequisite checks before compiling the Go helper. Task reports a missing Node runtime before invoking the script.
- Keep configuration generation and validation in a small Go command, using the existing YAML parser and configuration loader; add no runtime service or new third-party dependency.
- Generate development configuration from the versioned development template. Generate a cryptographically random JWT secret and resolve the project-local upload directory into an absolute path.
- Create actual development configuration exclusively; an existing file is preserved, including its JWT. Configuration containing credentials remains excluded from version control.
- Keep database readiness checks in the database infrastructure boundary, with a finite timeout and sanitized diagnostic messages. Do not expose raw configuration parsing or database errors that may contain credentials.
- PostgreSQL checks connectivity to the configured database without supplying it. SQLite checks must not create an absent database or run migrations; absence is a pending-migration notice rather than an external-service failure.
- Respect existing environment and SQLite profile precedence, and tell users which startup task matches the effective configuration.
- Initialization finishes with instructions, rather than automatically installing dependencies, migrating, starting services, creating a PostgreSQL database or starting Docker.
- No schema changes, migration changes, REST API changes, runtime configuration precedence changes or project renaming.

## Testing Decisions

- The user confirmed the command entry point as the primary test boundary. Assert generated configuration, preserved file contents, exit status and visible diagnostic output; avoid tests of template assembly internals.
- Cover clean PostgreSQL and SQLite initialization, absolute upload paths, non-template JWT secrets, existing configuration protection and invalid command arguments.
- Cover missing prerequisites before Go compilation, configuration validation failures, PostgreSQL connection failure within a deadline, SQLite pending migration, existing SQLite checks and output that omits credentials.
- Use temporary checkout/configuration fixtures and temporary SQLite files. Never connect automated tests to actual deployment credentials or modify the developer's real configuration.
- Prior art: existing Go migration and backup commands expose a command runner, and configuration tests already use temporary configuration directories and verify SQLite profile precedence.
- Run backend tests and static checks through the existing Task entry point. Verify the maintenance script with Node's built-in test runner, requiring no extra test dependency.
- Verify explicit migration and bounded API/frontend startup using isolated configuration and storage. Check API health and database readiness, then clean up started processes. PostgreSQL verification requires an available isolated database; report an environmental limitation if it cannot be provided.

## Out of Scope

- Project branding or Go module renaming.
- Production configuration generation or deployment provisioning.
- Automatic Docker startup, PostgreSQL database creation, dependency installation, migration or service startup by init/doctor.
- Overwriting existing configuration or rotating an existing JWT secret.
- New database drivers, migration/schema changes or business features.
- Changing existing database support boundaries or replacing the established startup tasks.

## Further Notes

The agreed effort target is 1–3 days. The feature follows the accepted YAML runtime-configuration and SQLite support decisions. It needs no new ADR or generic development-tool glossary entries. Initialization cannot make an unprepared PostgreSQL database ready; clear preparation guidance is part of the result.
