# 本厂组件安装、升级与恢复

适用当前 ADR 0030 单厂形态：本地业务服务（内嵌 Web）、设备 Agent、PostgreSQL，摄像头与视频服务由 Server 管理。默认三者在同一台主机，也支持 Server-only 与 Agent-only 分机安装。本文命令供 Linux amd64 的专用普通用户执行，不表示已部署到现场；真实仪表、摄像头和连续纸仍需现场验收。

## 组件与准备

| 组件 | 运行内容 | 主机准备 |
| --- | --- | --- |
| Server | 业务 API、内嵌 Web、摄像头取帧与图片保存、维护通道 | PostgreSQL 数据库及独立账号；RTSP 抓拍需要 PATH 中的 FFmpeg；图片与上传目录 |
| Agent | 仪表串口采集、打印执行、打印账本、本机维护 | CUPS 客户端工具（`lp`、`lpstat`、`cancel`）、实际打印队列、串口读写权限、Python 3 与打印渲染依赖、systemd 用户会话及 linger |
| MediaMTX | 浏览器 WebRTC 预览与流管理 | 可独立部署在本厂任意可达主机；安装器不下载、不启动，也不修改其全局网络 |

Agent-only 只要求 Agent 主机可访问的 Server 地址，不需要业务数据库凭据，也不需要拷贝 Server 配置。数据库连接、JWT、图片目录等启动参数只属于 Server 主机，由安装工具生成。

准备过磅单打印标题、Agent 主机可访问的 Server 地址和操作员资料。安装时需准备总部或实施方预先分配的厂区编码；地磅编码默认 `WB01`。Agent 不要求填写厂区／地磅编号或连接密钥。

构建机执行 `task local:package VERSION=<唯一版本>`，得到 `bin/weighbridge-<版本>-linux-amd64.tar.gz`。发布包带 SHA-256 manifest 和组件清单；安装机无需 Node。

## 首次安装

1. 准备独立空 PostgreSQL 数据库及持久目录。已有单厂实例走升级流程，先备份业务库、图片、配置及账本，保留现有数据。
2. 解包发布文件，按部署形态选择一种：

   **默认同机（Server + Agent）**

   ```sh
   export WEIGHBRIDGE_DATABASE_URL='postgres://user:password@127.0.0.1:5432/weighbridge'
   bash install.sh "$HOME/weighbridge"
   ```

   **只安装 Server**（数据库在 Server 主机或可达位置）

   ```sh
   export WEIGHBRIDGE_DATABASE_URL='postgres://user:password@127.0.0.1:5432/weighbridge'
   bash install.sh --component server "$HOME/weighbridge"
   ```

   **只安装 Agent**（分机；不读取数据库凭据）

   ```sh
   bash install.sh --component agent --server-url http://192.168.1.10:18900 "$HOME/weighbridge"
   ```

   数据库账号密码可写在 `WEIGHBRIDGE_DATABASE_URL` 中，也可单独设置 `WEIGHBRIDGE_DATABASE_USERNAME`／`WEIGHBRIDGE_DATABASE_PASSWORD`。安装器校验二进制与 manifest，生成随机 JWT 和受限配置，建立持久目录；已有配置、数据及 unit 不被静默覆盖，不自动迁移或启动服务，也不创建 `current`。
3. 组件安装完成后核对依赖检查结果。Agent 侧缺少 CUPS 工具或指定队列会明确报错；串口当前不存在只提示，设备接入后在维护页填写实际路径。隔离测试可用 `--skip-dependency-check`，生产安装不使用该参数。
4. 仅 Server（或同机）执行迁移。进入 `$HOME/weighbridge/releases/<版本>`：

   ```sh
   APP_ENV=prod ./bin/weighbridge-migrate up --kind all
   ```

   数据库 schema／seed 使用显式 Goose 迁移，API 不自动建表。Agent-only 没有迁移二进制，也不连接数据库。
5. 仅 Server（或同机）受控输入 `WEIGHBRIDGE_ADMIN_PASSWORD`（12..72 字节），执行 `APP_ENV=prod ./bin/weighbridge-local-admin-init --config-dir ../../configs`。此命令仅初始化 seed admin 密码、撤销相关会话并写入防重复 marker；生产 API 在 marker 存在前拒绝启动。
6. 激活 Server：

   ```sh
   cd "$HOME/weighbridge"
   ln -s releases/<版本> current
   systemctl --user daemon-reload
   systemctl --user enable --now weighbridge-business.service
   curl --fail --max-time 5 http://127.0.0.1:18900/health
   curl --fail --max-time 5 http://127.0.0.1:18900/ready
   ```

   `/health` 只证明存活，`/ready` 检查数据库。已有 `current` 链接或 unit 时先核对，不覆盖现场配置。
