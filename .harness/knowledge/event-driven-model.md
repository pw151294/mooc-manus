# 事件驱动模型（AgentEvent）

> Agent → SSE 之间的唯一通信方式是 `chan events.AgentEvent`。强约束见 `R-45-event` 与总仓 `event-protocol.md`（`mooc-manus-all/.harness/knowledge/`）；本文聚焦"事实层面：16 种事件、payload、顺序约束、上下游接缝"。

源码：`internal/domains/models/events/`（base.go / constants.go / events.go / message.go / planner.go / tools.go / items.go / manager.go）。

## 接口层（`base.go`）

```go
type AgentEvent interface {
    EventId() string
    EventType() string
    SaveConversationId(string)
}

type BaseEvent struct {
    ID             string    `json:"id"`
    ConversationId string    `json:"conversationId"`
    MessageId      string    `json:"messageId"`
    Type           string    `json:"type"`
    CreatedAt      time.Time `json:"created_at"`
}
```

所有具体事件 struct 嵌入 `BaseEvent`，自动获得 ID / 类型 / 时间戳字段，并满足 `AgentEvent` 接口。

## 16 种事件常量（`constants.go`）

```go
// 消息组（3）
EventTypeTitle, EventTypeMessageEnd, EventTypeMessage

// 工具组（3）
EventTypeToolCallStart, EventTypeToolCallComplete, EventTypeToolCallFail

// 计划组（4）
EventTypePlanCreateSuccess, EventTypePlanUpdateSuccess,
EventTypePlanUpdateFailed,  EventTypePlanCompleted

// 步骤组（3）
EventTypeStepStart, EventTypeStepComplete, EventTypeStepFail

// 系统组（3）
EventTypeWait, EventTypeError, EventTypeDone
```

合计 16 种。R-45 §"事件类型清单" 在 rule 层做了同等枚举（双向 grep 校验，防止前后端漂移）。

## 状态枚举

辅助 status 枚举与 `EventType*` 平行存在：

```go
type ToolEventStatus string  // calling / completed / failed
type PlanEventStatus string  // created / updated / failed / completed
type StepEventStatus string  // started / completed / failed
```

`EventType` 表达 *发生了什么*，`Status` 表达 *目标对象处于什么阶段*；二者一一对应（如 `EventTypeStepStart` ⇔ `StepStarted`）。冗余但便于前端 switch 语义。

## 具体 struct 与 payload（`events.go`）

| 事件 struct | 关键字段 | 触发方 |
|------------|---------|--------|
| `MessageEvent` | `Role`/`Message`/`Timestamp`/`Attachments` | `BaseAgent.Invoke` 最终回答、`StreamingInvoke` 流式 token |
| `ToolEvent` | `ToolCallID`/`ToolName`/`FunctionName`/`FunctionArgs`/`FunctionResult`/`Status` | `BaseAgent.InvokeToolCalls` |
| `PlanEvent` | `Plan`/`Status` | `PlanAgent.CreatePlan`/`UpdatePlan` |
| `StepEvent` | `Step`/`Status` | `ReActAgent.ExecuteStep` |
| `WaitEvent` | — | 当前未广泛使用（预留用户输入等待） |
| `ErrorEvent` | `Error`/`Timestamp` | Agent / Adapter 失败上抛 |
| `DoneEvent` | `Timestamp` | 流终结 |
| `TitleEvent` | `Title`/`Timestamp` | 摘要生成场景（如对话首句标题） |

`ToolEvent` 还预留了两个未启用结构体 `BrowserToolContent` / `McpToolContent`（与 `// todo ToolContent` 注释呼应），用于将来扩展工具结果的富文本载体。

## 事件工厂（与触发点）

每个事件都有一个对应的 `On*` 工厂，负责填 `uuid.New()` / `time.Now()` / `Status` / `Type`，让上游 Agent 一行调用即得合法事件：

