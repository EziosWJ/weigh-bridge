# 06：CPU/内存进程 Top 10

GitHub：[#70](https://github.com/EziosWJ/base-project-golang/issues/70)；状态：ready-for-agent。

## Parent

[Spec #64](https://github.com/EziosWJ/base-project-golang/issues/64)

## What to build

ADMIN 在原生和 Docker 部署中查看宿主机 CPU、内存占用最高的各 10 个进程，看到进程名、PID 和占用量。进程退出或读取权限不足时页面保留有效结果并说明覆盖情况，帮助定位整机瓶颈的主要占用者。

## Acceptance criteria

- [ ] 两种采集方式均读取宿主机进程，分别按 CPU 占用和常驻物理内存量提供 Top 10，结果不足 10 项时照实显示。
- [ ] 展示进程名、PID 与占用量；进程 CPU 与总体 CPU 同一窗口、以整机总算力为 100%。
- [ ] 新进程或尚未取得有效 CPU 差值时明确标为采集中；进程退出等采样竞争不使整个概览失败。
- [ ] 权限不足或其他读取失败保留有效项，并显示覆盖不完整；旧数据遵循有效时间和过期规则。
- [ ] 接口与页面均维持 ADMIN 限制，第一版仅提供进程查看能力。
- [ ] 通过真实 HTTP 路由验证排序、不足 10 项、CPU 归一化、进程退出、部分失败与过期。
- [ ] Playwright 验证两组排行和覆盖不完整提示；Docker 场景记录主机进程身份对照。
- [ ] 后端检查、前端 lint/build 通过，API 与必要读取权限说明同步。

## Blocked by

- #67：Docker 宿主机采集端到端接入。
