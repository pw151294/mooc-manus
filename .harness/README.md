# mooc-manus Harness（后端）

本目录承载 mooc-manus 后端（Go + DDD + Agent 内核）专属约束、知识与剧本。

**继承**：`../mooc-manus-all/.harness/`（跨仓共识、契约、安全 rules）。
具体继承机制：`inherits` 字段由 `scripts/sync-bridges.sh` 在烘焙桥接层时解析；agent 通过桥接层间接消费父仓 rules。

## 关系图

```
mooc-manus-all/.harness/  ← 父（跨仓 rules）
       ↑ inherits
mooc-manus/.harness/      ← 本仓（后端业务 rules）
       ↓ sync-bridges
mooc-manus/CLAUDE.md      ← agent 运行时实际加载
mooc-manus/AGENTS.md
mooc-manus/.cursorrules
```

## 子目录职责
（同总仓 README 七大目录定义）

详细设计见根仓 `docs/superpowers/specs/2026-06-28-harness-doc-architecture-design.md`。
