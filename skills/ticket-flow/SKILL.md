---
name: ticket-flow
description: "按已确认顺序，使用独立 Subagent 串行实现 to-tickets 产出的 Ticket；每个后续 Ticket 从前一个 Ticket 的完成 commit 创建独立 worktree，并在实现、验证、代码审查后提交。"
disable-model-invocation: true
---

# 使用 Subagent 实现 Ticket

按照下面的顺序驱动串行 Ticket 实现协议执行。

## 执行模式

默认使用**自动串行模式**：按照已确认顺序连续完成所有指定 Ticket。一个 Ticket 成功后，当前代理接收、核验并记录完成报告，不停下来等待用户确认，直接创建并启动下一个 Ticket。

如果用户在开始时明确声明“自动执行所有 Ticket”或同等意思，必须采用自动串行模式。除非发生失败、阻塞、依赖歧义、检查未通过或需要用户作出决策，否则不得中途提问或暂停。

只有用户明确要求“每个 Ticket 完成后暂停审阅”时，才切换为逐 Ticket 审阅模式；该模式下才需要在每个完成报告后等待用户指令。

自动串行模式包含有限 API 重试：调用失败时，由直接管理该调用的上一级代理负责间隔重试，最多 5 次。

## 一、执行主体

1. 必须使用 Subagent 实现 Ticket。
2. 每一个 Ticket 必须由一个独立的 Subagent 处理。
3. 当前代理只负责：
   - 解析 Ticket 及其依赖关系；
   - 按顺序创建 worktree；
   - 派发和监督 Subagent；
   - 验证 Subagent 的完成报告、commit 和检查结果；
   - 汇总执行结果。
4. 当前代理不得直接修改代码，也不得代替 Subagent 实现 Ticket。
5. 不使用 Teammate，也不要并行执行多个 Ticket。

### 责任边界

顶层执行关系固定为：

```text
主代理
└── Ticket Subagent A：实现一个 Ticket
    ├── TDD 子代理（如被实现流程创建）
    └── code-review 子代理（如被实现流程创建）
```

1. 每个代理只管理自己的直属子代理，不越级管理。
2. 子代理的结果和问题直接反馈给直属代理，由直属代理处理并汇总。
3. API 调用失败时，由直属上级间隔重试，最多 5 次；重试复用原 Ticket、worktree、分支和上下文。
4. 只有直属代理无法解决问题时，才向上一级报告；主代理只接收顶层 Ticket Subagent 的结果。

## 二、Ticket 执行前提

开始执行前必须明确：

1. 本次待实现的全部 Ticket 及其真实编号；
2. Ticket 的执行顺序。

任一项未知或存在歧义，都不得开始任务，必须先向用户确认。确认后的顺序就是执行链：每个后续 Ticket 默认继承前一个 Ticket 的完成 commit。

## 三、依赖与执行顺序

1. 按照已确认的 Ticket 列表和执行顺序开始执行。
2. 第一个 Ticket 从任务开始时锁定的基准 commit 创建；每个后续 Ticket 都从前一个 Ticket 的完成 commit 创建新的 worktree 和分支。
3. 同一时间只能有一个 Subagent 工作。
4. 只有当前 Ticket 完成并通过规定检查后，才能开始下一个 Ticket。
5. 如果某个 Ticket 的顶层 Subagent A 失败、阻塞、测试失败、类型检查失败，或无法解决其后代子代理报告的问题：
   - 立即停止后续 Ticket；
   - 不要跳过该 Ticket；
   - 不要自行改变执行顺序；
   - 报告阻塞原因并等待用户决定。
6. 不得为了建立执行链而合并到主分支。
7. 如果 Ticket 缺失、顺序不明确或无法从前一个完成 commit 创建 worktree，先停止并报告，不要猜测。

## 四、Subagent 的强制参考文件

每个 Subagent 在开始代码工作前，必须读取本 Skill 的 `references/implement-skill.md`。派发时提供该文件的绝对路径；读取失败则不得开始实现。

## 五、Worktree 与分支规则

1. 不得在当前分支或当前工作区直接修改代码。
2. 每一个 Ticket 必须使用独立的 worktree 和独立分支。
3. 创建 worktree 前，先解析仓库根目录、当前基准 commit、Ticket 真实编号和分支安全名称。
4. worktree 创建失败时，立即停止，不得退回当前工作区执行。
5. 每个 worktree 和分支都必须保留，方便用户审阅。
6. 不得自动删除 worktree 或分支。
7. 分支命名必须包含 Ticket 的真实编号和简短名称，例如：

