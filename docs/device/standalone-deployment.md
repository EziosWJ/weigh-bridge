# 本厂三服务安装、升级与恢复

适用 [Issue #6](https://github.com/EziosWJ/weigh-bridge/issues/6)、[当前规格](../requirements/standalone-site-spec.md)与 ADR 0023／0024。软件交付与真实现场验收分别见[本次交付记录](../requirements/standalone-site-delivery.md)。本文命令供目标 Linux amd64 工控机的专用普通用户执行，不表示已部署到现场。

## 组件与准备

常驻核心为 PostgreSQL、本地业务服务（含 Web 和维护 API）、设备 Agent。CUPS 为系统配套；不启动中心、Vite、Node 或独立维护服务。数据库与 CUPS 由系统服务管理，两个 Go 进程由同一专用用户的 systemd user manager 管理，便于业务服务在 Agent 停止时读取有效配置、检查队列和启动设备。业务进程不打开串口。

系统管理员先准备 PostgreSQL 数据库及独立账号、CUPS 队列、Python 3/Pillow/中文字体（现有打印渲染要求）、`flock`、systemd 用户会话及开机 linger。RTSP 抓拍另需 FFmpeg；HTTP 抓拍不依赖 FFmpeg。按实机赋予串口组和 CUPS 必需权限。维护功能中的系统 CUPS restart 如需 polkit，只授予专用用户重启 `cups.service`，不授予通用 sudo/systemctl；参考[原维护说明](agent-local-maintenance.md#cups-与服务权限)。

两个厂预先分配不同且稳定的 `node.site_id`（1..255，例如一厂 1、二厂 2），确认厂区码、地磅码、实名操作员资料。首次初始化不能各自使用默认的相同厂区 ID。账号及资料可来自同一份经确认清单，但各库之后独立维护；不建立同名关联或同步。

构建机执行 `task standalone:package VERSION=<唯一版本>`，得到 `bin/weighbridge-<版本>-linux-amd64.tar.gz`。包内四个二进制统一版本并有 SHA-256 manifest；只有业务与 Agent 两个常驻。迁移和初始化工具仅手动运行。安装机无需 Node。

## 首次安装

1. 封存联调库、图片、配置及完整打印账本。生产交易从空业务库开始；仅采用经确认账号、资料及现场身份。已有生产记录不能使用本初始化路径，参见下文转换边界。
2. 解包发布文件。使用受控本机输入设置 `WEIGHBRIDGE_DATABASE_URL`（本机 PostgreSQL 连接串）、`WEIGHBRIDGE_SITE_ID`；数据库账号密码可在 URL 中提供，也可分别设置 `WEIGHBRIDGE_DATABASE_USERNAME`／`WEIGHBRIDGE_DATABASE_PASSWORD`。避免把密码写入 shell 历史或说明。运行 `bash install.sh "$HOME/weighbridge"`。安装器校验二进制、生成随机 JWT 和受限配置；已有配置、数据及 unit 不覆盖，不自动迁移或启动服务。
3. 进入 `$HOME/weighbridge/releases/<版本>`，执行 `APP_ENV=prod ./bin/weighbridge-migrate up --kind all`。本目录的 `configs` 指向持久配置；数据库 schema／seed 使用显式 Goose 迁移，API 不自动建表。
4. 受控输入环境变量 `WEIGHBRIDGE_ADMIN_PASSWORD`（12..72 字节），在同一目录执行：

   ```sh
   APP_ENV=prod ./bin/weighbridge-standalone-init \
     --config-dir ../../configs \
     --site-code SITE01 --site-name '一厂' \
     --scale-code SCALE01 --scale-name '一号地磅' \
     --agent-id scale-agent-SITE01 \
     --agent-env-path "$HOME/weighbridge/agent/agent.env"
   unset WEIGHBRIDGE_ADMIN_PASSWORD WEIGHBRIDGE_DATABASE_URL WEIGHBRIDGE_DATABASE_USERNAME WEIGHBRIDGE_DATABASE_PASSWORD
   ```

   厂区码／名称按现场确认值替换。命令从配置读取固定 site ID，创建本机身份与设备连接配置，替换 seed admin 初始密码，拒绝覆盖已有 `agent.env`。串口默认禁用；不能把初始化命令作为密码恢复、密钥轮换或升级命令重复运行。
5. 核对 unit 与已有 drop-in：`systemctl --user cat weighbridge-business.service weighbridge-agent-printer.service`。初次激活版本：

   ```sh
   cd "$HOME/weighbridge"
   ln -s releases/<版本> current
   systemctl --user daemon-reload
   systemctl --user enable --now weighbridge-business.service
   curl --fail --max-time 5 http://127.0.0.1:18900/health
   curl --fail --max-time 5 http://127.0.0.1:18900/ready
   ```

   `/health` 只证明存活，`/ready` 检查数据库。访问本厂地址登录 admin，通过既有账号、角色和厂区授权入口准备至少两名实名、有 `weighing:confirm` 权限及本厂操作授权的用户；准备不足时可做管理，但正式业务受阻。日常不使用共享管理员账号称重。
6. 使用统一设备维护页检查有效 Agent 连接、CUPS 队列和校准；配置仪表的串口、波特率、现场源单位和陈旧门限。D12 已确认现场源单位为吨的设备使用 `t`，其他设备须重新核对。草稿保存后需明确应用并核对运行摘要，不能把“已保存”当作“已生效”。之后启用 `systemctl --user enable --now weighbridge-agent-printer.service`。

## 摄像头与识别

Agent 的 `agent.env` 支持以下本地字段，维护页面不回显敏感 URL：

| 字段 | 含义 |
| --- | --- |
| `WEIGHBRIDGE_CAMERA_FRONT_URL` | 前视 HTTP(S) 单张 PNG/JPEG 或 RTSP 地址 |
| `WEIGHBRIDGE_CAMERA_REAR_URL` | 后视地址，须与前视同时设置 |
| `WEIGHBRIDGE_CAMERA_TIMEOUT_MS` | 整次抓拍、上传和识别时间预算，默认 2000，允许 200..10000；外层采集最大 8 秒 |
| `WEIGHBRIDGE_PLATE_RECOGNITION_URL` | 可选本地识别接口，POST multipart `file`，返回 `{"candidate":"川A12345"}` |

HTTP 抓拍请求禁用缓存；RTSP 通过 FFmpeg 每次读取新帧。两视角必须有效并由本地业务服务确认保存、校验摘要；抓拍期间重量、稳定或链路异常则失败，不使用旧图替代。候选识别失败时明确标记，车牌需操作员核对。通用识别适配契约需要设备识别服务或本机 HyperLPR 适配器实际实现，不代表已交付某品牌驱动或已安装识别引擎。未取得现场品牌、协议及参数之前，不能宣称真实摄像头接入或 S3 完成。

真实密码只保存在受限本机配置。界面“已配置”只表示存在参数，在线能力在真实采集时验证。摄像头修改也须显式应用并检查运行状态。

## 升级、维护退出与回退

升级前停止接单并查询未决称重／打印，核实结果；保留未知状态，不自动重打。停止两个用户服务，备份数据库、整个持久 `configs`、`data` 和 `agent`（包括 `agent.env`、维护 JSON、审计、暂停标志、身份文件及 `state` 下全部账本）。打印账本不清空、不重建。

新包安装器仅增加 `releases/<新版本>`，不会覆盖现场配置、有效身份及已有 unit；候选 unit 在发布目录供比较。先以备份副本验证迁移，再进入新发布目录显式执行 migrate。迁移失败不切换版本，修复或恢复配套备份，不直接在生产盲目 Goose down。

确认数据库兼容后在停机窗口更换 `current` 符号链接，重新加载 unit 并启动业务，检查 health、ready、页面及数据库身份，再启动 Agent 并检查配置摘要和设备链路。旧版本目录保留；回退时停止两个服务，切回旧链接。数据库有不兼容变化时，须同时恢复匹配的数据库、图片、配置及账本快照，不仅替换二进制。回退与重启均不重放物理打印。

统一维护页以业务账号授权 `agent:maintenance:view`／`agent:maintenance:manage`；维护操作记录实际账号并进入业务审计。分别测试 Agent 停止时检查/配置/启动，以及旧 `weighbridge-agent-maintenance.service` 停止时完整维护。现场通过后才 disable 旧服务与独立密码入口，且核对旧 drop-in 不再指向历史目录。旧维护服务软件保留作兼容，不属于最终常驻产品。

现有 center/site 数据库有不可变部署身份，不能通过改 YAML、直接 SQL 改身份或重跑初始化转换。转换前单独制定迁移、遗留临时主体逐项处置与回退方案；当前路径仅支持显式新建自治库。历史临时主体不会静默转正式，关联与单据快照保留。

## 断网与系统级恢复

验收时断开互联网并保持本机网络、PostgreSQL 和设备正常，刷新页面、重登、维护资料并以独立 mock 完成测试闭环；另一厂停止不影响本厂。随后重启两进程，检查本厂身份、图片和完整账本连续。设备 mock 仅用于测试，不能与真实 Agent 同磅并行。

业务网页无法启动时使用 `systemctl --user status weighbridge-business.service`、`journalctl --user -u weighbridge-business.service -n 200`，检查本机配置文件与 PostgreSQL 服务，不依赖已停用的旧维护页。Agent 同理使用其 unit。数据库不可用、设备未连接或抓拍失效时不能用浏览器缓存、手填原始重量或缺图完成确认。提交结果未知先查询原请求；打印结果未知先核实。服务恢复不自动补办或重打。

真实现场清单另行记录 D12 非零带载、稳定灯、重启串口权限、前后抓拍与候选核对、两次称重、单据、A4/五联连续纸及未知结果核实。软件测试不替代现场结论，单盘故障保护方案仍暂缓。