7. 管理员登录后在「系统管理 → 本厂身份」填写总部预分配的厂区编码（地磅编码默认 `WB01`）完成首次身份配置，编码和名称可维护；保留 seed 管理员初始本厂范围。使用「过磅单标题」页面编辑并预览样张；再按原流程分配账号权限与配置设备。本次使用空数据库初始化，不自动清理已有库或导入旧账本。上线后恢复备份时先核对已发单号并在「流水号水位」设置安全水位，再恢复出单。
8. Agent（或同机）核对 `agent/agent.env`。安装器已生成只含连接地址和本机设备默认值的文件，权限 0600：

   ```dotenv
   WEIGHBRIDGE_SERVER_URL="http://127.0.0.1:18900"
   WEIGHBRIDGE_PRINT_QUEUE="Aisino-SK820-raw"
   WEIGHBRIDGE_SERIAL_PORT=""
   WEIGHBRIDGE_SERIAL_BAUD=9600
   ```

   `WEIGHBRIDGE_SERVER_URL` 从 Agent 主机视角填写：同机可用回环地址，分机须为 Server 的局域网 IP 或域名。串口、波特率、打印队列与校准在 Agent 维护页保存并应用，日常不再手写配置。需要更新已有文件的连接地址时：

   ```sh
   ./releases/<版本>/bin/weighbridge-agent-provision \
     --agent-env-path "$HOME/weighbridge/agent/agent.env" \
     --update-connection --server-url http://192.168.1.10:18900
   ```

   该工具不再连接业务数据库；`--config-dir` 仅为旧调用方兼容而保留。更新保留串口、打印参数、注释、未知行和打印账本，并清理废弃的身份、密钥与相机字段。旧相机源地址会保存到同目录 `agent.env.legacy-camera.json`（0600），必须在本厂「摄像头管理」重新填写，不能从旧地址推测凭据。
9. 启用并启动 Agent，核对维护页的设备状态与运行配置摘要：

   ```sh
   systemctl --user daemon-reload
   systemctl --user enable --now weighbridge-agent-printer.service
   ```

10. 按厂方实际人员安排，通过本厂账号管理维护操作员姓名，并按 RBAC 授予称重权限和本厂操作范围。一名已授权人员即可办理，不设全厂人员数量启用条件。

## 摄像头、视频与识别

摄像头列表、RTSP 源地址与凭据、抓拍用途、统一超时、识别接口和 MediaMTX 管理／WHEP 入口均在 Server 的「摄像头管理」页维护，保存后用于后续采集；不再由 Agent 配置或重载。Server 直接连接摄像头取帧并保存图片，历史图片保存当时名称、来源和摘要。Agent 离线只阻止依赖仪表的正式采集，不阻止网络摄像头维护、测试和预览。

MediaMTX 可部署在本厂任意可达主机，以原生二进制配合 [mediamtx.service](../../deploy/mediamtx.service) 运行。发布包提供 [mediamtx.yml.example](../../deploy/mediamtx.yml.example)，复制到私有目录并替换占位地址；安装器不自动启动它，媒体主机也不需要 FFmpeg。Server 通过 Control API 同步本系统托管路径，部署文件重建或远端重启后由 Server 轻量核对并补回。浏览器必须能访问配置的 WHEP 入口，Server 必须能访问管理接口。

部署文件不设 MediaMTX 账号密码，控制面靠网络隔离保护：9997 只允许业务 Server 主机访问，该接口能读回相机源地址，不能对整个局域网开放；媒体端口只在本厂可信网段开放。上线前核对：

- `webrtcAdditionalHosts` 填浏览器可达的媒体主机 IP，不能是 `0.0.0.0`。
- `webrtcAllowOrigins` 与浏览器实际访问页面的 origin 完全一致；更换主机或端口须同步更新。
- 页面本身使用 HTTPS 时必须给 8889 配置证书，否则浏览器按混合内容拒绝 WebRTC。
- 相机 RTSP 源地址与可选凭据在业务页面填写，由 Server 经 API 建路径；配置文件不写 `paths`。
- 相机输出 H.264；H.265 在浏览器 WebRTC 预览中通常不可用。

历史 Agent 相机字段只用于升级期识别和备份，不再是运行配置入口。未取得现场品牌、协议及参数前，不能宣称真实摄像头接入或识别完成。

## 升级、维护退出与回退

升级前停止接单并查询未决称重／打印，核实结果；保留未知状态，不自动重打。停止 Server 与 Agent 服务，备份数据库、整个持久 `configs`、`data`、`agent`（包括 `agent.env`、维护 JSON、审计、暂停标志、身份文件及 `state` 下全部账本）。打印账本不清空、不重建。

