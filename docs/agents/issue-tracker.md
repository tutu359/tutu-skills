# Issue 跟踪：GitHub

本仓库的 Issue 和 PRD 记录在 GitHub Issues 中。所有操作都使用 `gh` CLI。

## 约定

- **创建 Issue**：`gh issue create --title "..." --body "..."`。多行正文使用 heredoc。
- **读取 Issue**：`gh issue view <number> --comments`，使用 `jq` 过滤评论，并同时获取标签。
- **列出 Issue**：`gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'`，并根据需要使用 `--label` 和 `--state` 过滤。
- **评论 Issue**：`gh issue comment <number> --body "..."`
- **添加或移除标签**：`gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **关闭 Issue**：`gh issue close <number> --comment "..."`

从 `git remote -v` 推断仓库信息；在仓库克隆目录中运行时，`gh` 会自动完成这项工作。

## 将 Pull Request 作为 triage 请求面

**PR 作为请求面：否。**（如果本仓库把外部 PR 视为功能请求，可将其改为 `yes`；`/triage` 会读取此配置。）

如果改为 `yes`，PR 将使用与 Issue 相同的标签和状态，并调用对应的 `gh pr` 命令：

- **读取 PR**：`gh pr view <number> --comments` 和 `gh pr diff <number>`。
- **列出用于 triage 的外部 PR**：`gh pr list --state open --json number,title,body,labels,author,authorAssociation,comments`，然后只保留 `authorAssociation` 为 `CONTRIBUTOR`、`FIRST_TIME_CONTRIBUTOR` 或 `NONE` 的 PR（排除 `OWNER`、`MEMBER` 和 `COLLABORATOR`）。
- **评论、添加或移除标签、关闭**：分别使用 `gh pr comment`、`gh pr edit --add-label` / `--remove-label`、`gh pr close`。

GitHub 的 Issue 和 PR 共用同一编号空间，因此单独的 `#42` 可能指 Issue，也可能指 PR——先用 `gh pr view 42` 判断；如果失败，再使用 `gh issue view 42`。

## 技能要求“发布到 issue tracker”时

创建一个 GitHub Issue。

## 技能要求“获取相关 ticket”时

运行 `gh issue view <number> --comments`。

## Wayfinding 操作

供 `/wayfinder` 使用。**map** 是一个 Issue，**child** Issue 作为 ticket。

- **Map**：创建一个带有 `wayfinder:map` 标签的 Issue，正文包含 Notes / Decisions-so-far / Fog。使用 `gh issue create --label wayfinder:map`。
- **Child ticket**：创建通过 GitHub sub-issue 端点（`gh api`）关联到 map 的 Issue。如果未启用 sub-issue，则在 map 正文中加入 ticket 任务列表，并在 child 正文开头写入 `Part of #<map>`。标签使用 `wayfinder:<type>`（`research` / `prototype` / `grilling` / `task`）。认领后，将 ticket 分配给负责推进的开发者。
- **阻塞关系**：使用 GitHub 的**原生 Issue 依赖**作为规范且在界面可见的表示。使用 `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>` 添加依赖，其中 `<blocker-db-id>` 是阻塞 Issue 的数字 **database id**（使用 `gh api repos/<owner>/<repo>/issues/<n> --jq .id` 获取，_不是_ `#number` 或 `node_id`）。GitHub 会通过 `issue_dependencies_summary.blocked_by` 报告未关闭的阻塞项，这是实时门槛。如果无法使用依赖功能，则退回到在 child 正文顶部添加 `Blocked by: #<n>, #<n>`。当所有阻塞项都已关闭时，ticket 才算解除阻塞。
- **Frontier 查询**：列出 map 的开放 child（使用 `gh issue list --state open`，范围限定为 map 的 sub-issue / 任务列表），排除存在开放阻塞项（`issue_dependencies_summary.blocked_by > 0`，或 `Blocked by` 行中列出的 Issue 仍开放）或已有 assignee 的 child；按 map 中的顺序选择第一个。
- **认领**：`gh issue edit <n> --add-assignee @me`——这是本次会话的第一次写操作。
- **解决**：使用 `gh issue comment <n> --body "<answer>"` 回复，然后运行 `gh issue close <n>`，最后将上下文指针（gist + link）追加到 map 的 Decisions-so-far 中。
