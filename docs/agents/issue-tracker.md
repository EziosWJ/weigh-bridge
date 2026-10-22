# GitHub 工作项

本仓库使用 GitHub Issues，目标仓库为 EziosWJ/weigh-bridge。
通过 gh 操作，显式指定 --repo EziosWJ/weigh-bridge。

## 操作约定

读取或实施旧工作项时，先用[当前开发基线](../development-baseline.md)核对范围；旧正文及标签不覆盖已替代条款。新建当前阶段工作项须链接[本厂自治规格](../requirements/standalone-site-spec.md)及 ADR 0023／0024，分别说明目标、已有实现和待验收项。

- 读取：gh issue view <number> --repo EziosWJ/weigh-bridge --comments
- 列表：gh issue list --repo EziosWJ/weigh-bridge --state open --json number,title,labels
- 创建：gh issue create --repo EziosWJ/weigh-bridge --title "<标题>" --body-file <正文文件>
- 评论：gh issue comment <number> --repo EziosWJ/weigh-bridge --body-file <正文文件>
- 标签：gh issue edit <number> --repo EziosWJ/weigh-bridge --add-label "<标签>" 或 --remove-label "<标签>"
- 关闭：gh issue close <number> --repo EziosWJ/weigh-bridge

多行正文写入文件并使用 --body-file，保留实际换行。
发布、评论、修改和关闭工作项均在用户授权的任务范围内执行。

技能要求“发布到 issue tracker”时，创建 GitHub Issue；
要求“读取相关 ticket”时，读取对应 Issue 及评论。
标签角色按 docs/agents/triage-labels.md 映射。

## 来源边界

历史 base-project-golang Issue 链接属于脚手架来源，保留其仓库身份，
不能映射成目标仓库的同号 Issue。
不得自动创建远程仓库或向来源仓库发布内容。

## Pull requests as a triage surface

**PRs as a request surface: no.**
