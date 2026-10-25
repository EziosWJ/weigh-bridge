> **历史档案（2026-10-25）**：以下保留页面演进、旧版操作及当时验证事实；现行说明见[原路径的收敛版](../../../web/design-system/pages/agent-maintenance.md)，实际交付见[当前状态](../../next.md)。

# 设备维护页面

适用路由：`/system/agent-maintenance`。本页为 Detail Page、Standard Form Page 和 Standard List Page 的组合变体，沿用 MASTER 中的后台规范。

## 视觉与布局

- 沿用背景 `#F5F7FA`、面板 `#FFFFFF`、边框 `#E5E7EB`、正文 `#1F2937`、辅助文字 `#6B7280`、操作蓝 `#1677FF` 对应的现有 token；状态色仅表达实际状态。
- 沿用后台字体栈，正文 14px、卡片标题 16px；重量和时间使用等宽数字，内容左对齐。
- 页面布局为“标题与刷新 → 最近检查时间 → 四个业务页签 → 当前内容”。复用 `SectionTabs` 的选中态、方向键、Home / End 和键盘焦点规则。
- 总览使用紧凑的四项服务状态条，下方为现场诊断和设备执行控制；设备页按称重仪表、打印队列与校准分区。摄像头、识别和 MediaMTX 不属于本页，见 `/weighing/cameras`。
- 维护记录复用 `DataTableCard` 和 `TableToolbar`；窄屏仅表格内部横向滚动。日志使用独立的可滚动文本区，保留换行。

## 配置与操作

- 复用 `FormSection`、`Field`、`Input`、`Select`，只保留“称重仪表”和“打印与校准”两组：串口路径、仪表波特率、CUPS 打印队列、单据节长、水平／垂直偏移、内容缩放和基准打印宽度。
- 只有一个“保存并应用”提交动作。内部完成校验、持久化、设备重载、结果核实和必要回退；不再呈现草稿、已保存、待应用三套状态，也不回显连接密钥、Agent 编号、相机源或识别地址。
- 页面顶部只读展示 Agent 实际成功生效的参数（版本、时间、仪表／打印状态）和 Server 保存的成功配置备份；恢复备份必须由管理员明确发起。
- 应用结果分别表达成功、拒绝、未知、已回退和回退失败。结果未知时先读取实际状态，表单保留输入供修正；不自动重试写操作。
- CUPS 队列由 Agent 主机枚举；无法读取时明确显示错误，不把 Server 主机的队列当作 Agent 队列。
- 切换页签和刷新状态保留当前编辑；保存、应用或恢复后按实际结果重新读取配置。
- 将 CUPS 重启、队列恢复和校准打印放在打印区；设备控制保留在总览。维护确认中使用中文动作名称，沿用原有权限、作业核实与 API 契约。
- 未知与异常分别展示。仪表重量仅在设备执行中且仪表数据可用、新鲜时展示，零值显示为 `0 吨`，不把旧读数作为当前重量。串口权限错误使用 Agent 实际运行账号说明。

## 软件验证

执行 `task frontend:lint`、`task frontend:build` 和 `npm --prefix web run test:local-management`。浏览器回归使用接口夹具，覆盖单次保存并应用（含小数节长与偏移）、成功备份恢复、队列来自 Agent、未知打印作业确认、只读 RBAC、页签键盘操作、记录与日志、桌面及 390px 窄屏布局；不代表现场设备验收。称重采集与摄像头页面分别由 `test:weighing:agent-capture`、`test:weighing:camera-live` 和 `test:camera-management` 覆盖。

## 2026-10-25 职责收敛

按 [ADR 0030](../../adr/0030-local-device-responsibilities-and-configuration.md) 与[设备规格](../../requirements/local-device-maintenance-spec.md)，本页不再包含相机、识别、MediaMTX 或连接身份配置。业务服务地址为只读排障信息，由 Agent 主机配置工具修改；设备参数仍由 Agent 本地持有，Server 只保存成功配置副本用于明确恢复。
