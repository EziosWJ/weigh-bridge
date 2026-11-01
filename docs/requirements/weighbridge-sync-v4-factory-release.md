# 厂云同步 v4 厂端对接发布包

> 发布状态：云端已实现摘要协议 v4 的接收、确认、单张/批量/旧编码只读对账及管理页查询；厂端实现和软件验证见[交付记录](cloud-sync-v4-delivery.md)，现场验收未完成。本文是给厂端的发布清单，详细云端设计见[接口设计](weighbridge-sync-api-design.md)，共享业务边界见[厂云同步契约](cloud-sync-contract.md)。本文不改变共享契约；摘要规则或接口字段调整时必须同步更新本文、接口设计并提升 `digestVersion`。

## 0. 发布清单

| 序号 | 内容 | 发布位置 | 状态 |
| --- | --- | --- | --- |
| ① | 摘要规范正文与测试向量（含期望摘要） | 本文第 2 节；`server/internal/weighbridge/testdata/digest_v2_vectors.json` | 已发布 |
| ② | 上传接口字段与错误类别枚举 | 本文第 3 节；[接口设计](weighbridge-sync-api-design.md) | 已发布 |
| ③ | 对账查询（单张、批量、旧编码历史确认）接口字段 | 本文第 4 节；[接口设计](weighbridge-sync-api-design.md) | 已发布 |
| ④ | 新协议版本标识及旧队列处置边界 | 本文第 1、5 节 | 已发布 |

## 1. 版本标识与旧队列处置边界

### 版本标识

| 标识 | 取值 | 说明 |
| --- | --- | --- |
| `schemaVersion` | `4` | 新协议消息结构版本；与 v2/v3 单据协议互不兼容 |
| `digestVersion` | `2` | SHA-256 摘要规则版本；不同版本不可直接比较 |
| 接口前缀 | `/api/v1/weighbridge/sync/v4` | 上传、单张对账、批量对账、摘要确认 |
| 成功响应包 | `{code,message,data}` | 成功 `code=200`，且必须校验 `digest` 与 `ticketNo` |
| 失败响应包 | `{code,message,data}` | `data.category/reason/retryable` 为机器可读分类 |

### 旧队列处置边界

- 云端保留 v2/v3 的 `/api/v1/weighbridge/sync/changes` 接口和既有数据，不自动删除、不自动迁移；v4 与 v2/v3 并行存在，不能据此推定旧协议已下线。
- v4 不兼容 v2/v3：不能把 v4 摘要当作 v2/v3 变化的重试标识，也不能用 v4 接口补传 v2/v3 载荷；反之亦然。
- 新方案不要求厂端兼容或迁移旧单据协议；在干净环境验证。现有测试环境及数据先保留，备份、清理或重建另行决定，不自动清空队列或转换旧变化。
- 不以混用协议的单据认定 v4 历史完整或对账一致；现有测试数据如需继续使用，须另行确定处理方案。
- v4 写入只接受当前启用厂区编码；v2/v3 旧编码写入继续按现行规则拒绝。旧编码的已接收摘要变化只能通过 v4 只读对账查询。
- v2/v3 接口的下线需另行发布；在明确下线前，云端不拒绝合法的 v2/v3 请求。

### 推荐的厂端切换顺序

1. 保留现有环境及数据，在干净环境验证新协议。
2. 按本文第 2 节实现摘要规则 v2，用固定测试向量逐项校验通过。
3. 按本文第 3 节实现上传、状态依赖和错误分类。
4. 按本文第 4 节实现单张/批量对账与旧编码历史确认。

## 2. 摘要规范正文与测试向量

### 2.1 摘要定义

摘要规则版本为 `2`。摘要由厂端计算，既作为变化标识，也作为对账依据；云端保存厂端上传的摘要，**不重新计算**。摘要不提供来源认证。

摘要为以下规范化字节串的 SHA-256，输出 64 位小写十六进制。

### 2.2 字节串构成

1. 固定前缀（UTF-8）：`wb-change-digest\n`。
2. 按固定顺序写入字段。每个字段编码为：

```text
字段名 ASCII 字节 + ":" + 值 UTF-8 字节长度（十进制） + ":" + 值 UTF-8 字节 + "\n"
```

可空字段为 `null` 时编码为：

```text
字段名 + ":null\n"
```

长度前缀保证值中出现冒号、换行或非 ASCII 字符时无歧义。

