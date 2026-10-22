# 本地开发初始化

当前 Server 开发、数据库测试和生产统一使用 PostgreSQL，见 [ADR 0024](adr/0024-postgresql-only.md)。

1. 准备 Go 1.26、Node.js 24/npm、Task 与 PostgreSQL；集成和浏览器验收需要 Docker。
2. 执行 `npm --prefix web ci` 和 `task init`，生成忽略的 `server/configs/config.dev.yaml`。
3. 填写 PostgreSQL 连接、账号和密码，并预先创建目标开发库。
4. 执行 `task doctor`，再执行 `task dev` 显式迁移并启动 API 和 Web。

初始化生成独立的随机 JWT、Agent 密钥与绝对上传目录；保留已有配置和已配置密钥。doctor 检查工具、有效配置与数据库连接，连接检查限时 5 秒，错误输出不包含凭据；不会创建数据库、启动 Docker、迁移或修改业务数据。CGO 和 C 编译器不再是 Server 开发前置条件。

SQLite profile、启动任务和备份命令已退役；`--database sqlite` 和非空 `APP_CONFIG_PROFILE` 明确报错。已有本地配置不会被初始化自动转换，需要按 PostgreSQL 模板更新；旧库文件保留。普通测试使用临时配置目录，数据库验收由 `task db:check` 创建并清理临时 PostgreSQL 容器，不连接真实部署库。

行为规格见 [初始化规格](development-initialization-spec.md)，本次验证见 [数据库收敛交付记录](requirements/postgresql-only-delivery.md)。