1. 解包新版本，用相同组件参数再次运行 `install.sh`。安装器只增加 `releases/<新版本>`；数据库、配置、图片、账本和自定义 unit 不覆盖。
2. 安装器核对实际 systemd unit、drop-in 和 `agent.env`：
   - unit 与模板不同时，在发布目录写 `<unit>.candidate`，并列出旧 unit／drop-in 中的 `--server`、`--agent-id`、`--scale-id`、`--queue`、旧密钥或相机参数。
   - `agent.env` 含废弃字段时生成 `agent.env.upgraded` 和相机地址备份，保留串口、打印、校准及未知行。
   - 默认只报告，不修改现场文件。核对候选内容后显式使用 `--update-units` 替换 unit、停用含旧参数的 drop-in，并用 `--update-env` 备份和应用清理后的 `agent.env`。安装器不会启动服务。
   - 发现旧 `weighbridge-agent-maintenance.service` 时明确提示；统一维护已并入 Agent 主进程，现场核对后停止并 disable 旧服务。
3. 先以备份副本验证迁移，再进入新发布目录显式执行 `weighbridge-migrate up --kind all`。迁移失败不切换版本，修复或恢复配套备份，不直接在生产盲目 Goose down。
4. 确认数据库兼容后更换 `current` 符号链接，`systemctl --user daemon-reload` 并启动 Server，检查 health、ready、页面及本厂身份；再启动 Agent，检查维护页连接、CUPS 队列、串口状态与配置摘要。
5. 旧版本目录保留；回退时停止两个服务，切回旧链接。数据库有不兼容变化时，须同时恢复匹配的数据库、图片、配置及账本快照，不仅替换二进制。回退与重启均不重放物理打印。

旧维护服务软件保留作历史兼容，不属于最终常驻产品；现场通过后 disable 旧服务与独立密码入口，并确认旧 drop-in 不再指向历史目录。现有单厂库通过新增 Goose migration 升级并保留历史文件；旧多模式身份转换或清库流程不适用于该升级。

## 换机与参数恢复

换机不需要更换编号、密钥或重新绑定。先在旧机停止 Agent，利用单磅互斥避免双接入；在新机安装 Agent-only 并填写新的 Server 地址。管理员在设备维护 → 配置备份中手动保留 Agent 成功生效的串口、波特率、打印队列与校准参数，并选择已有备份明确发起恢复；保存应用及重连不会自动新增备份。换机前应核对已有备份，必要时在旧 Agent 在线时手动创建；不恢复旧 Server URL、主机名、连接／暂停状态、测量窗口或执行请求。

恢复前核对新机实际串口路径、CUPS 队列与校准值，再在维护页明确应用；不因连接成功自动套用旧主机参数。打印账本可单独迁移，账本不可读不阻断换机；Server 未确定的打印保持待核实，人员检查纸面后确认结果或明确发起新打印。任何重连、恢复或换机都不自动补称、确认、出单或重发旧打印。

## 断网与系统级恢复

验收时断开互联网并保持本机网络、PostgreSQL 和设备正常，刷新页面、重登、维护资料并以独立 mock 完成测试闭环；另一厂停止不影响本厂。随后重启两进程，检查本厂身份、图片和完整账本连续。设备 mock 仅用于测试，不能与真实 Agent 同磅并行。

业务网页无法启动时使用 `systemctl --user status weighbridge-business.service`、`journalctl --user -u weighbridge-business.service -n 200`，检查本机配置文件与 PostgreSQL 服务。Agent 同理使用 `weighbridge-agent-printer.service`。数据库不可用、仪表未连接时不能用浏览器缓存或手填原始重量确认；抓拍失败仅在本厂明确关闭强制抓拍策略时允许保留缺图状态办理。提交结果未知先查询原请求；打印结果未知先核实。服务恢复不自动补办或重打。

真实 Server 电脑灾难恢复、单盘保护及恢复指标仍属暂缓议题；Agent 参数备份不是整套业务系统备份。

## 部署验证入口

打包后运行：

```sh
task local:install:smoke PACKAGE_DIR=<已解包目录>
task local:release:smoke PACKAGE_DIR=<已解包目录>
```

安装冒烟只用临时目录和替身，覆盖同机、Server-only、Agent-only、无数据库 Agent 安装、旧 `agent.env` 清理、unit/drop-in 核对与受控迁移、私有配置和账本保留，不启动服务或连接真实业务库。发布冒烟使用隔离临时 PostgreSQL 验证内嵌页面、迁移、管理员初始化、首次身份编码填写、Agent 连接配置与重启保留，不操作真实设备或实物打印。当前结果见[当前状态](../next.md)。

2026-10-25 联合回归复核：上述两个冒烟入口均通过，详见[交付记录](../requirements/local-device-maintenance-delivery.md)。这只是临时目录与隔离数据库的部署脚本验证，不等于已在真实主机、真实旧库或现场网络完成安装升级。
