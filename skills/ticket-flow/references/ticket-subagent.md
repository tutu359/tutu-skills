# Ticket Subagent

直属 Ticket Subagent（A）每次只负责一个 Ticket，并在主代理指定的 worktree 和分支中工作。A 必须先核对 Ticket 编号和内容、主 worktree、当前分支和基线 commit。发现不一致时，先向主代理报告。

开始代码工作前，A 读取 [implement-skill.md](implement-skill.md)，并按其中步骤实现。

## 后代代理

A 可按需要创建后代代理。后代的问题和结果向其直属上级汇报；主代理只接收 A 的最终摘要。后代可创建自己的辅助 worktree，A 只核验当前 Ticket 的主 worktree。

A 在提交完成报告或阻塞报告前，必须确认所有后代都已结束或停止。只要仍有后代在运行、等待、重试或未收敛，A 就不能提交任何报告。

## 调用失败

Agent/API 调用失败时，由直属上级复用原上下文持续重试，不设固定次数。主代理自身无法继续响应时，整个流程自然停止。

## 完成报告

A 完成实现、检查并提交 commit 后，向主代理发送：

~~~text
### Ticket <编号> 完成报告

- Ticket 编号：
- 完成 commit：
- 实现与检查摘要：
~~~

## 阻塞报告

A 的实现或检查无法继续时，且所有后代都已结束或停止，向主代理发送：

~~~text
### Ticket <编号> 阻塞报告

- Ticket 编号：
- 阻塞原因：
- 已完成工作与检查摘要：
~~~

阻塞后停止后续 Ticket；保留当前 worktree、分支、commit 和已有修改，不自行清理、合并或发布。
