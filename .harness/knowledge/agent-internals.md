# Agent 内部状态机与决策树

> 后端 4 种 Agent（`BaseAgent` / `ReActAgent` / `PlanAgent` / `A2ADomainService`）的内部状态机、控制流与场景路由。强约束见 `R-43-agent`；本文聚焦"事实层面如何运转"。

源码位置：`internal/domains/services/agents/`（base.go / react.go / plan.go / a2a.go / a2a_executor.go / agent.go）。

## 入口与统一签名

所有 Agent 的对外入口都遵循 R-43 §"统一返回约定"：返回值类型 `chan events.AgentEvent`，由调用方关闭前 drain。Agent 的入口工厂集中在 `BaseAgentDomainServiceImpl.createBaseAgent`（`agent.go`），它负责：

1. 读取 `appConfig`（含 `ModelConfig` / `AgentConfig`）
2. 通过 `PickInvoker(appConfig.ModelConfig)` 选 OpenAI / Anthropic adapter（R-42 / ADR-0001）
3. `memory.FetchMemory(request.ConversationId)` 取该会话的 ChatMemory（R-47）
4. `tools.InitTools(...)` 装配工具（R-44）
5. `NewBaseAgent(...)` 出包

然后业务层再用 `NewReActAgent(base)` / `NewPlanAgent(base)` "升级"为带提示词的高阶 Agent。

## 1. BaseAgent —— 简单 tool-use 循环

源码：`internal/domains/services/agents/base.go`。

状态机（`Invoke` 与 `StreamingInvoke` 共用主框架）：

```
[Idle]
  ↓ Invoke(query)
[AddToMemory(user msg)]
  ↓
[InvokeLLM]  ──→ err? ──→ 重试 ≤ MaxRetries ──→ 仍失败 ──→ [Error] → close(eventCh)
  ↓ ok
[ToolCalls == 0?]
  ├─ 是 → [emit OnMessage] → close(eventCh)       ← 终态
  └─ 否 → [InvokeToolCalls]
              ↓ 逐个 tool: jsonrepair → GetTool → Invoke → emit start/complete/fail
            [AddToMemory(tool results)]
              ↓
            round++; 回到 [InvokeLLM]
[round >= MaxIterations] → [emit OnError "思考轮次超过阈值"]
```

关键事实：

- `MaxRetries`：单轮 LLM 失败重试上限（`InvokeLLM` 内部，`time.Sleep(retryInterval)`，默认 5s）。
- `MaxIterations`：tool ↔ LLM 往返圈数上限（`Invoke` 外层 for），超过即 `OnError` 中断。
- `Invoke`（非流式）与 `StreamingInvoke`（流式）的差异：前者一次性 `InvokeLLM`，后者每 round 启一个 goroutine 跑 `StreamingInvokeLLM` + `llmEventCh` 把流式 token 转发出去；流式版本结束的判定依赖 `shouldEnd atomic.Bool`（当某轮 `ToolCalls == 0` 时翻为 true）。
- Memory 写入策略（`StreamingInvokeLLM`）：若 content 与 ToolCalls 都为空，会塞入一条 `{Role:Assistant, Content:""}` + `{Role:User, Content:"AI无响应内容，请继续"}`，下一轮再尝试。

## 2. ReActAgent —— 单步执行（被 PlanAgent 调度）

源码：`internal/domains/services/agents/react.go`。

ReActAgent 不暴露给 Application 层直接调用，它是 PlanAgent 的"步骤执行器"。`NewReActAgent(base)` 仅做一件事：把 `systemPrompt` 替换为 `prompts.GetReActSystemPrompt()`，其他字段全部继承。

状态机（`ExecuteStep`）：

```
[Idle]
  ↓ ExecuteStep(plan, step, request)
[拼装提示词]  modifies {message} {attachments} {language} {step} 占位符
  ↓
[step.Status = Running] → emit OnStepStart
  ↓
[BaseAgent.Invoke]  (阻塞)
  ↓ 监听 execCh
  ├─ EventTypeMessage → jsonrepair + Unmarshal → 更新 step.{Success,Result,Attachments}
  │     ├─ unmarshal fail → emit OnError + close → 返回
  │     └─ ok → emit OnStepComplete; 若 step.Result != "" 再 emit OnMessage(result)
  ├─ EventTypeError → step.Status=Failed + step.Error=... → emit OnStepFail
  └─ default → 透传 event
[step.Status = Completed]  ← 注意：即使中途 OnStepFail，循环结束仍会写 Completed（已知设计取舍）
  ↓ close(eventCh)
```

Summarize 子状态：`ReActAgent.Summarize` 用 `prompts.GetSummarizePrompt()` 再跑一轮 `Invoke`，把所有事件透传给上游，由 PlanAgent 在 Plan 完结时调度。

## 3. PlanAgent —— 规划 + 反思

源码：`internal/domains/services/agents/plan.go`。

PlanAgent 嵌入 `BaseAgent`，额外持有 4 个提示词字段：`systemPrompt / planSystemPrompt / planCreatePrompt / planUpdatePrompt`。两个核心方法：

