# 本地设备联调台

联调台在 Linux／WSL 上为同一个开发 Server 提供 D12 虚拟串口和车牌主动上报。两个模拟器在同一个 Go 程序里运行，打开一个页面即可操作。真实 Agent 作为子进程读取虚拟串口，并通过现有设备协议向 Server 上报。

## 启动

在仓库根目录启动开发环境，首次配置见[开发初始化](../development-initialization.md)。

```sh
task dev
```

在管理后台完成开发厂区身份设置。在「摄像头管理」添加测试摄像头，启用设备上报，绑定 `camera-sim-01`。同一地磅只能有一个 Agent 在线，启动联调台之前请停掉该开发环境中手动启动的 Agent 或 Agent mock。

另开终端运行：

```sh
task device:lab
```

打开 <http://127.0.0.1:8093>。默认开发 Server 为 `http://127.0.0.1:8900`，设备序列号为 `camera-sim-01`，心跳间隔为 30 秒。开发环境地址或绑定序列号有调整时可传参：

```sh
task device:lab -- --server http://127.0.0.1:18900 --serial camera-test --heartbeat 2s
```

控制页只接受回环 IP 监听。端口已被占用时可加 `--listen 127.0.0.1:8193`。联调台每次生成独立的临时 Agent 配置和状态目录，默认打印队列留空。按 Ctrl+C 或发送 SIGTERM 会关闭控制页、停止子进程、释放虚拟串口并删除临时目录。根目录的 `agent.env` 与常规 Agent 状态目录无需预先配置。

## 联调

1. 等待页面显示 Agent 已连接、串口读取端已打开、摄像头收到心跳 ACK。若上报返回 403，检查摄像头序列号绑定和 Server 允许的来源 IP。
2. 打开称重工作台并取得操作权。在联调台输入重量并点击「模拟上磅」或「设置稳定重量」，再指定或随机生成车牌上报。
3. 回到工作台核对车牌，按正常流程操作。回磅时设置新重量，点击「再次上报当前车牌」产生新的识别事件。「重试上次上报」会重发上次失败的原事件。
4. 展开「仪表故障模拟」可测试动态位、停发和坏帧。「已打开」表示 Agent 正在读取 PTY，实际重量是否可用以工作台状态为准。

车牌模拟仅提供识别事件与心跳。强制抓拍场景需要配置 [RTSP 模拟源](camera-simulation-dev-memo.md)，由 Server 直接取图。单项调试仍可使用 [D12 模拟器](d12-simulator.md)和[车牌识别模拟器](camera-simulator.md)。

`task device:lab:smoke` 使用隔离 PostgreSQL、真实 API、Agent 和 Chromium 验证联合联调，需要 Docker 和已安装的 Web 开发依赖及 Chromium。