```text
ticket/ABC-123-add-refund-flow
```

8. worktree 路径必须明确、唯一，并记录在报告中，例如：

```text
<repo-root>/../worktrees/ticket-ABC-123-add-refund-flow
```

9. 第一个 Ticket 从流程开始时锁定的基准 commit 创建；每个后续 Ticket 都从前一个 Ticket 的完成 commit 创建，不得隐式回到初始基准或跳过前一个 Ticket。

## 六、派发 Ticket Subagent

每个 Ticket 使用新的独立 Subagent。派发前必须提供：

- 当前 Ticket 的完整内容或权威来源；
- Ticket 编号和标题；
- worktree 路径和分支名；
- `references/implement-skill.md` 的绝对路径。

派发提示：

```text
你负责实现 Ticket <编号>：<标题>。

Ticket 内容：<完整正文或权威来源>
工作目录：<worktree 路径>
实现参考：<implement 参考文件绝对路径>

先读取实现参考文件，再只在指定 worktree 中完成当前 Ticket。
你负责管理自己创建的后代子代理；它们的问题先由你处理。
完成后创建 commit，并返回本 Skill 规定的完成或阻塞报告。
```

## 七、实现与验证要求

每个 Subagent 只需读取并严格遵循 `references/implement-skill.md`，其中已经包含实现、TDD、验证、code review 和 commit 要求。

主代理只负责核验：

- 参考文件已成功读取；
- Ticket worktree 中存在正确的 commit；
- Subagent 已返回实现和验证结果；
- 当前工作区没有被修改。

当前代理只核验顶层 Subagent 汇总后的结果，不直接处理后代子代理的中间报告。

## 八、禁止的自动操作

在用户明确审阅并授权之前，禁止：

1. merge 到当前分支；
2. cherry-pick 到当前分支；
3. rebase 当前分支；
4. push 到远程仓库；
5. 删除或清理 worktree；
6. 自动关闭 Ticket；
7. 自动修改 Ticket 状态、标签或评论；
8. 修改、关闭或更新父级 PRD；
9. 自动开始不在当前执行范围内的 Ticket；
10. 让当前代理直接修改 Ticket 代码以“帮助”Subagent 完成工作。

## 九、父级 PRD 规则

父级 PRD 只作为上下文和依赖来源：

- 不把父级 PRD 当作待实现 Ticket；
- 不修改父级 PRD 的内容；
- 不关闭父级 PRD；
- 不改变父级 PRD 的状态；
- 不向父级 PRD 添加评论、标签或进度信息；
- 除非用户明确授权，否则只读取，不写入。

## 十、每个 Ticket 的完成报告

每个 Ticket 完成后，必须返回：

### Ticket <真实编号> 完成报告

- Ticket 标识：
- Ticket 标题：
- 执行顺序：
- 依赖关系：
- Worktree 路径：
- 分支名称：
- Commit：
- 修改的文件：
- 实现摘要：
- 相关测试结果：
- 后代子代理及其结果摘要：
- 遗留问题：
- 建议审阅重点：

顶层 Subagent 必须先向主代理返回完成报告。自动串行模式下，主代理核验并记录后直接启动下一个 Ticket，不等待用户确认；逐 Ticket 审阅模式下，才等待用户确认。

## 十一、失败报告

如果 Ticket 失败或阻塞，必须返回：

### Ticket <真实编号> 阻塞报告

- Ticket 标识：
- Ticket 标题：
- 执行顺序：
- 依赖关系：
- 阻塞原因：
- API/工具重试次数与最终结果：
- 后代子代理阻塞处理结果：
- 是否修改过文件：
- 是否创建过 commit：
- Worktree 路径：
- 分支名称：
- 已执行的检查：
- 错误信息：
- 是否已停止后续 Ticket：是
- 等待用户决定：是

## 十二、全部完成后的行为

当所有指定 Ticket 都完成后：

1. 保留全部 worktree、分支和 commit；
2. 不合并任何变更；
3. 不关闭任何 Ticket 或父级 PRD；
4. 返回总报告，简略汇总每个 Ticket 的实现、验证结果、worktree、分支和 commit；
5. 给出每个 worktree 和 commit 的审阅入口；
6. 明确说明当前分支没有被修改；
7. 停止执行并等待用户审阅；
8. 不要在用户明确授权前继续合并、清理或发布。

## 十三、执行前提

如果 Ticket 内容、执行顺序、初始基准 commit、Issue Tracker 状态或 worktree 位置存在歧义，必须在开始前询问用户。不要用猜测替代缺失的决策。
