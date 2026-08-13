# 领域文档

说明工程技能在探索代码库时应如何使用本仓库的领域文档。

## 开始探索前读取以下内容

- 根目录的 **`CONTEXT.md`**；或者
- 如果根目录存在 **`CONTEXT-MAP.md`**，则读取它——它会指向每个 context 对应的 `CONTEXT.md`。根据当前主题读取所有相关文件。
- **`docs/adr/`**——读取与当前工作区域相关的 ADR。在 multi-context 仓库中，还要检查 `src/<context>/docs/adr/` 中按 context 划分的决策。

如果这些文件不存在，**静默继续**。不要专门提示它们缺失，也不要建议提前创建。`/domain-modeling` 技能（由 `/grill-with-docs` 和 `/improve-codebase-architecture` 触发）会在实际解决术语或决策时按需创建这些文件。

## 文件结构

Single-context 仓库（大多数仓库）：

```
/
├── CONTEXT.md
├── docs/adr/
│   ├── 0001-event-sourced-orders.md
│   └── 0002-postgres-for-write-model.md
└── src/
```

Multi-context 仓库（根目录存在 `CONTEXT-MAP.md` 时）：

```
/
├── CONTEXT-MAP.md
├── docs/adr/                          ← 系统级决策
└── src/
    ├── ordering/
    │   ├── CONTEXT.md
    │   └── docs/adr/                  ← context 级决策
    └── billing/
        ├── CONTEXT.md
        └── docs/adr/
```

## 使用术语表中的词汇

当输出中提到领域概念（例如 Issue 标题、重构提案、假设或测试名称），使用 `CONTEXT.md` 中定义的术语。不要改用术语表明确避免的同义词。

如果需要的概念尚未出现在术语表中，这是一个信号：要么你正在创造项目未使用的语言（请重新考虑），要么项目确实存在知识缺口（请记录下来，交由 `/domain-modeling` 处理）。

## 标记 ADR 冲突

如果输出内容与现有 ADR 冲突，请明确指出，不要静默覆盖：

> _与 ADR-0007（event-sourced orders）冲突——但值得重新讨论，因为……_
