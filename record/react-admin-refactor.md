# react-admin 脚手架改造记录

## 改造前状态

```
react-admin/
├── CLAUDE.md          # 指向 AGENTS.md、docs/agents/ 的引用
├── AGENTS.md          # 300 行完整开发规范（角色、技术栈、规则、禁止项）
├── CONTEXT.md         # 材料写作平台领域词汇表（特定业务）
├── README.md          # 项目说明
├── skills-lock.json   # skill 锁定文件
├── docs/              # 19 个文件 — API 对接文档、RBAC 总结、ADR、agent 配置
├── task/              # 26 个文件 — 00~25 编号任务拆解
├── experience/        # 4 个使用指南（组件、字典、文件管理、task 生成）
└── design-system/     # UI 规范（保留）
```

## 改造目标

- 简化上下文文件，减少 agent 读取负担
- 提炼可复用经验，删除特定业务文档
- CLAUDE.md 与 AGENTS.md 建立软链接，避免重复

## 改造步骤

### 1. 分析文档价值

| 目录 | 文件数 | 判断 |
|------|--------|------|
| docs/ | 19 | 大部分特定于旧项目（材料写作平台），3 个 API 文档有通用参考价值 |
| task/ | 26 | 全部是旧开发过程记录，无复用价值 |
| experience/ | 4 | 内容可提炼为通用经验 |
| docs/adr/ | 5 | 2 个通用模式（useListPage、错误处理），3 个特定领域 |

### 2. 创建 experience/LESSONS.md

从 docs/、experience/、task/ 中提炼关键经验，约 70 行：
- API 对接约定（响应格式、批量删除改 POST、token 续期、isBuiltin 保护）
- 组件使用模式（列表页结构、组件职责边界）
- 字典使用规范（useDictOptions、不硬编码）
- 文件管理流程（上传/下载/预览）
- 通用 ADR 模式（useListPage Hook、错误处理集中化、状态标签单一来源）
- 菜单与路由（后端驱动 + 前端保留本地默认）
- 系统配置模块（configKey 唯一索引、valueType 校验）

### 3. 简化 AGENTS.md

从 300 行精简到 ~20 行，保留：
- 技术栈声明
- 核心开发原则
- UI/UX 规范引用
- 指向 LESSONS.md 的引用

### 4. 建立软链接

`CLAUDE.md` → `AGENTS.md`，避免内容重复。

### 5. 删除冗余文件

删除：docs/、task/、旧 experience/ 4 个文件、CONTEXT.md、README.md、skills-lock.json

## 改造后状态

```
react-admin/
├── CLAUDE.md          # → AGENTS.md（软链接）
├── AGENTS.md          # ~20 行精简规范
├── experience/
│   └── LESSONS.md     # ~70 行提炼经验
├── design-system/     # 保留
└── src/               # 源码保留
```

## 改造效果

- 删除 62 个文件，减少 ~7000 行上下文
- 核心经验保留在 LESSONS.md（~70 行）
- agent 读取负担大幅降低
