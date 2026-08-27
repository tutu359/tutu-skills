---
name: ticket-flow
description: "按已确认顺序，使用独立 Ticket Subagent 串行实现 to-tickets 产出的 Ticket；每个后续 Ticket 从前一个完成 commit 创建独立 worktree，并保留全部审阅入口。"
---

# Ticket Flow

> 默认按确认顺序`自动串行执行`；用户要求逐个审阅时，每个 Ticket 完成后暂停。

## 角色分工

主代理只负责：

- 确认 Ticket 顺序和上下文；
- 创建独立 worktree 和分支；
- 派发 Ticket Implementer；
- 等待直属 Agent 的最终报告；
- 核验 commit、worktree 和工作区状态；
- 记录每个 Ticket 的最终结果。

SubAgent负责：

- 实现一个且仅一个 Ticket；
- 在指定的 worktree 和分支中工作；
- 读取并遵循此文件进行代码实现和复查 `references/implement-skill.md`；
- 提交完成 commit；
- 向主代理发送完成报告或阻塞报告。

主代理只负责编排，不直接修改 Ticket 代码，也不代替 SubAgent实现、测试、类型检查或 code review。

## 开始前

主代理先确认：

- Ticket 编号；
- Ticket 完整内容；
- Ticket 执行顺序；
- 仓库根目录；
- 当前分支；
- 当前基线 commit。

如有不确定的地方，或有其他疑问，不要自行做主，去向用户核对清楚，再进行派发子代理开始工作。

## 执行流程

对每个 Ticket：

1. 第一个 Ticket 从任务开始时的当前 commit 创建；后续 Ticket 从前一个 Ticket 的完成 commit 创建。
2. 在以下位置创建该 Ticket 的独立主 worktree 和分支：
   <repo-parent>/worktrees/<project-name>/<ticket-directory>
3. 成功创建独立 worktree 后，派发一个子代理，并提供 Ticket 编号与内容、主 worktree 路径，以及两个参考文件的路径。
4. 主代理只等待子代理的最终完成报告或阻塞报告，不处理子代理的后代代理的消息。
5. 子代理的 Agent/API 调用失败时，由主代理复用原上下文持续重试。其他后代调用失败时，由其直属上级重试。
6. 子代理报告完成后，主代理必须核验：
   - Ticket 编号与当前 worktree 对应；
   - HEAD 等于报告中的完成 commit；
   - 工作区干净；
   - 当前分支正确；
   - 子代理已确认所有后代代理都已结束或停止。
7. 核验通过后，才允许创建并执行下一个 Ticket。
8. 将每个 Ticket 的最终状态追加到：
   <repo-parent>/worktrees/<project-name>/ticket-flow-record.md

全部 Ticket 完成后，主代理输出总报告，汇总各 Ticket 的编号、worktree、分支、完成 commit 和检查摘要，并说明主分支未被修改。

## Ticket 派发契约

如果当前调用工具支持选择自定义 Agent，主代理必须使用名为 `ticket-implementer` 的 Agent作为子代理执行任务(如果有)。

并提供以下完整上下文：

```text
Agent role: ticket-implementer

Ticket:
- id: <ticket-id>
- content: <full-ticket-content>

Repository:
- root: <repo-root>
- worktree: <primary-worktree>
- branch: <branch>
- base_commit: <base-commit>

Instructions:
- ticket_flow_skill: <absolute-path-to-this-SKILL.md>
- implement_skill: <absolute-path-to-references/implement-skill.md>
```

## SubAgent--执行协议

子代理开始代码工作前必须：

1. 核对 Ticket 编号和完整内容；
2. 核对主 worktree 路径；
3. 核对当前分支；
4. 核对基线 commit；
5. 核对当前工作目录是主代理指定的 Ticket worktree；
6. 读取 `references/implement-skill.md`。

发现任何不一致时，子代理必须先向主代理报告，不得直接开始修改代码。

## SubAgent--SubAgent

子代理可以按需要创建后代代理，code review 必须由独立子代理并使用技能 /code-review 完成，但必须：

- 明确后代代理的任务范围；
- 由自己接收和整理后代结果；
- 负责处理后代调用失败；
- 在提交最终报告前确认所有后代代理都已结束或停止。

只要仍有后代代理处于运行、等待、重试或未收敛状态，子代理就不能提交完成报告或阻塞报告。

## SubAgent--实现

子代理必须按照以下参考文件执行实现流程：

```text
references/implement-skill.md
```

## SubAgent--交付条件

- Ticket 要求的代码实现已实现；
- 必要的单元测试或集成测试已完成；
- 类型检查已完成；
- code review 已通过独立子代理并使用/code-review 技能完成；
- 子代理完成 Ticket 后必须提交 commit。
- 所有后代代理都已结束或停止。
- 向父代理返回规定格式的完成报告或阻塞报告。

## SubAgent--禁止操作

- 修改当前 Ticket 之外的功能；
- 不自动执行 merge、cherry-pick、rebase 或 push。
- 不自动删除或清理 worktree、分支、commit 或记录文件。
- 不自动修改 Ticket 或父级 PRD 的状态、标签、评论或内容。
- 为了绕过阻塞而自行作出未确认的产品、架构、API 或范围决定；
- 把自我检查描述成独立 reviewer 的结果。

## SubAgent--完成报告

子代理完成实现、检查并提交 commit 后，必须发送：

```text
### Ticket <编号> 完成报告

- Ticket 编号：
- 完成 commit：
- 工作worktree：<worktree-path>
- 分支：<branch>
- 实现与检查摘要：
  - <实现摘要>
  - <测试摘要>
  - <子代理code review 摘要>
- 后代代理状态：全部已结束或停止
```

## SubAgent--阻塞报告

当实现或检查无法继续，且所有后代代理都已结束或停止时，子代理必须发送:

```text
### Ticket <编号> 阻塞报告

- Ticket 编号：
- 工作worktree：<worktree-path>
- 分支：<branch>
- 阻塞原因：
- 已完成工作与检查摘要：
- 后代代理状态：全部已结束或停止
```

收到阻塞报告后，主代理必须停止后续 Ticket，保留当前 worktree、分支、commit 和已有修改，不自行清理或合并。