### 2.3 固定字段顺序

标识字段（所有变化）：

```text
rulesVersion
siteCode
ticketNo
changeType
previousSiteCode
previousTicketNo
```

- `rulesVersion` 固定写十进制 `2`。
- `previousSiteCode`、`previousTicketNo`：重开及重开单作废写实际值；普通出单及其作废写 `null`。`previousTicketNo` 取 `ticket.previousTicketNo`。
- `ticket.ticketNo` 不重复写入摘要，它必须等于顶层 `ticketNo`，由顶层字段覆盖。
- `schemaVersion` 和请求中的 `digest` 字段**不参与**摘要计算。

所有变化继续写以下 `ticket` 字段（顺序固定）：

```text
title
plate
direction
materialCode
materialName
materialSpec
partyCode
partyName
firstConfirmedAt
secondConfirmedAt
issuedAt
issuerName
originalGross
originalTare
finalGross
finalTare
finalNet
netAdjustment
netAdjustmentMode
netAdjustmentRateBp
netAdjustmentCategory
netAdjustmentCategoryName
previousFinalGross
previousFinalTare
previousFinalNet
previousNetAdjustment
grossSource
tareSource
adjustmentRemark
adjustedBy
adjustedAt
driver
driverPhone
contactName
contactPhone
```

作废变化在以上字段之后继续写 `voidReason`、`voidActor`、`voidAt`，按该顺序编码。

### 2.4 规范化规则

- **文本**：按 UTF-8 原样编码，不做 Unicode 归一化、不去首尾空白、不改大小写。`null` 与空字符串 `""` 是不同值。
- **重量**：所有重量字段按最多三位小数的十进制吨解析为整数千分吨，禁止空白、空串和无符号负数；再格式化为 `符号 + 整数部分 + "." + 三位小数`。
  - `30`、`30.0`、`30.000`、`030` 都编码为 `30.000`。
  - `-0.5` 编码为 `-0.500`；`0` 编码为 `0.000`。
  - 无符号字段：`originalGross`、`originalTare`、`finalGross`、`finalTare`、`finalNet`、`previousFinalGross`、`previousFinalTare`、`previousFinalNet`。
  - 有符号字段：`netAdjustment`、`previousNetAdjustment`。
- **时间**：转换为 UTC，截断到毫秒（丢弃更小精度），格式为 `YYYY-MM-DDTHH:MM:SS.sssZ`。
  - `2026-10-30T08:00:00+08:00` 与 `2026-10-30T00:00:00Z` 编码相同。
  - `.123999Z` 截断为 `.123Z`。
- **可空整数**：`netAdjustmentRateBp` 为 `null` 或十进制整数字符串，不补前导零。
- **可空文本/时间/重量**：为 `null` 时写 `null` 标记；非空时按上述规则写值。`adjustedBy` 为空字符串与 `null` 产生不同摘要。

### 2.5 固定测试向量

向量文件：`server/internal/weighbridge/testdata/digest_v2_vectors.json`。文件包含 12 组完整请求、用途和期望摘要；每组 `change.digest` 必须等于 `expectedDigest`。参考实现：`server/internal/weighbridge/digest.go`（仅用于文档、向量和测试，云端接收路径不调用）。

