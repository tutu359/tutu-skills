---
name: ticket-flow
description: "按依赖顺序用独立 Subagent 实现 Tickets，并保留 worktree 供审阅。"
disable-model-invocation: true
---

# Ticket Flow

把本次提供的 Ticket 作为一条**串行执行链**处理。这个 Skill 由用户手动启动一次；启动后，当前代理只做编排，代码由独立 Subagent 实现。

## 目标

- 每个 Ticket 使用一个独立的 Subagent。
- 同一时间只推进一个 Ticket。
- Ticket 编号始终使用外部系统的真实标识符；编号不是执行序号。
- 执行顺序由显式依赖关系和用户给出的顺序决定，不由编号大小决定。
- 每个 Ticket 在独立 worktree 和分支中完成。
- 后续 Ticket 从其依赖 Ticket 的完成 commit 继续；当前分支始终不变。
- 全部完成后保留 worktree、分支和 commit，停止并等待用户审阅。

## 启动前检查

在产生任何代码变更或外部写操作前，完成以下检查：

1. 读取本文件同目录下的 `SUBAGENT.md` 和 `IMPLEMENT.md`。前者是每个实现 Subagent 的执行契约，后者是必须原样遵循的实现规范。
2. 确认当前目录是目标 Git 仓库，并记录当前分支、HEAD commit 和工作区状态。
3. 当前工作区有未提交变更、当前分支不是预期基线、Ticket 内容不完整、依赖关系有歧义、依赖图有环，或无法安全创建独立 Git worktree 时，先向用户一次性说明问题并暂停。
4. 读取本次范围内的 Ticket 和父级 PRD。父级 PRD 只读，用于上下文和依赖，不列入待实现 Ticket。
5. 验证执行链：确认当前唯一可开始的 Ticket，或确认用户给出的 Ticket 顺序。不要根据编号自行排序，也不要补齐缺失编号。
6. 向用户简要列出将要执行的顺序、每个 Ticket 的基线来源和预计分支命名。信息明确时直接开始，不为正常的命名或实现细节重复询问。

## 调度流程

对每一个 Ticket 严格重复以下流程，完成当前 Ticket 后才进入下一个：

1. 计算基线 commit：
   - 无依赖的 Ticket 从启动时记录的基线 HEAD 创建。
   - 只有一个前置依赖的 Ticket 从该依赖的完成 commit 创建。
   - 有多个前置依赖且不能自然落在一个已有 commit 上时暂停并询问；不要自行合并依赖分支。
2. 确定 Ticket 分支名和独立 worktree 路径。路径使用主仓库同级目录，避免把一个 worktree 放进另一个 worktree。
3. 确认目标分支和 worktree 路径尚不存在，然后使用 Git 通用命令创建：

   ```bash
   git worktree add -b <ticket-branch> <worktree-path> <base-commit>
   ```

   如果命令失败，立即停止当前 Ticket，不退回主工作区，也不使用其他目录替代。
4. 将创建好的 worktree 路径、目标分支、基线 commit、`SUBAGENT.md` 和 `IMPLEMENT.md` 的绝对路径、Ticket 完整内容及依赖关系传给一个 Subagent，并要求它在该 worktree 中工作。
5. Subagent 开始修改前必须确认其实际 `pwd`、Git 仓库根目录、分支和基线 commit。路径或分支不匹配时立即停止。
6. 等待 Subagent 完成。不得在它工作时启动另一个 Ticket，也不得由当前代理接手修改代码。
7. Subagent 返回后，由当前代理只读核验：
   - 报告中的 worktree 和分支确实存在；
   - commit 存在且位于预期基线之上；
   - 当前主工作区的 HEAD 和状态与启动时一致；
   - 实现、类型检查、相关测试、最终完整测试和 code review 都有明确结果；
   - code review 的可执行问题已经处理，或已明确阻塞。
8. 核验通过后保留该 Ticket 的 worktree、分支和 commit，并记录其 commit 作为后续依赖的基线。
9. 任意 Ticket 失败、阻塞、测试或类型检查失败、review 存在未解决问题、worktree 丢失，或主工作区发生变化时，立即停止后续执行并返回阻塞报告。

“串行”只约束 Ticket 实现。`/code-review` 内部若按其自身机制启动只读审查者，属于当前 Ticket 的审查步骤；在审查完成前不得启动下一个 Ticket。

## 编号、依赖和范围

- 使用真实 Ticket 标识符，例如 `#7`、`PROJ-42` 或本地追踪器中的 ID；不要改写成 `Ticket 001`。
- 编号不连续是正常情况，不创建不存在的编号，不因编号大小改变顺序。
- 用户指定“当前唯一可开始的 Ticket”时，只从它开始。
- 只有依赖已经完成并通过核验，才可启动后续 Ticket。
- 没有显式依赖时，遵循用户给出的顺序，仍然串行执行。
- 不执行依赖链或本次范围之外的 Ticket。
- 父级 PRD 只读，不修改、不关闭、不改变状态、不加评论、标签或进度。

## Worktree 和分支

- 每个 Ticket 使用 Git `worktree add` 创建独立 worktree 和分支。
- 分支名包含真实 Ticket 标识符和短名称，例如 `ticket-7-provider-openai`。
- worktree 路径使用主仓库同级目录，并在创建前确认路径和分支不存在。
- 每个 worktree 和分支都保留，供用户审阅。
- 当前代理不得进入 Ticket worktree 修改文件。

## 实现契约

每个 Subagent 在实现前必须读取 `IMPLEMENT.md`，并把该文件作为实现、测试、审查和提交要求的唯一来源。不得在派发提示或其他文件中转述、删减或改写这些要求；后续更新以替换后的 `IMPLEMENT.md` 为准。

## 禁止的自动操作

在用户明确审阅并授权前，所有代理都保持以下终态：

- 不 merge 到当前分支；
- 不 cherry-pick 到当前分支；
- 不 rebase 当前分支；
- 不 push；
- 不删除或清理 worktree、分支或 commit；
- 不关闭 Ticket；
- 不修改 Ticket 的状态、标签或评论；
- 不修改、关闭或更新父级 PRD；
- 不自动继续处理范围外 Ticket。

## 完成与阻塞

每个 Ticket 使用 `SUBAGENT.md` 中的完成或阻塞格式报告。所有 Ticket 完成后，汇总真实标识符、顺序、依赖、worktree、分支、commit、测试和 review 结果，列出审阅入口，然后停止。

如果在启动前存在必须由用户决定的歧义，先集中提问并暂停；不要带着不确定的基线、依赖或范围开始修改。
