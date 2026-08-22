---
name: ticket-flow
description: "按已确认顺序，使用独立 Ticket Subagent 串行实现 to-tickets 产出的 Ticket；每个后续 Ticket 从前一个完成 commit 创建独立 worktree，并保留全部审阅入口。"
---

# Ticket Flow 主代理编排协议

主代理窗口只负责 Ticket 编排，不实现 Ticket，不做 TDD、测试、类型检查或 code review，不读取后代子代理的中间内容。每个 Ticket 由一个独立的直属 Ticket Subagent（A）完成；A 的固定工作协议见 [`references/ticket-subagent.md`](references/ticket-subagent.md)，实现方法见 [`references/implement-skill.md`](references/implement-skill.md)。

## 执行模式

默认自动串行执行：按已确认顺序完成全部 Ticket。每个 Ticket 通过主代理核验后，立即进入下一个，不等待用户确认。

只有用户明确要求每个 Ticket 完成后暂停审阅时，才使用逐 Ticket 审阅模式。

## 开始前必须确认

在创建任何 worktree 或派发 Agent 前，主代理必须明确：

1. 全部 Ticket 的真实编号、标题和权威内容；
2. Ticket 执行顺序及依赖关系；
3. 当前仓库根目录、当前分支和启动时基准 commit；
4. 项目名（取仓库根目录 basename）、以及主代理为每个 Ticket 创建并管理的主 worktree 唯一路径和安全分支名；
5. 项目根目录的记录文件路径，默认是 `<repo-root>/ticket-flow-record.md`。

Ticket 或顺序存在歧义时先向用户确认，不得猜测或跳过。

## 编排流程

对每个 Ticket 严格串行执行：

1. 锁定基准 commit SHA。第一个 Ticket 使用流程开始时锁定的 commit；每个后续 Ticket 只使用前一个 Ticket 的完成 commit SHA，不从前一个 worktree 路径推导。
2. 在主仓库同级目录的 `worktrees/<项目名>/` 下创建当前 Ticket 的主 worktree 和分支。项目名取仓库根目录 basename，路径格式为 `<repo-parent>/worktrees/<项目名>/<ticket目录>`；创建失败时停止，不得退回当前工作区。
3. 启动一个新的 Ticket Subagent A，并只提供：Ticket 完整内容、编号和标题、worktree 路径、分支名、`ticket-subagent.md` 绝对路径、`implement-skill.md` 绝对路径。
4. A 开始后，主代理只等待 A 的完成报告或阻塞报告。不得接收、追问、转发或处理 A 的后代子代理消息。
5. A 的 Agent/API 调用失败由主代理负责重试，总共最多 5 次。每次重试复用同一个 Ticket、worktree、分支和上下文，并记录 `第 N/5 次尝试`。第 5 次仍失败则生成阻塞记录并停止后续 Ticket。
6. A 返回完成报告后，主代理只核验自己管理的主 worktree：报告对应 Ticket、该主 worktree 和分支存在、完成 commit 位于预期基线之上，以及 A 已给出实现检查结果。主代理不扫描或核验其他代理创建的 worktree，不重新执行实现细节，也不审查后代报告。
7. 将 A 的完成或阻塞报告摘要追加到项目根目录的 `ticket-flow-record.md`。记录文件不纳入 Git。
8. 只有完成报告核验通过后，才创建下一个 Ticket 的 worktree。

## Worktree、分支和 commit

- 主代理为每个 Ticket 使用一个独立的主 worktree 和独立分支；该主 worktree、分支和完成 commit 全部保留。
- 其他代理可以按自己的工作需要创建额外 worktree、分支或临时 commit；这些资源不属于主代理管理范围，不参与 Ticket 完成核验，也不作为下一个 Ticket 的基线。
- 分支名必须包含真实 Ticket 编号和短名称，例如 `ticket/ABC-123-add-refund-flow`。
- 主 worktree 路径必须明确、唯一，并记录在报告中。目录格式为 `<repo-parent>/worktrees/<项目名>/<ticket目录>`，例如 `<repo-root>/../worktrees/tutu-skills/ticket-ABC-123-add-refund-flow`。
- 主代理不得在当前分支或任何 Ticket worktree 中修改业务代码。
- 不得为了建立执行链而 merge、cherry-pick 或 rebase。

## 直属责任和越界防护

关系固定为：

```text
主代理
└── Ticket Subagent A
    └── A 的后代子代理（可选）
```

- 主代理只管理顶层 A；A 只管理自己的直属后代。
- A1/TDD/code-review/扫描 Agent 的结果必须先回到 A，由 A 汇总成摘要。
- 后代不得直接向主代理发送消息或报告。若运行环境无法可靠保证该路由，A 不得创建后代子代理；这条消息路由规则与 worktree 是否由后代创建无关。
- 主代理不得要求后代解释、补充或重做工作；所有问题返回 A，由 A 处理。
- 业务代码中的 HTTP 重试规则与 Agent/API 调用失败的调度重试规则完全分离，主代理只处理后者。

## 禁止的自动操作

在用户明确审阅并授权前，主代理及 A 均不得：

- merge、cherry-pick、rebase、push；
- 删除或清理 worktree、分支或 commit；
- 关闭 Ticket，或修改 Ticket/父级 PRD 的状态、标签、评论和内容；
- 自动开始执行范围外的 Ticket；
- 跳过失败或阻塞的 Ticket；
- 让主代理直接修改 Ticket 代码以帮助 A 完成工作。

## 停止条件和总报告

任一 Ticket 缺失、顺序不明、主 worktree 创建失败、A 第 5 次调用仍失败、A 返回最终阻塞、完成报告核验失败或主代理管理的主 worktree 状态异常，立即停止后续 Ticket，并在记录文件中写入阻塞报告，等待用户决定。其他代理创建的 worktree、分支、临时 commit 或元数据目录不触发停止条件。

全部 Ticket 完成后：

1. 保留主代理创建的全部 Ticket worktree、分支和完成 commit，不合并任何变更；
2. 在记录文件中追加总执行状态；
3. 返回总报告，汇总每个 Ticket 的编号、顺序、worktree、分支、commit 和 A 的验证摘要；
4. 明确当前主分支没有被修改，并给出各 worktree 和 commit 的审阅入口；
5. 停止执行，等待用户审阅。