| 向量 | 用途 | 期望摘要 |
| --- | --- | --- |
| `issue-basic` | 最小出单；可选文本为空字符串、可空字段为 null | `90d2aff38d8ed4f051e7403ac0ee86a04016f864936d5fab060fb7d131214fbd` |
| `issue-normalized` | 同一业务内容使用不同重量写法和时区，摘要必须与 issue-basic 相同 | `90d2aff38d8ed4f051e7403ac0ee86a04016f864936d5fab060fb7d131214fbd` |
| `issue-adjusted` | 含净重扣减、调整人和毫秒级调整时间 | `baf6c46a6b1e3d321d85b4e04d9f085fe111bbf689b24b5728e18ba9ae5ac3ac` |
| `issue-adjusted-truncated` | 时间截断到毫秒；摘要必须与 issue-adjusted 相同 | `baf6c46a6b1e3d321d85b4e04d9f085fe111bbf689b24b5728e18ba9ae5ac3ac` |
| `issue-empty-adjusted-by` | 空字符串与 null 不同，摘要必须与 issue-basic 不同 | `e54d6c3b0577192dac7e74c5563fd77fa845aff47ea3dcbe20e942876ff88cec` |
| `issue-delimited-text` | 文本包含冒号和换行；长度前缀保证无歧义 | `322cb4aa0a1e5a923200bb70e5b5b3925ebc0212153639228b03b9f197b289e7` |
| `void-basic` | 作废完整业务快照及作废信息；等价时间格式一致 | `a24323e3f50bd7b90f4aaf2b3d3fb563f8d8262aca652b51da8ca3170877b6ab` |
| `void-normalized` | 作废完整业务快照及作废信息；等价时间格式一致 | `a24323e3f50bd7b90f4aaf2b3d3fb563f8d8262aca652b51da8ca3170877b6ab` |
| `reopen-same-site` | 同厂区重开，携带原单关联和上版净重 | `fedf8924697ce8e03eebc7fc2cf0f733c08accaf0c9c637e222295c125020296` |
| `reopen-cross-site` | 新单使用 F001、原单使用旧编码 F000；previousSiteCode 参与摘要 | `642539bfec15d3ee9d7dc7f4179ffdce7e135ef104a893264d41e2891138850e` |
| `reopen-negative-adjustment` | 重开允许负数（上调）调整，负数按有符号千分吨规范化 | `e2f265dfa8897d4e225344a60421320508fbf9efd7749820e5eab502b96a1781` |
| `void-reopened-cross-site` | 跨厂重开单作废，摘要包含原单关联和完整业务快照 | `6d3965f1fa6c004b205531f8faa4e10f4f131cff934563fb3f9c5aa59264acdd` |

厂端必须用同一向量逐项校验；任何一组不一致都不得上线。重试必须使用已保存的原载荷和原摘要，不能重新生成。

## 3. 上传接口字段与错误类别枚举

### 3.1 接口

```text
POST /api/v1/weighbridge/sync/v4/changes
Content-Type: application/json
```

每次请求一个变化。厂区编码不是凭据；云端按请求中的 `siteCode` 校验当前启用厂区并归属数据。

变化和批量对账请求体上限均为 1 MiB（1,048,576 字节），超过时返回 HTTP 413、`data=null`，厂端按数据错误提示处理，不自动循环重试。

### 3.2 请求外层字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | 是 | 固定 `4` |
| `digestVersion` | integer | 是 | 本期固定 `2` |
| `digest` | string | 是 | 64 位小写十六进制；厂端计算；云端只校验格式，不重算 |
| `siteCode` | string | 是 | 出单厂区编码；1–40 位 `[A-Za-z0-9_.-]`；写入必须是当前启用编码 |
| `ticketNo` | string | 是 | 1–200 字节；必须等于 `ticket.ticketNo` |
| `type` | string | 是 | `issue`、`void` 或 `reopen` |
| `previousSiteCode` | string | 重开及重开单作废必填 | 原单出单厂区编码；普通出单及其作废不得出现 |
| `ticket` | object | 是 | 完整单据快照，见 3.3；重开及重开单作废时包含原单关联 |
| `void` | object | 作废必填 | 见 3.4；出单/重开不得出现 |

### 3.3 `ticket` 字段（出单/作废/重开）

