# 多厂区与 Agent 配置、切换

适用范围：[多厂区与 Agent 互斥需求](../requirements/multi-site-discovery.md)。每个厂区当前一台地磅及独立工控机、打印机；使用一套 Server。真实仪表和摄像头尚未接入，当前真实 Agent 仅支持打印。

## 厂区配置页面与下载（2026-10-21 新增）

后台已提供“基础资料 → 厂区配置”（`/weighing/site-config`），可维护厂区及其地磅和 Agent，填写一次工控机可访问的 Server 地址，再重复下载对应厂区的 JSON 启动配置。真实打印 Agent 在本地维护台“配置”页导入、预览并确认保存草稿，再明确应用；本机队列、校准和打印账本保留。mock 可直接使用同一文件：

```sh
task agent:mock -- --config /path/to/SITE02-agent.json --listen 127.0.0.1:8091
```

先执行本轮 schema 00015、seed 00009 迁移，再更新 API。旧配置中尚未迁入的身份在升级启动时导入，后续以后台记录为准，不覆盖已有后台绑定；新增绑定对新连接即时生效。旧 YAML 和旧命令继续兼容，下面各节保留其配置方法，日常新增厂区无需再手改 YAML。`fcc-com-01` 已部署新版本，第二厂区主机尚未指定，准确结果见[本轮交付](../requirements/site-agent-configuration-delivery.md)。

## 数据升级和授权

1. 先备份数据库、Agent 图片目录及工控机打印尝试账本，保留原 Agent 的身份与密钥。
2. 执行 `task db:migrate`；SQLite 使用 `task db:migrate:sqlite`。新增 schema 00014 和 seed 00008，历史 migration 保留，API 启动不会迁移。
3. 初始厂区为 `SITE01 / 原厂区`、`SITE02 / 新增厂区`，原地磅仍为原 ID，新增 `SCALE02` 属于第二厂区。已有任务和单据归第一厂区；新增厂区无旧业务。执行本轮迁移及 API 更新后，可直接在“厂区配置”设置真实名称；不修改已发布 migration。
4. 已有用户保留原厂区查询／操作范围，既有动作权限仍生效；管理员初始获得两厂区。新用户在“用户管理 → 厂区”中设置“未授权／仅查询／查询与操作”，再按角色分配动作权限。新增踢出权限为 `weighing:agent:kick`，默认仅管理员具备。

厂区授权是实时服务端校验；跨厂查询不赋予操作权限。共享物料和往来主体沿用原维护权限。不要用部门字段代替厂区授权。

## Server 多 Agent 配置

将以下内容合并到忽略的本地配置中，例如 `server/configs/config.dev.yaml`，厂区地磅 ID 以数据库实际值为准：

```yaml
weighing:
  image_dir: /absolute/path/to/agent-images
  agents:
    - id: scale-agent-1
      scale_id: 1
      key: replace-with-existing-first-site-secret
    - id: scale-agent-2
      scale_id: 2
      key: replace-with-independent-second-site-secret
```

`agents` 非空时作为完整映射列表；列表外身份不接受。每个地磅和 Agent 编号各自唯一，密钥必须非空；两厂使用独立密钥。原厂区沿用现有有效身份和密钥，不直接复制示例值。旧的 `agent_id / scale_id / agent_key` 在未配置列表时继续支持单磅兼容。

也可通过 `APP_WEIGHING__AGENTS` 提供 JSON 数组，字段为 `id`、`scale_id`、`key`；实际值仅在受保护环境中配置，不写入命令历史或仓库。设置列表后需重启 Server。

## 各工控机与 mock

真实打印 Agent 继续使用各自受保护的 `agent.env`，分别设置 Server 地址、Agent 编号、地磅 ID、对应密钥、本机 CUPS 队列与校准参数，沿用[本地维护部署说明](agent-local-maintenance.md)。原现场身份及本地账本目录保持一致；第二厂区使用自己的身份和队列。镜像／二进制更新后需升级客户端才能响应 kick，旧客户端只能断开，后台不能保证其进程退出。

mock 仍是完整 Agent 的替代实现。每个实例使用独立控制端口及对应身份；有效本地配置含 `agents` 时按指定编号与地磅读取对应密钥，未显式选择时默认第一项。也可通过受保护环境 `APP_WEIGHING__AGENT_KEY` 覆盖，例如：

```sh
# 先在环境中安全设置 APP_WEIGHING__AGENT_KEY，再启动对应实例。
task agent:mock -- --server http://127.0.0.1:8900 --listen 127.0.0.1:8090 --agent-id scale-agent-1 --scale-id 1
# 第二厂区使用自己的密钥和另一控制端口。
task agent:mock -- --server http://127.0.0.1:8900 --listen 127.0.0.1:8091 --agent-id scale-agent-2 --scale-id 2
```

真实 Agent 和 mock 不可服务同一地磅并行分工。mock 在线时打印也由 mock 处理；当前真实打印 Agent 在线时采集能力仍不可用。用 mock 验证业务不能认定真实设备通过验收。

## 后台切换与退出

- 从“厂区工作台”进入授权厂区；不同厂区可打开各自窗口，同一地磅仍受操作会话占用规则控制。
- 已有 Agent 在线时，后来者收到 `agent_conflict`，原连接不受影响。需要切换时，停止正在反复尝试的非目标进程，在后台选定当前 Agent 踢出，再启动目标 Agent／mock。
- kick 立即使未确认采样失效；收到指令的 Agent 退出整个进程，不自动重连。真实客户端正常退出码为 0，发布的 `Restart=on-failure` 不立即拉起；自定义守护策略应保持同样行为。
- 后台显示收到 kick 确认及断连；如果未确认，按“已断开，退出未确认”处理并到现场核对，不把网络断开视为进程结束。
- 踢出不永久封禁身份。维护人员手动启动、明确应用需启动的配置或工控机重启后可再次接入；本地维护服务继续可用。
- kick 不取消已提交的业务或 CUPS 作业。纸张可能继续输出，无最终回执保留未知，先核实再按原补打规则办理；不自动重发或清空账本。

## 回退边界

回退旧版本前先停止两厂业务并备份。旧服务不具备厂区隔离，不能直接承接新增厂区业务；存在两厂同车牌未完成任务时，恢复旧的全局唯一索引会拒绝回退。seed 回退保留已经可能被引用的厂区／地磅身份，不删除业务。跨版本回退需同时核对 schema、seed、权限及客户端，不能只替换 API 二进制。
