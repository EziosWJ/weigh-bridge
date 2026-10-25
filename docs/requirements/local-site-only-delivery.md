# 单一本厂运行形态交付记录

> **版本交付证据**：本文保留 ADR 0028／0029 当时实现和验证事实（含失败），不是当前安装指令或完整状态表。最新目标见[总规格](standalone-site-spec.md)，剩余工作见[当前状态](../next.md)；本页原路径保留供发布包引用。

日期：2026-10-24。依据：[ADR 0028](../adr/0028-single-local-runtime-identity.md)和[当前规格](standalone-site-spec.md)。本轮完成本地实现、隔离软件验证及当前开发库切换；没有生产切换、真实设备重新配对、Git 提交或推送。旧双模式验证事实保留在[历史交付](../history/requirements/standalone-site-delivery.md)，不用于推定本版通过。

> 后续决策（2026-10-24）：[ADR 0029](../history/adr/0029-local-initialization-and-agent-connection.md) 已确认取消两人启用门槛、将连接地址移出本厂配置。以下仍记录 ADR 0028 原版实现及测试事实，不能证明新要求已实现；新要求的代码与软件验证见本页[补充交付](#adr-0029-补充交付2026-10-24)，现场验收仍待完成。

## 实现范围

- 删除 `central`、`site`、`standalone` 与空角色运行分支、节点复制任务、临时主体中心审批和配置下发接口。旧 `node` YAML／环境变量明确报退役错误；业务 API 拒绝静态 Agent 密钥及绑定配置，身份从本厂数据库读取。
- 通过新增 schema 17、seed 13 收敛数据库，历史 Goose 文件不改写。`wb_local_identity` 保存唯一归属、ID 分配计数与首次业务启用状态，复用本厂、地磅及绑定表。迁移和首次配置拒绝非空旧业务资料，避免把旧物料、主体或幂等结果归给新厂；旧业务库按授权删除并重建。
- 管理员网页「本地管理 → 本厂配置 → 首次配置」提交身份、编号及地磅绑定。事务失败全部回滚，并发首次提交只成功一次；身份配置后无需重启，固定编号和已绑定身份不可编辑，名称、地址与启停继续按权限维护。
- 将配置完成与首次业务启用分别表达；未设置本厂可登录管理，正式业务仍需两名实名、有称重权限和本厂操作授权的用户首次启用，后续沿用现有 JWT、RBAC、会话及动作权限。
- 保留设备独立进程和 Agent 持有配置。新增 `local-admin-init` 一次性设置生产管理员口令，新增 `agent-provision` 仅重配连接身份；已有文件须显式 `--update-connection`，保留设备参数、注释和未知配置行，以 0600 原子写入，不操作打印账本。Agent 启动保留待应用草稿和回退参数，并同步当前连接身份，防止应用草稿时恢复旧密钥。
- 开发 mock 支持从配对生成的 `agent.env` 显式读取连接身份，不再从业务 YAML 读取共享密钥；保留 JSON 连接文件与显式 mock 凭据，文件与身份覆盖参数互斥。
- 单一本厂前端删除旧节点、同步及临时主体页面和分支，保留资料编辑、称重、权限及停用保护。开发、发布、安装与冒烟入口统一采用 `local:*`，详见[部署说明](../device/local-deployment.md)。

## 软件验证

| 检查 | 结果与范围 |
| --- | --- |
| `task check` | 通过开发入口测试、Go test/vet、前端 lint/build |
| `task db:check` | 通过完整隔离 PostgreSQL 集成；包括身份初始化回滚／并发／锁定、两独立库同车称重及编号、快照、查询、权限、维护、恢复及迁移边界；临时容器结束清理 |
| `task build:check` | 通过内嵌 Web 发布构建及 embedweb test/vet；前端嵌入资源与最终 dist 文件一致 |
| 前端页面回归 | `test:local-management`、`test:menu-order`、`test:business-layout`、`tests/site-configuration-smoke.mjs` 通过；覆盖无本厂身份的首次设置、配置／首次业务启用分离、重复拒绝、只读权限、身份锁定、Agent 接入及停用、390px 窄屏和业务列表布局 |
| `test:weighing:backend` | 通过真实 API 新库初始化、HTTP 创建两名实名操作员及授权后完成两方向称重、不可变快照、纠错重开、异常重量、打印未知结果、操作权接管及 Agent 踢出 |
| 设备重新配对单测 | 通过仅替换连接字段、保留设备和未知参数；已有维护草稿／回退参数保留、旧连接身份替换和重启持久化 |
| 本地安装与发布冒烟 | 最终版本 `local-runtime-20261024-r1` 的安装和发布冒烟通过；覆盖临时目录安装／升级／校验，以及临时 PostgreSQL 上生产未 bootstrap 拒绝启动、重复 bootstrap 拒绝、网页本厂初始化、配对与参数保留、API 重启和证据／未知账本保留 |

旧空角色模式中限制变更已引用主体业务身份的断言已按有效快照规则替换：资料变更用于新任务，已首磅任务及后续单据继续沿用快照；新任务不能使用不具供货身份的主体，删除已引用主体仍被拒绝。历史 migration 的整体回退不用于单厂切换；schema 17 明确拒绝原地重建已退役身份，恢复须使用匹配备份。

## 当前开发环境

已核对运行中的 API 属于本项目，目标为本机 `127.0.0.1:5432/weighbridge`，库中无称重交易、单据及未决打印。按用户授权重建开发库并显式迁移至 schema 17／seed 13；更新忽略的开发配置，移除静态绑定并使用本机上传／图片路径，数据库连接和 JWT 保持原值。

新版 API 已在 8900 启动，`/ready` 通过，现有 Web 5173 可访问。浏览器实测登录后显示「本厂配置 → 首次配置」，未替用户提交本厂身份。根目录 `agent.env` 和已有状态文件指纹核对未变化；未启动、停止或重新配对真实设备，不重放任何称重或打印。

## 仍需现场验收

本版软件和隔离测试不替代 D12 非零带载、稳定灯对照、串口重启权限、真实摄像头／识别、连续五联纸及完整现场闭环。当前开发库尚待管理员填写真实本厂身份，真实设备连接须随后显式配对并核对生效；生产部署及旧维护进程退役仍按现场步骤验收。

本地发布产物：`bin/weighbridge-local-runtime-20261024-r1-linux-amd64.tar.gz`。验证后仅补录交付文档并重新归档，五个二进制的 SHA-256 与已通过冒烟的版本一致。


## ADR 0029 补充交付（2026-10-24）

依据 [ADR 0029](../history/adr/0029-local-initialization-and-agent-connection.md) 与[补充规格](../history/requirements/local-initialization-simplification-spec.md)。本节记录本轮增量实现与验证，不改写上文 ADR 0028 原版事实；旧交付中的“两人启用”记录仍属于当时实现，不代表本次已验证。

### 实现

- 本厂身份、地磅与绑定首次事务成功即初始化完成。`local-status` 与 capabilities 保留 `{siteId, configured, initialized}` 字段，`configured`／`initialized` 同时表达配置完成；读取状态不再查询人员或写 `initialized`。旧库 `initialized=0` 不再阻止业务，列保留但不参与判断，未新增 migration。
- 首次配置与编辑忽略兼容字段 `serverUrl`，不再写入 `wb_site.server_url` 和 `wb_agent_setting.server_url`。旧 `/agent-settings` 路由保留但拒绝写入并提示使用配对工具。
- `agent-provision` 新增 `--server-url`：首次必须提供有效地址；更新已有文件未传地址时保留已有有效值，显式传入才替换；不再读取 `wb_site.server_url` 或 `AgentSetting.server_url`。保留 `--update-connection`、0600、原子写入、敏感信息不输出，以及设备参数、维护草稿、未知行和打印账本。
- 旧连接导出接口 `/site-config/:id/connection` 保留路由与鉴权，但要求请求显式提供 `serverUrl`；缺失或无效时返回明确业务错误，不再回退数据库地址。
- 本厂页面移除 Server 地址与两人提示，显示“配置状态：已配置／未配置”并明确不代表设备就绪。设备维护页展示 Agent 实际连接地址（只读）及同机回环／分机局域网含义；地址修改说明通过配对工具并显式应用／重启，不新增自动发现或远程离线配对。

### 软件验证

| 检查 | 结果与范围 |
| --- | --- |
| `task backend:check` | 通过 Go test 与 vet |
| `task frontend:lint`、`task frontend:build` | 通过 |
| `task build:check` | 通过内嵌 Web 发布构建及 embedweb test/vet |
| 完整隔离 PostgreSQL 集成 | 通过；覆盖空库拒绝正式写入、首次不填地址、失败回滚、8 路并发仅一次成功、重复拒绝、旧 `initialized=0` 兼容、单名已授权账号完成正式称重（既有两方向闭环继续通过）、无权限／无操作范围／停用账号拒绝、连接导出显式地址、维护不写旧地址与旧启用标记 |
| `web/tests/site-configuration-smoke.mjs` | 通过；首次配置无 Server 字段、配置即完成、编辑无地址、显式地址导出连接、停用保护、390px 窄屏 |
| `web/tests/local-management.mjs` | 通过；本厂页面无地址／两人提示、只读与加载失败、维护配置展示实际连接地址且不可编辑、390px 窄屏 |
| `web/tests/weighing-backend-smoke.mjs` | 本轮在 31 秒旧采样确认断言处失败：设备 revision 因状态接收间隔超过 3 秒而递增，前端按既有规则清除采样。该路径不涉及本次初始化／地址改动，属当前工作区既有的时序敏感断言；未据此宣称通过，其余本厂页面与单人称重闭环由集成测试及上述页面冒烟覆盖 |
| 打包安装／发布冒烟 | 通过；使用 `local-init-20261024-r2` 临时包和隔离 PostgreSQL，覆盖首次配对缺 `--server-url` 拒绝、有效地址写入、更新保留与显式替换地址、设备参数／未知行／账本保留、重启保留身份与证据 |

### 未完成与限制

真实 D12 带载、稳定灯对照、摄像头上传、连续打印、真实分机地址和 Agent 离线配对仍需现场验收；本轮未操作真实业务库或真实设备，未提交或推送 Git。`weighing-backend-smoke.mjs` 的时序失败未修复，留待现有本厂工作区一并处理，不用其他检查结果替代。