| 字段 | 类型 | 必填 | 空值/规则 |
| --- | --- | --- | --- |
| `ticketNo` | string | 是 | 必须等于外层 `ticketNo` |
| `previousTicketNo` | string \| null | 重开是 | 出单为 `null`；重开为原单号 |
| `title` | string | 是 | 非空 |
| `plate` | string | 是 | 非空 |
| `direction` | string | 是 | `inbound` 或 `outbound` |
| `materialCode` | string | 是 | 非空 |
| `materialName` | string | 是 | 非空 |
| `materialSpec` | string | 是 | 可为空字符串 |
| `partyCode` | string | 是 | 非空 |
| `partyName` | string | 是 | 非空 |
| `firstConfirmedAt` | RFC3339 时间 | 是 | 含时区 |
| `secondConfirmedAt` | RFC3339 时间 | 是 | 含时区 |
| `issuedAt` | RFC3339 时间 | 是 | 含时区 |
| `issuerName` | string | 是 | 非空 |
| `originalGross` | 十进制吨字符串 | 是 | 最多三位小数；> `originalTare` |
| `originalTare` | 十进制吨字符串 | 是 | 最多三位小数；> 0 |
| `finalGross` | 十进制吨字符串 | 是 | 最多三位小数；> `finalTare` |
| `finalTare` | 十进制吨字符串 | 是 | 最多三位小数；> 0 |
| `finalNet` | 十进制吨字符串 | 是 | > 0 且 < `finalGross`；= `finalGross − finalTare − netAdjustment` |
| `netAdjustment` | 有符号十进制吨字符串 | 是 | 正数扣减，负数上调；出单不得为负，重开可为负 |
| `netAdjustmentMode` | string | 是 | `amount` 或 `percent` |
| `netAdjustmentRateBp` | integer \| null | 是 | `amount` 时为 `null`；`percent` 时为 −10000 至 10000（0.01% 单位） |
| `netAdjustmentCategory` | string | 是 | `netAdjustment ≠ 0` 时非空 |
| `netAdjustmentCategoryName` | string | 是 | 可为空字符串 |
| `previousFinalGross` | 十进制吨字符串 \| null | 重开是 | 出单为 `null` |
| `previousFinalTare` | 十进制吨字符串 \| null | 重开是 | 出单为 `null` |
| `previousFinalNet` | 十进制吨字符串 \| null | 重开是 | > 0 且 < 上版毛重；与上版毛皮重、调整一致 |
| `previousNetAdjustment` | 有符号十进制吨字符串 \| null | 重开是 | 与上版净重一致 |
| `grossSource` | string | 是 | 可为空；建议 `manual` 或 `collected` |
| `tareSource` | string | 是 | 可为空；建议 `manual` 或 `collected` |
| `adjustmentRemark` | string | 是 | 可为空字符串 |
| `adjustedBy` | string \| null | 是 | 无调整人时为 `null` |
| `adjustedAt` | RFC3339 时间 \| null | 是 | 无调整时间时为 `null` |
| `driver` | string | 是 | 可为空字符串 |
| `driverPhone` | string | 是 | 可为空字符串 |
| `contactName` | string | 是 | 可为空字符串 |
| `contactPhone` | string | 是 | 可为空字符串 |

出单不允许携带 `previousTicketNo`、上版毛皮重、上版净重、上版调整或 `previousSiteCode`。重开必须携带 `previousSiteCode` 和 `ticket.previousTicketNo`，且新单不能与原单是同一单。

### 3.4 `void` 字段（作废）

| 字段 | 类型 | 必填 | 规则 |
| --- | --- | --- | --- |
| `reason` | string | 是 | 非空 |
| `actor` | string | 是 | 非空 |
| `at` | RFC3339 时间 | 是 | 含时区 |

作废携带完整 `ticket` 快照供厂端计算摘要；云端不以作废载荷覆盖已保存的业务和重量快照。重开单作废时同时携带原单厂区编码和原单号。

### 3.5 成功响应

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "digest": "<64 位小写十六进制>",
    "ticketNo": "<单号>",
    "changeType": "issue",
    "receivedAt": "2026-10-31T02:00:00.123Z",
    "duplicate": false
  }
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `digest` | string | 回显请求摘要；厂端必须核对 |
| `ticketNo` | string | 回显请求单号；厂端必须核对 |
| `changeType` | string | 回显变化类型 |
| `receivedAt` | RFC3339 | 云端成功提交事务的时间 |
| `duplicate` | boolean | `true` 表示该摘要已处理过，返回原确认，未重复执行业务 |

同一摘要重复上传必须使用相同载荷；云端返回原 `receivedAt`。成功只在事务提交后返回；网络超时或结果未知时，使用原载荷和原摘要重试。

### 3.6 失败响应与错误类别枚举

```json
{
  "code": 409,
  "message": "原单尚未确认作废，等待作废确认",
  "data": {
    "category": "predecessor_pending",
    "reason": "original_not_void",
    "retryable": true
  }
}
```

| `category` | HTTP | `retryable` | 含义 | 厂端处理 |
| --- | --- | --- | --- | --- |
| `temporary` | 503 | true | 云端暂时不可用或事务未完成，结果未知 | 退避重试原变化；不生成新摘要 |
| `predecessor_pending` | 409 | true | 出单或作废前序尚未获得确认 | 等待前序确认后重试；不跳过前序 |
| `config_or_data` | 400 | false | 厂区编码不存在/停用、协议或字段不符合约定 | 提示管理员处理；不要自动循环重试 |
| `conflict` | 409 | false | 摘要、单号或状态冲突，不能覆盖 | 暂停该单据及依赖它的重开；其他单据继续 |

