# 在 ReActAgent 中添加新工具步骤

`ReActAgent.ExecuteStep`（`internal/domains/services/agents/react.go`）是 PlanAgent 调度的单步执行单元，沿用 BaseAgent 的 `tools []tools.Tool` 决定可用工具。本剧本指导"在 ReAct 步骤循环中新增一种工具调用"或"在 ExecuteStep 内插入新流程节点（如 reflection）"。关联 R-43（Agent 编排）、R-44（工具注册）、R-45（事件发布）。

## 前置条件

1. **场景明确**：新增的是「一种工具」（如新 MCP function）还是「步骤内插入一个新阶段」（如 step 完成后追加自我评分）
2. 如果是新工具 → 先看 `integrate-new-mcp-server.md`（MCP 类）或对 Skill 类，先在 `applications/services/skill_provider.go` 注册
3. 如果是新阶段 → 必须不破坏 `step_start` / `step_complete` / `step_fail` 顺序约束（R-45）
4. 阅读 `knowledge/tool-invocation-flow.md` 与 `knowledge/agent-internals.md`

## 步骤

```bash
cd /path/to/mooc-manus-all/mooc-manus
git switch -c feat/react-step-<name>
```

### 路径 A：新增一种工具（ReAct 自动 pick）

工具识别由 LLM 通过 `tools.Tool.Name()` 自动选；不要在 `react.go` 写 `if name == "xxx"`（违反 R-44 第 1 条硬编码禁令）。

1. 新工具实现 `tools.Tool` 接口（参考 `internal/domains/services/tools/execute_skill.go` / `load_skill.go` / `mcp.go`）
2. 在 ToolProvider 装配路径注册：
   - Skill 类：`tools.SkillTools(...)`（`builtin.go`）
   - MCP 类：在 `applications/services/tool_provider.go` 走 `ToolProviderDomainService` 注册流程
   - A2A 类：通过 `A2ADomainService` 包装
3. 不需要改 `react.go`；ReActAgent 在构造时已注入 `tools`

### 路径 B：在 ExecuteStep 内插入新阶段

修改 `internal/domains/services/agents/react.go::ExecuteStep`（参考既有 step 流程：构建 prompt → 改 step 状态 → 发 `step_start` → 调 invoker → 收尾发 `step_complete` / `step_fail`）：

```go
func (ra *ReActAgent) ExecuteStep(plan agents.Plan, step *agents.Step, request agents.ChatRequest, eventCh chan<- events.AgentEvent) {
    // 1. 既有：构建 prompt
    // 2. step.Status = Running; eventCh <- events.OnStepStart(*step)
    // 3. invoker.Invoke(...)
    // ★ 新阶段（如 reflection）：基于 step 输出再发一次 LLM 调用
    //    ⚠️ R-42：经 Invoker，不要直接 import openai-sdk
    //    ⚠️ R-45：若发新事件类型必先走 add-new-event-type.md
    // 4. step.Status = Completed/Failed; eventCh <- events.OnStepComplete/Fail(*step)
}
```

如要新事件类型（如 `step_reflection_*`）→ 暂停回到总仓 `add-new-event-type.md`。

### 3. 测试

- 在 `internal/domains/services/agents/agent_provider_test.go` 风格下加用例：mock invoker 返回固定 ToolCall，断言 channel 收到事件顺序
- 重点验证：新阶段失败时是否仍发 `step_fail` 而不是吞掉
- 路径 A：在 `internal/domains/services/tools/<tool>_test.go` 单测工具

### 4. 构建 & commit

```bash
go build ./... && go test ./internal/domains/services/...
git add -A
git commit -m "feat(agents): ReActAgent 加入 <name> 步骤/工具"
git push -u origin feat/react-step-<name>
```

## 常见坑

1. **硬编码工具名**：在 `react.go` 内 `if call.Name == "load_skill"` → R-44 第 1 条违反。工具识别由 LLM + `tools.Tool.Name()`。
2. **跳过事件 channel**：新阶段直接调用 SSE writer → R-45 第 2 条违反。所有 step / message / tool 状态走 `eventCh`。
3. **panic 后没 close channel**：新阶段 panic → eventCh 不关 → SSE 不结束。`defer recover() + eventCh <- OnError + 最后兜 done`。
4. **prompt 拼接污染**：在新阶段把 tool 输出直接拼到下一轮 prompt → 违反 R-31（未受信内容）+ R-46（prompt 管理）。需 escape。
5. **goroutine 泄漏**：sub-step 起 goroutine 但未等 wg.Done → step 提前结束，事件错序。沿用既有 `sync.WaitGroup` 模式。

## 验证

```bash
go build ./...
go test ./internal/domains/services/agents/... ./internal/domains/services/tools/... -v
go vet ./...
HARNESS_ROOT=.harness ./.harness/scripts/validate-harness.sh

# 端到端：用 PlanAgent 触发多步任务，观察 step 事件序列含新阶段
```

## Agent 行为

- 用户说"在 ReAct 里加个工具" → 先问是路径 A 还是 B；A 不动 react.go，B 才动
- 看到改动里出现 `if toolName == "..."` → 拒绝并指 R-44 第 1 条
- 看到新阶段需要新事件类型 → 暂停跳到 `add-new-event-type.md`
- 看到 `react.go` 内 import LLM SDK → 直接 reject（R-42）
- ⚠️ 注意 R-45 顺序：新阶段若改变 step 事件发布点 → 用 sequence diagram 确认 `start → complete/fail` 配对
- 测试只跑 happy path → 提示补 invoker 失败 / 工具失败 / panic 场景
