# base-api 脚手架改造记录

## 改造前状态

```
base-api/
├── CLAUDE.md           # 指向 AGENTS.md、docs/agents/ 的引用
├── AGENTS.md           # 85 行通用开发规范 + 项目文档引用规则
├── README.md           # 项目说明
├── skills-lock.json    # skill 锁定文件
├── doc/                # 8 个文件 — 开发约束、项目架构、API 对接、反馈记录
├── docs/               # 3 个文件 — agent 配置（domain、issue-tracker、triage-labels）
├── task/               # 13 个文件 — 01~13 编号任务拆解
├── test/               # 2 个文件 — 测试报告
├── experience/         # 1 个文件 — 测试踩坑记录
├── .wangjian/          # 个人笔记（架构设计、计划等）
└── src/                # 源码保留
```

## 改造目标

- 简化上下文文件，减少 agent 读取负担
- 提炼可复用经验，删除特定业务文档
- CLAUDE.md 与 AGENTS.md 建立软链接

## 改造步骤

### 1. 分析文档价值

| 目录 | 文件数 | 判断 |
|------|--------|------|
| doc/ | 8 | `development-constraints.md` 和 `ai-project-prompt.md` 有架构参考价值，其余特定于旧项目 |
| docs/ | 3 | agent 配置（issue-tracker、triage-labels、domain），属于旧协作流程 |
| task/ | 13 | 全部是旧开发过程记录，无复用价值 |
| test/ | 2 | 旧测试报告，无复用价值 |
| experience/ | 1 | 测试踩坑记录有通用价值，已提炼 |

### 2. 创建 experience/LESSONS.md

从 doc/、experience/ 中提炼关键经验，约 80 行：
- 项目架构（单 Spring Boot 模块化单体）
- 开发约束（REST 规范、批量删除、树结构、内置数据保护）
- 数据库设计注意（逻辑删除 + 唯一索引冲突）
- 测试经验（时间戳前缀、@AfterAll 清理、四层源码确认）
- 认证与鉴权（Sa-Token Bearer、滑动续期）
- 统一响应格式
- 系统配置模块注意点

### 3. 简化 AGENTS.md

从 85 行精简到 ~20 行，保留：
- 技术栈声明
- 核心开发原则
- 关键约束引用
- 指向 LESSONS.md 的引用

### 4. 建立软链接

`CLAUDE.md` → `AGENTS.md`，避免内容重复。

### 5. 删除冗余文件

删除：doc/、docs/、task/、test/、.wangjian/、README.md、skills-lock.json、旧 experience/testing-lessons.md

## 改造后状态

```
base-api/
├── CLAUDE.md           # → AGENTS.md（软链接）
├── AGENTS.md           # ~20 行精简规范
├── experience/
│   └── LESSONS.md      # ~80 行提炼经验
└── src/                # 源码保留
```

## 改造效果

- 删除 28 个文件，减少 ~3000+ 行上下文
- 核心经验保留在 LESSONS.md（~80 行）
- agent 读取负担大幅降低