| `reason` | 所属 category | 含义 |
| --- | --- | --- |
| `invalid_request` | `config_or_data` | 报文、字段或版本不符合协议 |
| `site_not_found_or_disabled` | `config_or_data` | 写入厂区编码不存在或已停用 |
| `site_not_found` | `config_or_data` | 查询/批量中厂区编码从未登记 |
| `previous_site_not_found` | `config_or_data` | 重开的原单厂区编码不存在 |
| `ticket_missing` | `predecessor_pending` | 作废的原单尚未在云端创建 |
| `original_missing` | `predecessor_pending` | 重开的原单尚未在云端创建 |
| `original_not_void` | `predecessor_pending` | 重开的原单尚未确认作废 |
| `digest_reused` | `conflict` | 同一摘要被其他厂区/单号/类型使用 |
| `change_conflict` | `conflict` | 同一单据同类变化已有其他摘要 |
| `ticket_exists` | `conflict` | 单号已存在，不能重复出单 |
| `ticket_not_valid` | `conflict` | 单据已作废，不能重复作废 |
| `original_already_reopened` | `conflict` | 原单已重开，不能重复重开 |
| `concurrent_change` | `conflict` | 同一变化并发处理，需重试确认 |
| `ticket_conflict` | `conflict` | 单据或原单关联被其他变化占用 |
| `storage_unavailable` | `temporary` | 云端存储暂时不可用 |

厂端不得只根据 HTTP 状态推断结果，必须读取 `data.category`。`retryable=true` 的 `predecessor_pending` 仍需等待前序确认，不能立即跳过依赖。

### 3.7 去重与状态约束

- 同一 `(siteCode, ticketNo, changeType, digest)`：返回 200，`duplicate=true`，不重复执行业务。
- 同一摘要但厂区、单号或类型不同：`conflict / digest_reused`。
- 同一 `(siteCode, ticketNo, changeType)` 但摘要不同：`conflict / change_conflict`。
- 出单：`(siteCode, ticketNo)` 必须不存在，否则 `conflict / ticket_exists`。
- 作废：原单必须存在且为 `valid`；缺失为 `predecessor_pending / ticket_missing`，已作废为 `conflict / ticket_not_valid`。
- 重开：新单必须不存在；原单厂区编码必须已知；原单必须存在且已作废；原单不得已被重开。前序未满足按第 3.6 节类别返回。
- 较早变化不会覆盖较晚状态：已处理出单的重试只返回原确认；之后的作废/重开不会被旧出单回退。

## 4. 对账查询接口字段

### 4.1 单张对账

```text
GET /api/v1/weighbridge/sync/v4/tickets?siteCode=F001&ticketNo=T1
```

`siteCode` 为出单时厂区编码；历史编码只读允许。响应 `data`：

该接口无厂端凭据，返回的完整快照包含司机和联系人等信息；仅在可信网络内开放，并限制网络访问范围。厂端仅向有权限的管理员展示差异详情。

快照中 `netAdjustmentCategory`、`netAdjustmentCategoryName` 为空字符串时省略；`netAdjustmentRateBp`、`previousFinalNet`、`previousNetAdjustment` 为 `null` 时省略。比较前须分别还原为空字符串或 `null`。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `found` | boolean | 云端是否找到该单据 |
| `siteCode` | string | 回显出单厂区编码 |
| `ticketNo` | string | 回显单号 |
| `status` | string \| 缺省 | `valid` 或 `void`；未找到时缺省 |
| `ticket` | object \| null | 云端完整业务快照；未找到为 null |
| `voidReason`、`voidedBy`、`voidedAt` | string \| null | 云端作废原因、操作人和时间；未作废或未找到为 null |
| `latestDigest` | string \| null | 单据最新变化摘要 |
| `latestDigestVersion` | integer \| null | 最新摘要规则版本 |
| `latestChangeType` | string \| null | `issue`、`void` 或 `reopen` |
| `latestReceivedAt` | RFC3339 \| null | 最近一次成功接收时间 |
| `previousSiteCode` | string \| null | 重开单的原单厂区编码 |
| `previousTicketNo` | string \| null | 重开单的原单号 |
| `reopenedBy` | object \| null | 该单被重开时的新单 `{siteCode,ticketNo}` |
| `changes` | array | 历史确认，按接收顺序 |

