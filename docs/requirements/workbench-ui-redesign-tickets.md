# 称重工作台视觉与弹窗重构 — 工单拆分计划

> **依据规格**：[称重工作台视觉、布局与弹窗体验重设计规格](workbench-ui-redesign-spec.md)  
> **关联基线**：[总规格](standalone-site-spec.md) · [业务规则](weighing-business-spec.md) · [设备维护规格](local-device-maintenance-spec.md)  
> **说明**：本文件只维护工单边界、依赖关系与验收口径，不重复规格细则。已同步发布为 GitHub Issues（`EziosWJ/weigh-bridge` `#15`～`#19`）。

---

## GitHub Issues 索引

| 工单 | Issue | 标题 | 标签 | 前置依赖 |
| --- | --- | --- | --- | --- |
| T1 | [#15](https://github.com/EziosWJ/weigh-bridge/issues/15) | `[Workbench UI] T1｜回归浅色中性基调与精简顶栏，移除暗黑大屏覆写` | `ready-for-agent` | 无 |
| T2 | [#16](https://github.com/EziosWJ/weigh-bridge/issues/16) | `[Workbench UI] T2｜主工作台布局重构：实体重量仪表置顶、右侧三段式作业台与主画面在上` | `ready-for-agent` | `#15` |
| T3 | [#17](https://github.com/EziosWJ/weigh-bridge/issues/17) | `[Workbench UI] T3｜集中核对弹窗与净重扣减区字号体系与排版重构` | `ready-for-agent` | `#15` |
| T4 | [#18](https://github.com/EziosWJ/weigh-bridge/issues/18) | `[Workbench UI] T4｜其余工作台弹窗与抽屉统一浅色与排版（运输资料/完成/补单/抽屉/临时主体）` | `ready-for-agent` | `#15`（建议 `#17` 后） |
| T5 | [#19](https://github.com/EziosWJ/weigh-bridge/issues/19) | `[Workbench UI] T5｜规范文档收敛、无用暗色素材清理与全量工作台回归验证` | `ready-for-agent` | `#16`、`#17`、`#18` |

---

## 依赖总览

```text
T1 浅色底座与顶栏重构
 ├──▶ T2 主工作台布局重构（仪表置顶 + 作业区 + 主画面在上）
 ├──▶ T3 集中核对弹窗与净重扣减排版重构
 └──▶ T4 其余工作台弹窗与抽屉排版重构（资料/完成/补单/抽屉/临时主体）
        │
        ▼ (T2 + T3 + T4 全部完成)
      T5 规范文档收敛、无用暗色素材清理与全量回归验收
```

- **T1** 为公共样式底座，必须最先落地；
- **T2、T3、T4** 在 T1 完成后互不阻塞，可顺序推进也可并行拆给不同执行者（注意协调 `workbench.css` 区块避免冲突）；
- **T5** 做最终文档收敛、死代码清理和多分辨率全量浏览器回归。

---

## 工单明细

### T1：`[Workbench UI] 回归浅色中性基调与精简顶栏，移除暗黑大屏覆写`

- **建议标签**：`ready-for-agent`
- **前置依赖**：无
- **目标**：移除工作台深黑蓝暗黑主题与网格光晕补丁，让工作台及所有 Portal 弹窗自然回归全站浅色 Token 体系，并将顶栏重构为简洁统一的桌面磅房状态栏。
- **涉及文件**：
  - `web/src/pages/weighing/workbench-shell.tsx`
  - `web/src/pages/weighing/workbench.css`（清理顶部全局暗色覆写、`body:has(.weighing-workbench)` 补丁及 `.workbench-topbar` 样式）
  - `web/src/pages/weighing/plate-input.css`（移除 `Consolas` 等宽编程字体及工作台暗色输入框覆写）
- **核心改动**：
  1. 移除 `body:has(.weighing-workbench)` 与 `.weighing-workbench` 对全局 `--color-*` 的暗色覆写以及 `industrial-grid.svg` 背景；确保工作台内打开的 `Dialog`、`RecordPicker`、`DataTable`、`Button` 直接继承 `globals.css` 浅色 Token。
  2. 移除 `.workbench-modal`、`.workbench-business-dialog`、`.weighing-review`、`.ticket-drawer` 上的青色径向光晕渐变、多层发蓝光阴影与 `::after` 描边。
  3. 重构 `WorkbenchShell` 顶栏：高度收敛为 `48px~52px` 白底单行；替换 `scale-mark.svg` 为简洁 `lucide-react` 图标；厂区/地磅名称与标题同行展示；右侧 `nav[aria-label="工作台操作"]` 内按钮统一使用 `Button` / `buttonStyles` 尺寸与样式，按「高频业务（过磅单/人工补单/后台查询）」与「会话控制（释放操作权/全屏）」区分视觉权重。
  4. 清理嵌套 `.weighing-workbench` 导致的冗余样式补丁。
- **验收标准**：
  - 打开 `/weighing/workbench` 及任意弹窗，全页为明亮浅色中性工业风格，无暗黑残留、无网格/光晕、无文字对比度不足。
  - 顶栏所有按钮高度对齐、权限显隐不变、`接管操作权` 警告态醒目。
  - `task frontend:lint`、`task frontend:build` 通过。
  - `npm run test:manual-supplement`、`npm run test:weighing:ticket-drawer` 中顶栏操作区与权限校验通过。

---

### T2：`[Workbench UI] 主工作台布局重构：实体重量仪表置顶、右侧三段式作业台与主画面在上`

- **建议标签**：`ready-for-agent`
- **前置依赖**：T1
- **目标**：将主工作台重构为贴近传统高效桌面磅房软件的“看车 → 看数 → 办事 → 动作”结构，彻底解决右侧挤压、双主按钮冲突和视频主辅颠倒问题。
- **涉及文件**：
  - `web/src/pages/weighing/workbench.tsx`
  - `web/src/pages/weighing/workbench.css`
  - `web/src/pages/weighing/camera-panel.tsx`
  - `web/src/pages/weighing/camera-live.tsx`
  - `web/src/pages/weighing/shared.tsx`（仅调整工作台 `variant="simple"` 方向分段控件与紧凑字段样式）
  - `web/tests/weighing-camera-multiview.mjs`（同步主画面在上断言）
- **核心改动**：
  1. **右侧改为三段式固定结构**（桌面 `>=1100px` 顶部固定 + 中部可滚动 + 底部吸底固定）：
     - **顶部（实体重量仪表盒）**：移至右侧最上方并固定不滚出视口；深石墨蓝背板 `#16222B` + 高对比大数字（`60px` / `>=1600px: 72px` / `<=800px height: 56px` / 手机 `48px`）；顶部清晰标牌展示「待定位车辆」或「首磅/回磅 + 毛重/皮重」及右侧状态标签；去掉大号 `Truck` 装饰图标；保留上报/采集时间及冻结态 `重新采集重量` 按钮。
     - **中部（车牌 + 独立本趟资料区）**：
       - 车牌行紧凑单行排布「省份 + 车牌输入 + 确认车辆」；**已定位车辆后「确认车辆」降为 `secondary` 次级样式**，把唯一主按钮焦点让给底部操作区；
       - 识别候选改为轻巧胶囊条；流转单匹配/选择/告警改为浅色语义条；
       - 首磅资料区中方向改为紧凑分段单选条（`38px`），物料与供收方并排；**回磅只读态直接展示「首磅毛/皮重与时间」（含隔日标签）**，无需弹窗即可核对。
     - **底部（唯一主操作区）**：固定吸底，云端卸货确认条与阻断原因紧贴主按钮上方；主按钮高度 `44px` 全宽。
  2. **左侧现场画面重构**：
     - 多路预览网格改为**主画面在上（`grid-row: 1`）、辅助小画面并排在下（`grid-row: 2`）**；保持切换画面不重建 WebRTC 连接；
     - 优化离线/未配置占位视觉，去除大片死黑；
     - 底部「本次抓拍」等待空态收敛为极简单行状态条（高度 `<= 128px`）。
  3. 同步更新 `web/tests/weighing-camera-multiview.mjs` 第 126 行上下位置断言为 `main.y < small.y`。
- **验收标准**：
  - `1920×1080`、`1366×768`、`1280×720` 下首屏完整可见实体重量仪表、主画面、车牌输入与底部主按钮；展开首磅资料不挤压重量仪表。
  - 390px 窄屏纵向自然堆叠、无横向溢出。
  - `task frontend:lint`、`task frontend:build` 通过。
  - `npm run test:weighing:camera-live`、`npm run test:weighing:camera-multiview`、`npm run test:weighing:agent-capture`、`npm run test:weighing:lookup`、`node tests/plate-candidates.mjs` 通过。

---

### T3：`[Workbench UI] 集中核对弹窗与净重扣减区字号体系与排版重构`

- **建议标签**：`ready-for-agent`
- **前置依赖**：T1
- **目标**：按 Spec 第 4.1~4.2 节统一字号阶梯与清单排版，消除核对弹窗字号倒挂（28px 常规字段）与三层卡套卡问题，优化回磅净重扣减的录入与核对效率。
- **涉及文件**：
  - `web/src/pages/weighing/review-dialog.tsx`
  - `web/src/pages/weighing/deduction-fields.tsx`
  - `web/src/pages/weighing/workbench.css`（核对弹窗区块）
  - `web/tests/review-final.mjs`（更新旧的 `>=22px` / 独立方框 / `>=28px` 样式断言为新 Spec 断言）
- **核心改动**：
  1. 弹窗最大宽收敛至 `960px`，白底浅灰顶底栏，去除多层卡片嵌套。
  2. **第一步（核对车牌、资料与抓拍）**：
     - 顶部浅灰高对比摘要栏：左车牌（`22px / 700`），右本次采集重量（`30px / 700` 等宽）；
     - 业务核对改为白底细线分隔的紧凑描述网格：标签 `13px` 灰字、值 `15px / 600`（行高 `1.45`），回磅首磅依据与预计净重轻量高亮；
     - 抓拍图区域给足可视高度（`140~160px` 缩略图），确保 1366×768 首屏完整显示在固定底栏上方。
  3. **第二步（回磅扣减）**：
     - 顶部压缩为单行紧凑上下文条（车牌 + 方向 + 物料 + 供收方，保留完整可检索文字）；
     - 原始毛重 / 原始皮重 / 调整前净重三列对齐（`20px / 600`）；
     - `NetAdjustmentFields` 每条扣减项重排为表格式紧凑单行（原因 | 方式 | 数值 | 折算量 | 删除按钮），避免文字与按钮拥挤错位；
     - 底部浅蓝汇总条突出最终出单净重（`28px / 700`）。
  4. 同步更新 `web/tests/review-final.mjs` 中过时的 22px/28px 方框断言，新增新字号阶梯与首屏可见性断言。
- **验收标准**：
  - 弹窗内字号严格收敛到 Spec 6 档阶梯，长物料/长企业名换行整齐不糊字。
  - `1920×1080` 与 `1366×768` 首屏可见抓拍图与最终出单净重；`1024px` 与 `390px` 无横向溢出且底栏主按钮在视口内。
  - `task frontend:lint`、`task frontend:build` 通过。
  - `npm run test:review-final`、`npm run test:net-adjustment`、`npm run test:weighing` 通过。

---

### T4：`[Workbench UI] 其余工作台弹窗与抽屉统一浅色与排版（运输资料/称重完成/人工补单/过磅单抽屉/临时主体）`

- **建议标签**：`ready-for-agent`
- **前置依赖**：T1（建议在 T3 之后执行以复用改造后的 `NetAdjustmentFields`）
- **目标**：补齐工作台剩余 5 个弹窗/抽屉的排版、宽度与浅色一致性，消除巨幅横幅、单列长表单与暗色残留。
- **涉及文件**：
  - `web/src/pages/weighing/workbench.tsx`（查看运输资料弹窗、称重完成弹窗）
  - `web/src/pages/weighing/manual-supplement-dialog.tsx`
  - `web/src/pages/weighing/ticket-drawer.tsx`
  - `web/src/pages/weighing/temporary-party-dialog.tsx`
  - `web/src/pages/weighing/workbench.css`
- **核心改动**：
  1. **查看运输资料弹窗**：宽度从 `1200px` 收敛为 `760px`，去掉内部多余 `ContentCard`；顶部紧凑摘要栏放车牌（`22px`）与首磅重量/时间（`24px`，移除 44px 居中巨幅横幅）；正文用干净的 `15px` 双列键值表展示基本与补充信息。
  2. **称重完成弹窗**：保持 `480px~520px`，顶部浅绿成功摘要块突出车牌（`20px`）与核心重量（`24px`），下方对齐展示任务号/单号与时间，保留打印区与双操作按钮。
  3. **人工补单弹窗**：宽度扩至 `700px`，重组为三区配对网格（基本资料双列 / 首磅重量+首磅时间同行、回磅重量+回磅时间同行 / 扣减与补单原因 + 底部 `24px` 实时出单净重汇总），保留 `aria-label="出单核对"` 区域及所有字段标签。补单成功回执对齐称重完成弹窗风格。
  4. **过磅单抽屉 & 临时主体登记弹窗**：彻底清理暗色覆盖样式，恢复后台一致的浅色表格、筛选栏、浅色单选卡片与浅琥珀同名核对提示。
- **验收标准**：
  - 所有 5 个弹窗/抽屉视觉与字号体系和 T1~T3 完全一致，无暗色样式残留。
  - `1366×768`、`1024×768`、`390×844` 下均无横向溢出，表单校验与焦点行为不变。
  - `task frontend:lint`、`task frontend:build` 通过。
  - `npm run test:manual-supplement`、`npm run test:weighing:ticket-drawer`、`npm run test:weighing:lookup` 通过。

---

### T5：`[Workbench UI] 规范文档收敛、无用暗色素材清理与全量工作台回归验证`

- **建议标签**：`ready-for-agent`
- **前置依赖**：T2、T3、T4
- **目标**：更新权威设计文档以替代旧暗黑工业规范，清理不再使用的暗色素材与死 CSS，执行全量工作台浏览器回归与多分辨率视觉核查，并如实记录软件验证结果与现场验收边界。
- **涉及文件**：
  - `web/design-system/pages/weighing.md`
  - `docs/requirements/README.md`
  - `docs/next.md`
  - `web/public/weighbridge/`（仅在确认无其他页面或测试引用后清理废弃暗色素材）
  - `web/src/pages/weighing/workbench.css`（最终清理死选择器与重复媒体查询）
- **核心改动**：
  1. 按 `docs/development-baseline.md` 维护规则，直接更新 `web/design-system/pages/weighing.md` 的「独立工作台视觉」「核对、失效与反馈」「预览与证据」正文，移除已失效的暗黑主题条款。
  2. 检查仓库内对 `industrial-grid.svg`、`scale-mark.svg` 及废弃 CSS 类名的全部引用，清理确认无引用的暗色冗余代码。
  3. 完整运行前端检查与工作台全量测试套件，并在 `1920×1080`、`1366×768`、`1280×720`、`1024×768`、`390×844` 截屏复核主界面与关键弹窗。
  4. 在 `docs/next.md` 顶部补充本次改版的实现内容、实际执行的验证清单及“未做现场实车/真实仪表/真实摄像头/实物打印验收”的边界声明。
- **验收标准**：
  - `task frontend:lint`、`task frontend:build` 全部通过。
  - 全量工作台相关 Playwright 测试通过（`test:weighing`、`test:net-adjustment`、`test:review-final`、`test:weighing:agent-capture`、`test:weighing:camera-live`、`test:weighing:camera-multiview`、`test:weighing:lookup`、`test:weighing:ticket-drawer`、`test:manual-supplement`、`node tests/plate-candidates.mjs`）。
  - 文档链接有效、无过时冲突条款。
