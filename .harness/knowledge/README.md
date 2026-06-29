# Knowledge - 后端（mooc-manus）

> 本目录承载 **mooc-manus 后端** 的"知识地图"。与 `rules/` 不同的是：
> - `rules/` 回答"必须 / 禁止做什么"（强约束，可静态校验）
> - `knowledge/` 回答"现状是怎样的、为什么这么做"（叙述性，配合代码与图理解架构）

补充父仓 knowledge：跨仓 / 部署 / 子模块工作流详见 `mooc-manus-all/.harness/knowledge/`。

## 索引

| 主题 | 文件 | 关联 rules / ADR |
|------|------|-----------------|
| 4 种 Agent 内部状态机 + 决策树 | [agent-internals.md](./agent-internals.md) | R-43-agent, R-47-memory |
| 工具调用全链路（Mermaid） | [tool-invocation-flow.md](./tool-invocation-flow.md) | R-44-tool, R-42-llm, R-45-event |
| LLM 协议抽象（Message / Tool / Invoker） | [llm-protocol-abstraction.md](./llm-protocol-abstraction.md) | R-42-llm, ADR-0001 |
| PromptManager 单例机制 | [prompt-management.md](./prompt-management.md) | R-46-prompt |
| ChatMemory 生命周期与隔离 | [memory-lifecycle.md](./memory-lifecycle.md) | R-47-memory |
| 事件流模型（16 种事件） | [event-driven-model.md](./event-driven-model.md) | R-45-event |
| DDD 三层正反例 | [ddd-examples.md](./ddd-examples.md) | R-40-ddd, R-42-llm |

## 阅读顺序

新成员推荐路径：

1. **先看 ADR-0001**（`mooc-manus-all/.harness/retro/decisions/ADR-0001-llm-protocol-abstraction.md`）—— 了解 LLM 抽象为什么这么切。
2. **llm-protocol-abstraction.md** —— 值对象与 Invoker 接口的边界。
3. **agent-internals.md** —— 4 种 Agent 与决策树（这是后端最复杂的部分）。
4. **tool-invocation-flow.md** —— 一次 tool call 端到端走了哪些层。
5. **event-driven-model.md** —— Agent 与 Application / SSE 的接缝。
6. **memory-lifecycle.md** + **prompt-management.md** —— 两个全局单例的隔离与安全语义。
7. **ddd-examples.md** —— 用真实例子收尾，做"正反面"对照。

## 与 rules 的协同

knowledge 不重复 rules 正文，只在跨文档交叉处补充语境。rule 短码（R-40 ~ R-48）在 `mooc-manus/.harness/rules/` 落库，跨仓 rule 短码与 ADR 详见父仓 `mooc-manus-all/.harness/`。

## 维护

- 实质重构改动（例如新增第 5 种 Agent / 第 3 个 LLM provider / 事件常量增删）必须同步更新对应 knowledge 文件，并视情况落 ADR。
- 文档行数偏离原则参考 plan §3：每份 50–150 行，`agent-internals` / `ddd-examples` 可放宽。
