# 现场 Agent 与开发 mock

现场 Linux Agent 的 D12 仪表、RTSP/HyperLPR3 识别尚未实现。SK-820 最小打印 Agent 已实现并部署，A4 单张纸版面已获现场确认，五联效果及连续走纸仍待验收，见[部署记录](../docs/device/aisino-sk820-agent-test.md)。开发和上线前测试使用独立 agent-mock，通过[同一通信契约](../protocol/v1.md)连接 Server。

## 本机启动

先运行 `task init -- --database sqlite`。初始化会在忽略的本地配置中生成独立随机 Agent 密钥，API 与 mock 自动读取同一份配置。已有开发配置也可重新运行 `task init` 补齐缺失密钥，已有 JWT、数据库设置及已配置密钥不会改写。补齐后重启 API。

```bash
# 初始化或补齐已有本地配置
task init -- --database sqlite
# 终端一：API 与系统 Web
task dev:sqlite
# 终端二：独立 mock
task agent:mock
```

也可用 `APP_WEIGHING__AGENT_KEY` 覆盖本地密钥；覆盖时 API 与 mock 必须使用同一值。独立可执行文件没有本地配置时需要此环境变量。真实密钥不得提交。

mock 控制台：http://127.0.0.1:8090。默认连接地址跟随本地 API 的 `http.address` 配置，例如 API 改为 `:8900`，mock 自动连接 http://127.0.0.1:8900；通配监听地址会转换为本机回环地址。启动日志显示实际连接目标。Agent 编号默认 `scale-agent-1`，地磅 ID 默认 `1`。Server 与 mock 的编号、地磅和密钥必须一致。远程地址仍可显式覆盖：`task agent:mock -- --server https://your-server.example --listen 127.0.0.1:8090`。独立程序没有可用 Server 配置时回退到 http://127.0.0.1:8080。

## 页面操作

1. 设置目标吨重，点击“模拟上磅”；系统可看到连续 4 秒的不稳定波动，再转为稳定。
2. 发送识别候选，或模拟识别失败；系统由操作员核对车牌。
3. 默认有车头、车尾示例图，也可上传 PNG/JPEG 替换；采集时取得独立图片引用。
4. 在系统工作台采集并人工确认。mock 可强制失稳、故障、断连或下磅，验证系统的失效处理。
5. 在系统发起打印，mock 展示任务内容，可回报成功、失败或暂不回报以保留未知结果。

控制台只服务于开发测试，默认监听本机。无需 SQLite、本地业务库或设备驱动；重连只发送当前状态，不补办旧称重、不自动重打。真实设备以后实现同一协议，不需要将模拟代码放回系统。

`task agent:build` 生成独立可执行文件。实现沿用 Server 的 Go module 和既有 WebSocket 依赖，页面及示例图嵌入该程序。Server host-collector 仍只负责宿主机监控，与现场 Agent 无关。

## 证据保留

Server 将抓拍保存于 `weighing.image_dir`，默认 `server/data/agent-images`（相对 Server 工作目录）。该目录已忽略；备份、迁移和恢复需与业务数据库一起保留。历史内置测试图片继续用于读取旧快照，不参与新采集。
