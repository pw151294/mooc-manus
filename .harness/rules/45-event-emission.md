---
rule_id: R-45-event
severity: high
---

# 事件发布（AgentEvent）

后端 Agent 通过 `chan events.AgentEvent` 向 SSE 层流式发布事件，事件类型定义在 `internal/domains/models/events/constants.go`。本规则约束事件类型、payload 必填字段、发布顺序与对外契约。与总仓 R-20-contracts（前后端契约，详见 mooc-manus-all/.harness/rules/20-cross-repo-contracts.md）协同。

## 禁止行为

1. **禁止发布未定义事件类型**
   - 仅允许 `events/constants.go` 中已声明的常量（如 `EventTypeMessage` / `EventTypePlanCreateSuccess`）
   - 新增类型必须先改 `constants.go` 并同步前端 `mooc-manus-web/src/api/sse.ts` 的 `EventType`

2. **禁止跳过事件 channel 直接 write SSE writer**
   - 禁止：Domain Service 内 `writer.Write(...)`、`fmt.Fprintf(writer, "data: ...")`
   - 所有事件先入 `chan events.AgentEvent`，由 Application / SSE 层统一序列化

3. **禁止发布缺失 BaseEvent 必填字段的事件**
   - `BaseEvent` 含 `ConversationId` / `MessageId` / `EventType` 等关键字段；缺一不可
   - `ToolEvent` 缺 `ToolCallID` / `FunctionName` → reject
   - `StepEvent` / `PlanEvent` 缺 `Status` → reject

4. **禁止断线重连重发旧事件**
   - SSE 是单向推流，断线后客户端需重发起新对话；后端不缓存事件回放

## 要求行为

1. **事件类型清单（17 种，分四组）**

   | 组 | 类型 | 触发条件 | payload 必填 |
   |----|------|---------|-------------|
   | 消息 | `message` / `message_end` / `title` | LLM 流式文本到达 / 单轮文本结束 / 摘要标题生成 | `Message` / `Role` / `Timestamp`；title 含 `Title` |
   | 工具 | `tool_call_start` / `tool_call_complete` / `tool_call_fail` | LLM 决定调用 / 调用成功 / 调用失败 | `ToolCallID` / `ToolName` / `FunctionName` / `Status` |
   | 计划 | `plan_create_success` / `plan_update_success` / `plan_update_failed` / `plan_completed` | PlanAgent 各阶段 | `Plan` / `Status` |
   | 步骤 | `step_start` / `step_complete` / `step_fail` | ReActAgent 步骤执行 | `Step` / `Status` |
   | 系统 | `wait` / `error` / `done` | 等待用户输入 / 异常 / 流结束 | error 含 `Error`，done 含 `Timestamp` |

2. **顺序约束**
   - 一次对话流必以 `done` 结束（成功 / 失败都要发）
   - `plan_create_success` 必先于任何 `step_start`
   - `step_complete` / `step_fail` 必有先行的 `step_start`（同一 step）
   - `tool_call_complete` / `tool_call_fail` 必有先行的 `tool_call_start`（同一 ToolCallID）

3. **跨仓契约（详见 R-20-contracts）**
   - 后端新增 / 改动事件类型 → 同步更新前端 EventType 与 payload TS 类型
   - 字段命名 camelCase（与 `json:` tag 对齐，参见 `events.go`）
   - 与 PR 一起提交 ADR，说明向后兼容策略

4. **断线策略**
   - 断线由前端发起新会话；后端不实现 resume / replay
   - Application 层 `defer sse.CloseChat(messageId)` 时必发 `done`，避免前端 hang

## Agent 行为

- 新增事件类型请求 → 强制走"先 constants → 再 events.go struct → 再前端 EventType → ADR"链路；先告知 R-20-contracts 影响
- 检测 Domain Service 内 `writer.Write` / `fmt.Fprintf(writer,` → 重构为 chan 发布
- PlanAgent 改动 → 检查 `plan_create_success` → `step_*` → `plan_completed` / `plan_update_failed` 顺序

## 可验证性

- 单测：每个 Agent 路径覆盖事件顺序断言（参考 PlanAgent 测试模式）
- `event-contract-checker` 子代理：扫描新增 `EventType*` 常量，断言前端同步
- 静态：
  - `grep -rn "EventType[A-Z][a-zA-Z]*" internal/domains/models/events/constants.go` 列出后台所有事件
  - `grep -rn "writer.Write\|fmt.Fprintf(writer" internal/domains/` 应为空
- 集成测试：mock SSE writer，跑完整 Plan 流程，断言事件序列与文档一致
