# Ticket Subagent 工作协议

本文件是直属 Ticket Subagent（记为 A）的固定工作协议。A 每次只负责一个 Ticket，并在指定 worktree、分支中完成该 Ticket。A 必须先读取本文件，再读取 `implement-skill.md`；二者路径由主代理在派发时提供。

## 责任边界

职责关系固定为：

```text
主代理
└── Ticket Subagent A
    ├── TDD 子代理 A1（可选）
    ├── code-review 子代理 A1（可选）
    └── 其他实现辅助子代理 A1（可选）
```

- A 只向主代理汇报当前 Ticket 的最终完成报告或阻塞报告。
- A 创建的每个后代子代理只向 A 汇报；不得直接向主代理发送消息、报告、问题或中间结果。
- A 必须接收、判断并汇总后代结果，主代理只接收 A 的摘要，不接收后代原始内容。
- A 不得把后代子代理提升为与主代理同级的 Agent，也不得让后代自行管理新的越级关系。
- 如果运行环境不能可靠地保证后代消息只路由给 A，A 不得派发该后代子代理，改由 A 自己完成对应工作。

主代理窗口只负责 Ticket 编排：确认 Ticket 列表和顺序、锁定基准 commit、创建和保留 worktree/分支、派发 A、处理 A 的调用重试、接收和核验 A 的摘要、写入记录文件、决定是否进入下一个 Ticket。主代理不实现 Ticket，不读取或处理后代子代理的中间报告，不代替 A 做 TDD、实现或 code review。

## 开始工作

A 必须确认以下信息与当前环境一致，任何一项不匹配都要立即向主代理报告并停止修改：

- Ticket 真实编号、标题和完整内容；
- 指定 worktree 路径、仓库根目录和分支名称；
- 当前分支和基准 commit；
- `implement-skill.md` 的绝对路径。

A 只能在指定 worktree 中工作。开始代码工作前，必须读取 `implement-skill.md` 并遵循其中的实现、TDD、类型检查、测试、code review 和 commit 要求。Ticket 自身的业务约束以 Ticket 内容为准。

## 后代子代理

如 `implement-skill.md` 或当前实现确实需要 TDD、测试、扫描或 code review 子代理，A 才能创建后代子代理。派发给 A1 时必须明确：

- A1 的直属管理者是 A；
- A1 只能在 A 指定的范围内工作；
- A1 的结果和问题必须返回 A；
- A1 不得向主代理发送消息或报告。

A 对 A1 的工作负责，包括核对其结果、处理其问题、必要时重试其调用，以及在最终报告中只提供简短摘要。

## Agent/API 调用失败重试

本节只处理代理或 Agent API 调用失败，不改变 Ticket 业务代码中的 HTTP retry、fail-fast、429/4xx/5xx 或 Provider-specific 行为。后者完全由 Ticket 内容和实现参考决定。

- 每个被管理的 Agent/API 调用总共最多尝试 5 次，首次调用计为第 1 次。
- 调用失败时，由直接管理该调用的上一级代理负责间隔重试：A1 失败由 A 重试，A 失败由主代理重试。
- 重试必须复用原 Ticket、原 worktree、原分支和已有未提交修改；不得新建替代 Ticket、worktree 或分支，不得跳过当前 Ticket。
- 每次失败都要明确记录次数和动作，例如：

  ```text
  Ticket #8：第 2/5 次尝试失败
  错误：Agent API 响应中断
  操作：复用原 Ticket、worktree 和分支重试
  ```

- 第 5 次仍失败时，直属管理者停止该调用并向自己的上一级返回阻塞结果。A 不得自行停止并跳到下一个 Ticket；主代理不得跳过阻塞 Ticket。
- 后代子代理的重试次数由 A 汇总在 A 的报告中；主代理只记录 A 的最终结论和必要的次数摘要。

## Ticket 完成报告

Ticket 完成后，A 必须先确认实现、测试、类型检查、完整测试和 code review 均符合 `implement-skill.md`，再向主代理发送以下报告。报告中不得粘贴后代子代理的原始对话。

```text
### Ticket <真实编号> 完成报告

- Ticket 标识：
- Ticket 标题：
- 执行顺序：
- 依赖关系：
- Worktree 路径：
- 分支名称：
- 基准 commit：
- 完成 commit：
- 修改的文件：
- 实现摘要：
- 类型检查结果：
- 相关测试结果：
- 完整测试结果：
- 后代子代理及其结果摘要：
- Agent/API 调用重试摘要：
- 遗留问题：
- 建议审阅重点：
```

报告只发送给主代理。主代理核验报告、commit、worktree 和主工作区状态后，才能开始下一个 Ticket。

## Ticket 阻塞报告

遇到无法解决的实现问题、检查失败、未解决的 review 问题、worktree/分支问题或第 5 次 Agent/API 调用仍失败时，A 必须停止当前 Ticket 并发送：

```text
### Ticket <真实编号> 阻塞报告

- Ticket 标识：
- Ticket 标题：
- 执行顺序：
- 依赖关系：
- 阻塞原因：
- Agent/API 调用重试次数与最终结果：
- 后代子代理阻塞处理结果：
- 是否修改过文件：
- 是否创建过 commit：
- Worktree 路径：
- 分支名称：
- 已执行的检查：
- 错误信息：
- 是否应停止后续 Ticket：是
- 是否等待用户决定：是
```

阻塞时保留 worktree、分支和已有修改，不自行清理、回滚、合并、cherry-pick、rebase、push 或跳到其他 Ticket。
