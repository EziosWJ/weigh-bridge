# 通知与公告使用及开发说明

对应 [Issue #73](https://github.com/EziosWJ/base-project-golang/issues/73)。设计依据为[实施规格](notification-enhancement-spec.md)、[ADR-0007](adr/0007-atomic-site-notifications.md)及 [ADR-0012](adr/0012-notification-realtime-boundary.md)。

## 使用入口

- 登录页展示有效公开公告，关闭公告栏后仍可通过“查看公告”阅读；关闭只影响当前标签页本次访问。
- 登录后的公共布局展示公开公告和站内公告；“公告”页面按范围分页查询当前有效公告。
- ADMIN 通过“公告管理”保存草稿、编辑草稿、确认发布、撤下和查看历史正文。公开公告的发布确认明确提示未登录用户也可查看；已发布正文不可编辑。
- “我的通知”和铃铛保留个人收件箱与未读数。预览不算已读，打开详情自动已读，详情可继续前往有权限的站内业务页面。
- 仅获得焦点且可见的页面给新通知轻提示，其他页面静默更新。断线重连只恢复完整状态，不补弹离线通知，不使用消息轮询。

## API 契约

REST 沿用统一的 `{code, message, data}` 响应；参数错误与不存在的资源分别为业务码 400、404，HTTP 状态保持既有兼容约定的 200；未登录和无管理权限分别返回 HTTP 401、403。页码默认 1、页大小默认 10，最大 500。

| 方法与路径 | 受众 | 行为 |
| --- | --- | --- |
| GET /api/public/announcement/page | 匿名及登录用户 | 有效公开公告分页 |
| GET /api/public/announcement/{id} | 匿名及登录用户 | 有效公开公告详情 |
| GET /api/system/announcement/page | 登录用户 | 有效站内公告分页 |
| GET /api/system/announcement/{id} | 登录用户 | 有效站内公告详情 |
| GET /api/system/announcement-admin/page | ADMIN | 草稿、发布、到期、撤下的管理历史分页 |
| POST /api/system/announcement | ADMIN | 保存新草稿，返回公告 |
| PUT /api/system/announcement/{id} | ADMIN | 编辑草稿，返回公告 |
| PUT /api/system/announcement/{id}/publish | ADMIN | 立即发布未到期草稿 |
| PUT /api/system/announcement/{id}/withdraw | ADMIN | 撤下已发布公告 |
| GET /api/public/announcement/events | 无需登录 | 公开公告变化 SSE |
| GET /api/system/notification/events | 登录用户 | 本人通知、已读及站内公告变化 SSE |

公告草稿输入：`title`、`content`、`scope`（PUBLIC / INTERNAL）、可选 `expiresAt`（RFC3339，null 表示长期有效）。标题最多 200 字符，正文最多 100,000 字节；前端编辑器限制为 5,000 字符。到期时间必须在未来。公告阅读没有逐人已读或签收状态，发布者账号标识不暴露在公告 JSON 中。

原通知接口保持兼容，手动发送和通知记录增加可选 `jumpPath`，最多 1,000 字节，只接受本系统路径，拒绝站外地址、脚本协议与反斜杠路径。管理写操作继续写入事务内审计。来源支持 MANUAL、ROLE_CHANGE、BUSINESS。

## SSE 与恢复

响应为 `text/event-stream`，每帧为 `data: {"type":"...","id":...}` 加一个空行。帧中不包含消息正文或接收人列表。

- ready：订阅已经建立，客户端查询当前状态；首次打开和每次重连都查询。
- announcement：重新查询对应范围的有效公告，并复核已打开的详情。
- notification：重新查询个人通知和未读数，id 用于本次连接生命周期内的提示去重。
- read：重新查询个人通知和未读数，不弹新消息提示。
- heartbeat：连接心跳，不查询消息。

浏览器以 fetch 流读取支持既有 Authorization Bearer 请求头，不把 token 放入 URL。匿名与登录流分别建立，登录身份失效不影响公开公告。

服务器在查询快照前建立订阅，客户端收到 ready 后加载；同步期间的新变化会继续触发刷新，旧请求被取消，避免旧结果覆盖新结果。首次建连失败时允许一次初始 REST 读取，用户仍可查看已有消息；持续重试不会重复查询消息，重连收到 ready 后重新获取状态。连接重试采用 1 秒起步、最多 15 秒的退避，不提供轮询降级。正常查询失败显示错误或允许手动刷新。

服务端每次发送个人事件前重新认证，并每 15 秒随心跳检查撤销状态；JWT 到期通过单次计时器关闭连接。闲置连接的会话撤销至迟在下一次心跳发现，发送新事件时立即检查。客户端登录态变化直接取消旧流并清除个人页面；重新建立连接收到 401 时清理登录态。

每个连接的待发队列最多 32 个变化提示；慢连接队列满后结束订阅，客户端重连并恢复状态。单次写操作有 5 秒截止时间，流路由单独解除普通 HTTP 请求的总写超时，不更改其他请求超时。API 停机先关闭消息 Hub，再执行 HTTP shutdown。

到期更新使用公告截止时间的单次计时器，不扫描轮询：流建立、公告变化后查询下一到期时间并安排计时；页面也按已知到期时间隐藏内容，接口始终过滤失效公告。API 重启后的新连接重新加载下一到期时间。

## 业务接入

在业务 Repository 注入应用组装阶段创建的同一个 notification.Repository；它已由通知 Service 绑定实例内 Hub。业务 Service 不接触 Gin 或 GORM。

在业务持久化事务内调用 `RecordBusiness(ctx, tx, PublishInput{Title, Content, UserIDs, JumpPath})`。指定用户必须为启用且未删除账号；任何通知写入错误返回给事务，业务与通知、审计共同回滚。该入口不接受 AllUsers，群发由管理员发送入口负责。

返回值为提交后回调与错误。保存回调，只有整个业务事务成功返回后才调用，事务失败时丢弃。回调只推送变化，不再写数据库，也不会返回错误或撤销已提交业务。

```go
var notifyAfterCommit func()
err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    // 在这里保存业务变化。
    var err error
    notifyAfterCommit, err = r.notifications.RecordBusiness(ctx, tx, notification.PublishInput{
        Title: "订单审核通过",
        Content: "订单已通过审核，请查看详情。",
        UserIDs: []int64{recipientID},
        JumpPath: "/orders/123",
    })
    if err != nil {
        return err
    }
    return audit.RecordOn(ctx, tx, auditEvent)
})
if err != nil {
    return err
}
notifyAfterCommit()
return nil
```

该片段示意业务 Repository 的事务编排，业务变化与审计事件由接入项目提供。角色分配已按同样的提交后回调规则接入共享消息持久化入口；相同角色集合重复保存不产生通知。角色分配沿用既有禁用账号可调整角色的行为，产生的角色通知保留至账号重新启用后查看，禁用账号不在线接收。业务重试是否需要新消息由业务本身判断，不另建去重平台。

## Migration 与开发入口

- 00008：增加公告表和通知站内跳转字段。
- 00009：增加 BUSINESS 来源；SQLite 重建相关表并复制既有通知及接收、已读记录，PostgreSQL 修改来源约束。SQLite 历史保留由真实迁移及 HTTP 契约验证。回退 00009 时 BUSINESS 来源映射为 MANUAL；回退 00008 会删除新增公告及跳转字段，因此不要在生产中直接回退该新增能力。
- 开发升级执行 `task db:migrate`；SQLite 配置可使用 `task db:migrate:sqlite`。API 不自动迁移。
- `task backend:check`：后端测试与 go vet。
- `task backend:notifications`：两种数据库公告、通知、SSE、回滚、迁移与容量专项契约，需要 Docker、Node、前端依赖和 Playwright 浏览器。
- `task db:integration:sqlite`：完整 SQLite 集成契约；新增容量契约同样需要 Node 与 Playwright。
- `task backend:integration`：完整 PostgreSQL 集成契约。
- `task frontend:notifications`：公告与通知浏览器行为验证。
- `task frontend:lint`、`task frontend:build`：前端静态检查及构建。
- `task backend:swagger`：使用固定版本 Swag 生成提交的 Swagger。

所有集成测试使用临时隔离数据库与临时 HTTP 服务。容量测试的 100 条连接包含 98 个 HTTP 流客户端与一个浏览器中的公开、登录两条流，群发接收人数从管理 HTTP 接口核验为 1,000；同时测量协议到达与真实页面更新，不将协议到达当作页面已更新。测试输出 `/tmp/notification-capacity-{sqlite,postgres}.json`，Vite 使用隔离缓存并清理测试进程。

## 本次验证记录（2026-10-05）

- `task check` 通过：后端测试、go vet、前端 lint 与生产构建；构建保留既有包体积提示。
- `task backend:integration`、`task db:integration:sqlite` 和 `task backend:notifications` 通过，覆盖 PostgreSQL / SQLite 公告范围、管理权限、个人已读隔离、事务回滚、提交后推送、会话退出及 JWT 到期、公告到期、业务通知与历史迁移保留。
- `task frontend:notifications` 通过，覆盖匿名公告、关闭后仍可阅读、同标签页登录后关闭状态保留、新公告提示、失效详情移除正文、通知已读及跳转、后台静默更新、ADMIN 权限、移动端布局、退出关闭个人流、初始 SSE 失败时可读取消息、重试不形成消息轮询。
- 100 条 SSE 连接、1,000 个启用账号群发，在 Linux amd64、Go 1.26.5、Chrome 153、本机 HTTP 无代理环境下测得：PostgreSQL 页面更新约 84 ms，SQLite 约 46 ms；协议最慢到达分别约 35 ms、7 ms。页面更新时间从群发成功计至真实 DOM 更新，两者均通过 2 秒目标。此结果只代表本次验证环境。
- 本地 PostgreSQL 已创建 `local_project` 数据库和 `base_project_golang` schema，由专用非超级管理员开发账号持有；实际 `config.dev.yaml` 已更新且继续由 Git 忽略。`task db:migrate` 的 schema、seed 均成功，本地 API `/health`、`/ready` 均返回 200，验证进程已停止。未将管理员凭据写入仓库或开发配置。
- Swagger 已重新生成；`git diff --check` 通过。当前环境未安装 golangci-lint，未运行该额外检查。

## 部署

首版仅承诺一个 API 实例，继续使用 PostgreSQL 或 SQLite，不依赖 Redis、MQ。多个 API 实例之间没有跨实例事件传播，不能直接据此承诺多实例实时一致性。

反向代理对两个 SSE 路径关闭响应缓冲和缓存，读超时应大于 15 秒心跳间隔，建议 60 秒以上；支持持续流式响应。应用提供 `X-Accel-Buffering: no`，代理仍应明确配置对应路由。登录流的 Authorization 请求头必须转发，跨域部署应精确配置允许来源；保持现有 Bearer 认证约定。

例如 Nginx 针对 SSE 路径配置 `proxy_buffering off`、`proxy_cache off`、`proxy_read_timeout 60s`；其余请求维持原有配置。部署前执行 Goose migration，通过 `/health` 与 `/ready` 验证存活与数据库就绪，再验证连接长于普通 15 秒写超时仍能收到新变化。

容量基线为单实例、100 条并发连接、最多 1,000 个启用账号；它是验证规模，不是产品硬上限或任意部署规模的性能承诺。部署环境、代理和浏览器不同，应重新验证 2 秒更新目标。