`CreatePlan(message, files, eventCh)` 状态机：

```
[Idle]
  ↓
[拼装 planCreatePrompt]  替换 {message}/{attachments}
  ↓
[BaseAgent.Invoke(query)]  (阻塞)
  ↓ 监听 agentEventCh
  ├─ EventTypeMessage → ConvertMessage2Plan
  │     ├─ err → emit OnError("计划格式不正确") → 继续监听（不中断）
  │     └─ ok → plans.SaveOrUpdate(plan) → emit OnPlanCreateSuccess(plan)
  └─ default → 透传 event
[close(eventCh)]
```

`UpdatePlan(plan, step, eventCh)` 状态机：

```
[Idle]
  ↓
[拼装 planUpdatePrompt]  json.Marshal(plan) + json.Marshal(step)
  ↓
[BaseAgent.Invoke(query)]
  ↓ 监听
  ├─ EventTypeMessage → ConvertMessage2UpdatedPlan
  │     ├─ err → emit OnPlanUpdateFailed
  │     └─ ok → 查 plan.Steps 中第一个 Pending/Running 索引 → 用 updatedPlan.Steps 替换尾部
  │            → plans.SaveOrUpdate(updatedPlan) → emit OnPlanUpdateSuccess(plan)
  └─ default → 透传
[close(eventCh)]
```

Plan 持久化走 `internal/domains/models/prompts/plans/manager.go` 的进程内单例 `PlanManager`，键为 `Plan.ID`，并同步维护 `id2Step` 反向索引（删除时先清子 step，防残留）。

## 4. A2ADomainService —— 远端协同

源码：`internal/domains/services/agents/a2a.go` + `a2a_executor.go`。

A2A 是"本地 Agent 充当 A2A server 把工具调用转给远端 Agent"。状态机：

```
[A2AChat(request)]
  ↓
[createBaseAgent]   入口 Agent，systemPrompt = a2aSystemPrompt
  ↓
[appConfigDomainSvc.GetById]  拿到 a2a server 配置列表
  ↓
[startA2AServers(conversationId, appConfig)]
  └─ 对每个 srvCfg：
       1. createA2AExecutor → 构造子 BaseAgent（conversationId 拼成 srvCfg.ID::conversationId 做 memory 隔离）
       2. http.Serve 在 srvCfg.BaseURL 启 a2a-go 的 JSONRPCHandler + AgentCard
       3. 100ms 内无 listen 错误视为成功，缓存进 id2A2AExecutor map
  ↓
[agent.StreamingInvoke(query)]  入口 Agent 自由调用 a2a tools（远端 server）
  ↓ 事件流透传给上游
[close(eventCh)]
```

`A2AExecutor.Execute(ctx, reqCtx, q)` 是 a2a-go 协议入口，负责把远端调用方的 `TextPart` 喂给子 BaseAgent，过滤 message / error 事件并写回 a2a-go 的 `eventqueue.Queue`。其他事件（tool / step / plan）不上报远端，避免协议噪声。

## 决策树（场景 → Agent）

```
是否远端 a2a agent 协同?
  ├─ 是 → A2ADomainService.A2AChat
  └─ 否 → 是否要"规划 + 多步执行 + 可反思"?
            ├─ 是 → PlanAgent
            │        ├─ CreatePlan：生成 plan
            │        ├─ Chat / 自驱：内部 NewReActAgent.ExecuteStep 逐步执行
            │        └─ UpdatePlan：偏离时局部修订
            └─ 否 → 是否需要 LLM + tool 直接对话?
                     ├─ 是 → BaseAgent.Chat
                     └─ 否 → 不应该走 Agent，回 application 层重审
```

该决策树是 R-43 §"决策树" 的展开版本，附带了 ReActAgent 的"被 PlanAgent 调度"语义。

## 跨 Agent 共享的隐式契约

- **Memory**：4 种 Agent 共用 `BaseAgent.memory`，写入路径仅 `AddToMemory`（首次会注入 systemPrompt）；A2A 的子 Agent 用 `srvCfg.ID::conversationId` 派生 key（见 a2a.go:120），保证父子会话不互相污染。
- **事件 channel**：所有 Agent 都尊重 R-45 的"必发 done / error"约定；channel 由 Agent 在内部 close（这是 R-43 落地的关键边界，应用层不可外部 close）。
- **重试**：仅 LLM 调用层重试（`InvokeLLM` 内部，sleep `retryInterval`）；工具调用走 `InvokeTool` 的 `MaxRetries`，无 sleep；超出迭代次数走 OnError 而非死锁。

## 与 archive 的对应

本文取代 `.harness/archive/AGENTS-pre-harness-v1.md` §1.1 + §6.1 的"业务域"段落中关于 Agent 的描述。原文记的是"有哪些 Agent"，本文补充的是"它们的状态机长什么样、怎么联动"。Phase 4 完成 LLM 抽象后，原 §1.1 中 `base.go` / `a2a.go` / `plan.go` 描述已与现实一致；ReActAgent 是后续新增项，archive 未覆盖。
