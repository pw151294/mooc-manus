---
rule_id: R-43-agent
severity: high
---

# Agent 编排（4 种 Agent 调用）

mooc-manus 提供 4 种 Agent 实现：`BaseAgent` / `ReActAgent` / `PlanAgent` / `A2ADomainService`（A2A Agent），位于 `internal/domains/services/agents/`。本规则定义每种 Agent 的适用场景与统一调用契约。

## 禁止行为

1. **禁止跳过 Agent 抽象直接调 LLM 或 Tool**
   - Handler / Application 层不得绕过 Agent 直接 invoke LLM
   - 工具调用必须通过 Agent 的 `tools` 切片注入（详见 R-44）

2. **禁止 Agent 直接返回字符串结果给 caller**
   - 必须通过 `chan<- events.AgentEvent`（详见 R-45）流式发布 message / tool / step / done 事件
   - Agent 函数签名形如 `func(req agents.ChatRequest, eventCh chan<- events.AgentEvent)`，不得改成 `(string, error)`

3. **禁止跨场景错用 Agent**
   - 单轮对话场景使用 PlanAgent（重场景）→ 浪费上下文 & 多轮调用
   - 复杂多步骤任务使用 BaseAgent → 缺乏规划能力
   - 远端协作场景跳过 A2A Agent → 缺乏 a2a 协议封装

## 要求行为

1. **4 种 Agent 适用场景**

   | Agent | 适用 | 关键特征 | 入口 |
   |-------|------|----------|------|
   | `BaseAgent` | 单轮 / 简单 tool-use 对话 | 直接 LLM + Tools，无 plan/反思 | `BaseAgent.Chat` |
   | `ReActAgent` | 单步执行（Reason+Act 一步） | 包装 BaseAgent，注入 ReAct 系统提示词；由 PlanAgent 调度 | `ReActAgent.ExecuteStep` |
   | `PlanAgent` | 多步骤复杂任务 | 先生成 Plan → 逐步用 ReActAgent 执行 → 可 Update Plan | `PlanAgent.Chat / CreatePlan / UpdatePlan` |
   | `A2ADomainService` | 调用远端 a2a-compatible agent | 走 `a2aproject/a2a-go` 协议封装 | `A2ADomainServiceImpl.A2AChat` |

2. **统一入参约定**
   - 所有 Agent 入口必须接收 `agents.ChatRequest`，其中：
     - `ConversationId`（必填）— 隔离 Memory（详见 R-47）
     - `MessageId`（必填，由 SSE 层生成后注入）— 隔离 Skill 容器（详见 R-48）
     - `Query`（用户消息）/ `Files` / `SkillRefs` 按需
   - 缺失 `ConversationId` 或 `MessageId` → 直接 reject，不得给默认空值

3. **统一返回约定**
   - 通过 `chan events.AgentEvent` 发布事件流；Agent 内部 panic 必须 recover 并发 `error` 事件
   - 结束时必须发 `done` 事件，由 Application 层 `defer` 关闭 SSE writer（参考 `docs/skill-executor-fix-plan.md` §6）

4. **决策树**（参考 plan spec §3.9 `agent-internals.md`，待 Phase 8 落库）
   ```
   是否远端 a2a agent?
     ├─ 是 → A2ADomainService.A2AChat
     └─ 否 → 是否多步骤?
              ├─ 是 → PlanAgent.Chat (内部调度 ReActAgent.ExecuteStep)
              └─ 否 → BaseAgent.Chat
   ```

## Agent 行为

- 用户请求"加一种新 Agent" → 先回答能否复用现有 4 种；如确实新增，要求扩 `agents/README.md` 与本规则
- 检测到 Application 层把 `chan AgentEvent` 转成字符串再返回 → 标记 blocker，要求保留事件流
- 缺 ConversationId / MessageId 的调用 → 拒绝并提示注入点（`api/handlers/agent.go` → `applications/services/agent.go` → `Convert*Request2DO`）

## 可验证性

- 单测：
  - 每个 Agent 至少覆盖"正常对话 / tool 调用 / 错误中断"3 个分支
  - 断言 done 事件最终发出（含 error 路径）
- 静态：
  - `grep -rn "func.*Chat.*string.*error" internal/domains/services/agents/` 应为空（不允许 Agent 返回字符串）
  - `grep -rn "ConversationId == \"\"" internal/applications/services/` 至少存在对应校验
- 集成测试：用 mock Invoker + mock Tool，跑 PlanAgent 完整两步任务，断言事件顺序
