# 01：主机 CPU/内存监控与 ADMIN 入口

GitHub：[#65](https://github.com/EziosWJ/base-project-golang/issues/65)；状态：ready-for-agent。

## Parent

[Spec #64](https://github.com/EziosWJ/base-project-golang/issues/64)

## What to build

在原生 Linux 部署中，ADMIN 可以从“系统监控 → 服务器监控”打开真实页面，看到主机信息、总体/逐核 CPU、系统负载、内存与 Swap，以及各区域的有效采集时间和状态。后台每 5 秒持续采样，页面关闭后采样仍运行。这个切片贯通真实主机采集、授权、API、双数据库菜单种子与页面，不等待磁盘、网络或 GPU 才能演示。

## Acceptance criteria

- [ ] 新增系统监控目录与服务器监控菜单，PostgreSQL/SQLite seed 版本锁步、可重复执行，ADMIN 角色关联正确。
- [ ] ADMIN 可打开菜单、页面和版本化只读概览接口；普通角色及未登录请求通过真实路由验证为 403/401，直接访问不能绕过授权。
- [ ] 角色授权读取当前启用且未删除的 ADMIN 关系，覆盖管理员角色撤销或禁用后的访问拒绝。
- [ ] 返回并展示主机名、系统版本、运行时长、CPU 型号、核心数、总体/逐核占用、1/5/15 分钟负载、内存总量/已用/可用/占用率及 Swap。
- [ ] 总体 CPU 以整机为 100%，逐核单独计算；内存按总量减可用量，未配置 Swap 明确标识。
- [ ] 每 5 秒采样，页面刷新读取已采集数据；页面关闭后继续采样，同一任务不重叠且可取消退出。
- [ ] 各区域独立返回有效时间及正常、不支持、采集失败等状态；超过 15 秒无有效新样本标为过期，缺失值不填零。
- [ ] 其他正式未支持平台显示支持状态，不使用模拟资源冒充主机数据。
- [ ] 以可控采集输入和时间在真实 HTTP 路由验证权限、正常与部分失败/过期；沿用 Playwright 验证真实页面和轮询清理。
- [ ] 双数据库菜单契约、后端检查、前端 lint/build 通过；更新 API 文档和原生 Linux 启动验证说明。

## Blocked by

- None (can start immediately).
