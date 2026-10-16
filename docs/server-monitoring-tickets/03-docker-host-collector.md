# 03：Docker 宿主机采集端到端接入

GitHub：[#67](https://github.com/EziosWJ/base-project-golang/issues/67)；状态：ready-for-agent。

## Parent

[Spec #64](https://github.com/EziosWJ/base-project-golang/issues/64)

## What to build

Docker 部署的 ADMIN 打开同一个监控页面，可以通过同机 Linux 宿主机采集进程取得真实主机 CPU、内存和主机身份。采集进程经本地 Unix socket 提供最新样本，API 使用现有概览与授权契约；采集断开、超时或权限不足时页面准确报告。该切片独立演示 Docker 主机概览，不等待趋势或其他设备模块。

## Acceptance criteria

- [ ] 提供可发布的宿主机采集进程和明确的运行配置，使用与原生 API 相同的主机 CPU/内存指标语义。
- [ ] API 通过本地 Unix socket 读取最新样本，采集源显式选择，失败时不静默回退为容器指标。
- [ ] Unix socket 提供最新样本，不承担长期或持久历史；历史仍由 API 侧按后续已接入的行为保存。
- [ ] 采集运行于宿主机，以专用用户和必要读取权限启动；Unix socket 目录、组/用户访问与容器映射有可执行说明。
- [ ] 原生与 Docker API 返回同一概览契约，主机身份和 CPU/内存数据可与宿主机对照。
- [ ] 不修改现有 API 容器为全权限宿主机采集容器；保留其既有隔离方式与网络语义。
- [ ] 采集有超时、取消及不重叠处理；socket 缺失、权限错误、断开或采集端部分失败可观察，其他 API 继续工作。
- [ ] 旧样本有效时间不因拉取而更新；采集进程重启重新建立基线并继续提供新样本，已有 API 历史不因该重启主动清空。
- [ ] 使用真实 Unix socket 的端到端测试贯通采集端、API 和页面状态；真实 Linux Docker 场景验证数据属于宿主机。
- [ ] 接入项目统一发布/启动入口并给出启动、升级、状态检查及有超时和清理的验证方法；后端检查和受影响前端检查通过。

## Blocked by

- #65：主机 CPU/内存监控与 ADMIN 入口。
