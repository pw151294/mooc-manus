# 新增第 5 种 Agent

当前 4 种 Agent（`BaseAgent` / `ReActAgent` / `PlanAgent` / `A2ADomainService`）覆盖单轮 / 单步 / 多步 / 远端协作场景。新增第 5 种（如 `SwarmAgent` 多 Agent 协作、`WorkflowAgent` 静态 DAG）必须遵循统一抽象。关联 R-43（Agent 编排）、R-45（事件发布）、R-47（Memory 边界）。

## 前置条件

1. **场景论证**：写一句话说明这个新 Agent 解决既有 4 种解决不了的什么问题；与 R-43 的"Agent 适用场景表"对齐
2. 决定调用入口名（如 `SwarmChat`）与是否复用 `agents.ChatRequest`（强烈推荐复用）
3. 决定 Memory 隔离粒度（按 `ConversationId` / 按 `MessageId` / 自定义）
4. ADR 写完并合并到 `.harness/specs/`，给后续多 Agent 决策点留证据
5. 阅读 `knowledge/agent-internals.md` 与 `knowledge/event-driven-model.md`

## 步骤

```bash
cd /path/to/mooc-manus-all/mooc-manus
git switch -c feat/agent-<name>
```

### 1. Domain 层

新建 `internal/domains/services/agents/<name>.go`（参考 `react.go` / `plan.go` 既有形态）：

```go
package agents

type <Name>Agent struct {
    BaseAgent              // 嵌入复用 invoker / memory / tools / config
    // 新增字段（如多个 sub-agent、调度策略）
}

func New<Name>Agent(baseAgent *BaseAgent /* + 依赖 */) *<Name>Agent {
    a := &<Name>Agent{}
    a.agentConfig = baseAgent.agentConfig
    a.invoker = baseAgent.invoker
    a.memory = baseAgent.memory
    a.tools = baseAgent.tools
    return a
}

// 入口签名遵循 R-43：req + eventCh，禁止改成 (string, error)
func (a *<Name>Agent) <Method>(req agents.ChatRequest, eventCh chan<- events.AgentEvent) {
    defer close(eventCh)
    // 发送 step_start / message / tool_call_* / done 等事件
    // ⚠️ R-45：仅使用 events/constants.go 已声明的事件类型；新事件需先走 add-new-event-type.md
}
```

### 2. Application 层装配

- 在 `internal/applications/services/agent.go` 新增方法（或在既有 method 内根据 `AgentMode` 路由）
- DI 在 `api/routers/route.go::InitRouter` 装入新 Agent 实例
- DTO 入口在 `internal/applications/dtos/` 增加 `AgentMode` 枚举值

### 3. Memory（R-47）

- 沿用 `BaseAgent.memory`（按 ConversationId 隔离）即可，除非新 Agent 要跨会话共享 → 必须新建 Memory 实现并写 ADR
- 不要在 Agent 内直接读写全局 map 当 Memory 用

### 4. 测试

- 在 `internal/domains/services/agents/agent_provider_test.go` 风格下新增 `<name>_test.go`
- 断言：channel 收到的事件序列符合 R-45 顺序约束（如 `step_start` 前必先发 `plan_create_success`）
- mock invoker（参考既有 base test）

### 5. 构建 & commit

```bash
go build ./... && go test ./internal/domains/services/agents/...
git add -A
git commit -m "feat(agents): 新增 <Name>Agent 用于 <场景>"
git push -u origin feat/agent-<name>
# 走 PR → merge
```

## 常见坑

1. **改了入口签名**：把 `(req, eventCh)` 改成 `(req) (string, error)` → 违反 R-43。所有 Agent 必须流式发事件。
2. **跨场景错用**：复杂任务用 `BaseAgent`，单轮对话用 `PlanAgent` → 用户体验差。新 Agent 写好 R-43 风格的适用场景表，挂到 `knowledge/agent-internals.md`。
3. **Memory 越权**：直接读 `BaseAgent.memory` 字段改写记录 → 绕过 Memory 抽象。所有读写经 Memory 接口。
4. **吞 panic**：goroutine 内 panic 没 recover，eventCh 不会关 → SSE 流不结束。统一在入口 `defer recover() + eventCh <- OnError + done`。
5. **R-42 越界**：在新 Agent 里 import SDK（如 `go-openai`）→ 违反 R-42。只能 import `internal/domains/models/llm` 与 `invoker`。

## 验证

```bash
go build ./...
go test ./internal/domains/services/agents/...
go vet ./...

# harness 完整性
HARNESS_ROOT=.harness ./.harness/scripts/validate-harness.sh

# 端到端（如有 docker compose）
docker compose up -d
# curl /api/v1/agent/chat 触发新 Agent 模式，验证 SSE 事件序列
```

## Agent 行为

- 接到"加一种 Agent"请求 → 先写 ADR 论证为何 4 种不够用；用户若直接说"我就要写"，提示"R-43 第 3 条要求场景不冲突"
- 实现到一半发现需要新事件类型 → 暂停，跳到总仓 `add-new-event-type.md` 走完再回来
- 实现期间 import 任何 LLM SDK → 直接阻止（R-42 红线）
- ⚠️ 注意 R-43：函数签名若不带 `eventCh chan<- events.AgentEvent` → reject
- ⚠️ 注意 R-47：Memory 实现若跨 ConversationId 共享 → 强制写 ADR
- 测试覆盖率不足（只测 happy path）→ 提示补 panic / 超时 / tool 失败场景