- `message.go`：`OnMessage(content, attachments)`、`OnMessageEnd()`
- `tools.go`：`OnToolCallStart` / `OnToolCallComplete` / `OnToolCallFail`（三者共用 `convert2ToolEvent` 拷贝 `llm.ToolCall` 的 ID/Name/Arguments）
- `planner.go`：`OnPlanCreateSuccess` / `OnPlanUpdateSuccess` / `OnPlanUpdateFailed` / `OnPlanComplete` / `OnStepStart` / `OnStepComplete` / `OnStepFail`
- `items.go` / `base.go`：`OnError` / `OnDone` / `OnTitle` / `OnWait`（按需查阅源文件）

## EventManager（`manager.go`）

一个进程内单例，按 conversationId 缓存事件流，主要用途是"事后查询某会话的最新 Plan"：

```go
func AddEvent(conversationId string, event AgentEvent)
func GetLatestPlan(conversationId string) agents.Plan
func Delete(conversationId string)
```

- 读写锁 `sync.RWMutex`，`GetLatestPlan` 走 `RLock`，`AddEvent`/`Delete` 走 `Lock`。
- **不**用于 SSE 流回放（R-45 §"禁止断线重连重发旧事件" 明文禁止）；它的真正用途是 PlanAgent 后续步骤查"最近的 Plan 是什么"。
- 同 `memoryManager`，目前没有 TTL，是同类无界增长缺口。

## 顺序约束（R-45 §"顺序约束"）

每次完整对话流必满足：

```
?(plan_create_success) → ? step_start → step_complete | step_fail → ...
?(plan_update_success) → ?
tool_call_start → tool_call_complete | tool_call_fail            (同一 ToolCallID 配对)
... 多轮 message / message_end ...
done                                                              (强制终态)
```

具体强约束：

- 一次流必以 `done` 结束（成功 / 失败都要发） —— 由 Application 层 `defer sse.CloseChat` 配合 Agent 内部 close(eventCh) 后发送。
- `plan_create_success` 必先于任何 `step_start`。
- `step_complete` / `step_fail` 必有先行的 `step_start`（同一 step.ID）。
- `tool_call_complete` / `tool_call_fail` 必有先行的 `tool_call_start`（同一 `ToolCallID`）。

## 上下游接缝

- **下游（Application → SSE）**：`internal/applications/services/agent.go::Chat` 的核心循环：
  ```go
  for event := range eventCh {
      event.SaveConversationId(clientRequest.ConversationId)
      sse.SendEvent(event, messageId)
  }
  ```
  application 不再发明事件，只做"补会话 ID + 转发"。
- **上游（Agent → Application）**：Agent 在内部 close eventCh；application 用 `for range` 自然结束循环，再走 `defer sse.CloseChat`。
- **跨仓**：前端 `mooc-manus-web/src/api/sse.ts` 必须与本文件的 16 种 `EventType*` 一一对应；R-20-contracts（`mooc-manus-all/.harness/rules/20-cross-repo-contracts.md`）锁定双方同步发版。

## 安全 / 失败语义

- Domain 层禁止直接 `writer.Write(...)` / `fmt.Fprintf(writer, ...)` —— 必须经 `chan` 发布（R-45 §"禁止跳过事件 channel"）。
- 漏发 `done` 会让前端 hang —— 必须靠 application 层 `defer` 兜底；Agent 自身 panic 时 `recover` 后也要发 error + done。
- 新增事件类型 → 走"先 constants → 再 events.go struct → 再 `On*` 工厂 → 再前端 EventType → ADR"链路（R-45 §"跨仓契约"），不可单仓提交。

## 静态校验

```
grep -rn "EventType[A-Z][a-zA-Z]*" internal/domains/models/events/constants.go
grep -rn "writer.Write\|fmt.Fprintf(writer" internal/domains/   # 应为空
```

`event-contract-checker` 子代理在 PR 阶段比对 constants.go 与 `sse.ts` 的 EventType 集合，差集报 blocker。