`changes` 每项：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `digest` | string | 变化摘要 |
| `ticketNo` | string | 单号 |
| `changeType` | string | `issue`、`void` 或 `reopen` |
| `digestVersion` | integer | 摘要规则版本 |
| `schemaVersion` | integer | 接收时的协议版本 |
| `receivedAt` | RFC3339 | 云端接收时间 |

单据不存在返回 200 且 `found=false`；厂区编码从未登记返回 400 `config_or_data / site_not_found`。

### 4.2 批量对账

```text
POST /api/v1/weighbridge/sync/v4/reconcile
Content-Type: application/json
```

```json
{"items":[{"siteCode":"F001","ticketNo":"T1"},{"siteCode":"F000","ticketNo":"T2"}]}
```

- 每批 1 至 200 条；空批次或超过 200 条整批返回 400 `config_or_data / invalid_request`。
- 按请求顺序返回 `data.items`；允许重复项，不去重。
- 单项格式错误或厂区编码不存在只影响该项；其余项照常返回。

`items` 每项：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `siteCode` | string | 回显 |
| `ticketNo` | string | 回显 |
| `found` | boolean | 云端是否找到该单据 |
| `status` | string \| 缺省 | `valid` 或 `void` |
| `latestDigest` | string \| null | 最新变化摘要 |
| `latestDigestVersion` | integer \| null | 最新摘要规则版本 |
| `latestChangeType` | string \| null | 最新变化类型 |
| `latestReceivedAt` | RFC3339 \| null | 最近接收时间 |
| `previousSiteCode` | string \| null | 原单厂区编码 |
| `previousTicketNo` | string \| null | 原单号 |
| `reopenedBy` | object \| null | 新单 `{siteCode,ticketNo}` |
| `error` | object \| null | 该项错误 `{category,reason,retryable}` |

批量结果**不返回 `changes`**；需要历史确认时，对具体单据调用 4.1 单张对账。批量仍可查询旧编码的已接收单据。

### 4.3 旧编码历史确认

```text
GET /api/v1/weighbridge/sync/v4/changes?siteCode=F000&digest=<64 位小写十六进制>
```

`siteCode` 为出单时厂区编码，允许历史编码。响应 `data`：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `found` | boolean | 该摘要是否已被云端接收 |
| `siteCode` | string | 回显 |
| `digest` | string | 回显 |
| `confirmation` | object \| null | 原确认；未找到时为 `null` |

`confirmation` 字段：`digest`、`ticketNo`、`changeType`、`digestVersion`、`schemaVersion`、`receivedAt`。

`found=false` 表示云端未保存该摘要（包括被拒绝的变化），不代表内容无效；厂区编码从未登记返回 400 `config_or_data / site_not_found`。该接口只读，不能用于继续写入旧编码变化。

### 4.4 对账语义

- 摘要相同只表示双方记录的**最新变化摘要**一致，不是云端独立校验了业务内容。
- 最近接收时间只表示云端最近一次成功保存变化的时间，不代表厂端在线、没有积压或数据已完整上传。
- 云端不主动读取厂端数据库，不要求厂端上报积压或对账结果。
- 摘要规则版本不同或无法比较时显示待核查，不直接判为内容冲突；先结合变化历史和上传进度判断。
- 缺单可能是前序变化尚未上传或已被拒绝；应结合厂端上传状态处理，不静默补造或覆盖。

## 5. 厂端验收清单

- [ ] 12 组摘要测试向量全部匹配，`issue-normalized`/`issue-adjusted-truncated`/`void-normalized` 与对应基准摘要相同。
- [ ] 出单、作废、重开正常上传并获得确认；重复上传返回 `duplicate=true` 且不重复执行业务。
- [ ] 回执丢失后使用原载荷/原摘要重试，或通过 4.3 查询到原确认。
- [ ] 作废/重开前序未完成时收到 `predecessor_pending`，并等待前序确认。
- [ ] 相同摘要与不同厂区、单号或变化类型冲突，同单同类变化不同摘要也冲突；云端不重算摘要，不能检测相同摘要搭配不同业务内容。
- [ ] 跨编码重开成功，原单与新单关联可通过对账查询看到。
- [ ] 改码后旧编码写入被拒绝，旧编码历史确认只读可查。
- [ ] 新协议在干净环境验证；现有测试环境和数据未被自动清理或转换。
