# 通过本厂 Server 匿名读取实时重量

酒厂内的其他系统需要读取仪表当前值，且无需维护称重系统账号。决定由本厂 Server 在 `GET /api/weighing/scales/{code}/device` 提供匿名只读快照，由现有 Agent 继续独占仪表串口。公开字段限于实时重量、状态和上报时间，完整设备详情继续使用现有业务授权，访问公开端点的网络范围由厂内部署控制。

接口字段和错误语义见[厂内实时重量读取契约](../requirements/local-device-maintenance-spec.md#41-厂内实时重量读取契约)。
