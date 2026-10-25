---
status: accepted (consolidated summary)
---

# 全系统只有一种本厂运行形态

2026-10-24 有效决定摘要，合并已确认修订，不构成新的业务决策。[原始决定与演进记录](../history/adr/0028-single-local-runtime-identity.md)保留来源与历史事实；实现及验收统一见[当前状态](../next.md)。

## 有效决定

每厂独立运行本地 Server、Agent 和 PostgreSQL，不保留中心、site／standalone 或空角色兼容。本厂未配置可登录管理，首次名称及地磅配置成功即初始化完成，身份编号固定，无操作员人数门槛；Agent 接入和配置归属按 ADR 0030。本厂账号及资料独立维护，互联网断开不影响正常本地链路。

## 取舍

退出多模式和中心依赖，减少安装、初始化和维护分支；接受当前没有跨厂统一管理的边界。退出旧多模式时曾选择重建旧业务库，该一次性切换已记录；当前设备改造保留现有单厂业务、图片和账本，不沿用清库步骤。

## 当前细则

[当前总规格](../requirements/standalone-site-spec.md)、[设备职责决策](0030-local-device-responsibilities-and-configuration.md)。
