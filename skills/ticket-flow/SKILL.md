---
name: ticket-flow
description: "按已确认顺序，使用独立 Ticket Subagent 串行实现 to-tickets 产出的 Ticket；每个后续 Ticket 从前一个完成 commit 创建独立 worktree，并保留全部审阅入口。"
---

# Ticket Flow

主代理只负责编排，不直接修改 Ticket 代码，也不代替 A 实现、测试、类型检查或 code review。A 的执行协议见 [references/ticket-subagent.md](references/ticket-subagent.md)，实现步骤见 [references/implement-skill.md](references/implement-skill.md)。

## 开始前

主代理先确认：

- 要执行的 Ticket 编号、完整内容和顺序；
- 仓库根目录、当前分支和当前 commit。

默认按确认顺序自动串行执行；用户要求逐个审阅时，每个 Ticket 完成后暂停。

## 执行

对每个 Ticket：

1. 第一个 Ticket 从任务开始时的当前 commit 创建；后续 Ticket 从前一个 Ticket 的完成 commit 创建。
2. 在以下位置创建该 Ticket 的独立主 worktree 和分支：
   <repo-parent>/worktrees/<project-name>/<ticket-directory>
3. 成功创建独立 worktree 后，派发一个 A，并提供 Ticket 编号与内容、主 worktree 路径，以及两个参考文件的路径。
4. 主代理只等待 A 的最终完成报告或阻塞报告，不处理后代代理的消息。
5. A 的 Agent/API 调用失败时，由主代理复用原上下文持续重试。其他后代调用失败时，由其直属上级重试。
6. A 报告完成后，主代理核验：Ticket 与主 worktree 对应、HEAD 等于报告的完成 commit、工作区干净，且 A 已确认所有后代都已结束或停止。核验通过后才进入下一个 Ticket。
7. 将每个 Ticket 的最终状态追加到：
   <repo-parent>/worktrees/<project-name>/ticket-flow-record.md

每个 Ticket 必须在自己的 worktree 和分支中开发。A1、A2 等后代可按需创建辅助 worktree；主代理无需管理，A 只关注自己 Ticket 的主 worktree。

## 交付边界

- A 完成 Ticket 后必须提交 commit。
- 不自动执行 merge、cherry-pick、rebase 或 push。
- 不自动删除或清理 worktree、分支、commit 或记录文件。
- 不自动修改 Ticket 或父级 PRD 的状态、标签、评论或内容。

全部 Ticket 完成后，主代理输出总报告，汇总各 Ticket 的编号、worktree、分支、完成 commit 和检查摘要，并说明主分支未被修改。
