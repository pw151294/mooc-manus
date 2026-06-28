# LLM 协议抽象重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `BaseAgent` 与其周边（Memory / Tool / Events）从强耦合 OpenAI 协议改造为协议无关的统一抽象，让框架可以同时承载 OpenAI 与 Anthropic 适配器。

**Architecture:** 在 `internal/domains/models/llm/` 新增厂商无关的 `Message` / `ToolCall` / `Tool` / `Invoker` 抽象，所有 OpenAI / Anthropic 类型转换收敛到 `internal/infra/external/llm/*_adapter.go` 各自的适配器内部。Memory / Tool / Events / BaseAgent 全链路换成统一类型，`createBaseAgent` 按 `ModelConfig.Provider` 选择适配器。

**Tech Stack:** Go 1.25, `github.com/openai/openai-go v1.12.0`（仅 OpenAI 适配器内部使用）, GORM, zap, jsonrepair, uuid。

**Spec:** `docs/superpowers/specs/2026-06-28-llm-protocol-abstraction-design.md`

**包路径约定**：
- 领域层包导入路径：`mooc-manus/internal/domains/models/llm`,包名 `llm`(只放 `Message` / `Tool` / `ToolCall` / `Role` 等数据结构)。
- **`Invoker` 接口的包**：`mooc-manus/internal/domains/models/invoker`,包名 `invoker`。**与 plan 原始设计的偏离**:`Invoker` 同时依赖 `events.AgentEvent` 与 `llm.Message`,而 `events` 又需要依赖 `llm.ToolCall`,直接放 `llm` 包会形成 import cycle,因此独立为 `invoker` 包(在 Task 5 执行过程中发现并修正)。后续所有引用 `Invoker` 接口的代码统一写 `invoker.Invoker`,而不是 `llm.Invoker`。
- 基础设施层(既有)包导入路径:`mooc-manus/internal/infra/external/llm`,包名 `llm`。
- 同名冲突时,按调用点取别名:领域层用 `llm`,基础设施层用 `llmadapter`(举例:`import llmadapter "mooc-manus/internal/infra/external/llm"`)。

**TDD 策略**：
- 有真实逻辑的改动（双向转换、Compact bug 修复、Provider 选择分支）走严格 TDD：先写失败测试 → 看红 → 写最小实现 → 看绿 → 重构。
- 纯类型搬移与签名重命名走"编译驱动"：以 `go build ./...` 报错为失败信号，修到全绿即可，不强制写无意义的同义测试。

---

## File Structure（新建/修改一览）

**新建：**
- `internal/domains/models/llm/message.go` — `Role` 常量、`ToolCall`、`Message` 结构体
- `internal/domains/models/llm/tool.go` — `Tool` 结构体
- `internal/domains/models/llm/invoker.go` — `Invoker` 接口
- `internal/infra/external/llm/openai_adapter.go` — `OpenAIAdapter` 与转换函数
- `internal/infra/external/llm/openai_adapter_test.go` — 双向转换单测
- `internal/infra/external/llm/anthropic_adapter.go` — `AnthropicAdapter` 接口骨架
- `internal/domains/models/memory/memory_test.go` — `ChatMemory` 行为与 `Compact` bug 修复单测
- `internal/domains/models/events/tools_test.go` — `OnToolCallStart/Complete/Fail` 构造函数单测
- `internal/domains/services/agents/agent_provider_test.go` — `pickInvoker` 分支选择单测

**修改：**
- `internal/infra/external/llm/openai.go` — 保留底层 SDK 封装，无功能改动
- `internal/domains/models/memory/memory.go` — 内部存储改 `[]llm.Message`，修 `Compact` 拼写 bug
- `internal/domains/models/memory/manager.go` — 同步替换类型引用
- `internal/domains/services/tools/base.go` — 接口与 `BaseTool.GetTools()` 返回 `[]llm.Tool`
- `internal/domains/services/tools/convert.go` — `convertDO2Tool` 返回 `llm.Tool`
- `internal/domains/services/tools/mcp.go` / `custom.go` / `a2a.go` / `load_skill.go` / `builtin.go` / `execute_skill.go` — 如有自定义 `GetTools()` 则同步改造（grep 已确认目前仅 `BaseTool` 有实现，其它通过嵌入复用，可能仅需 import 修正）
- `internal/domains/models/events/events.go` — `ToolEvent` 字段去 openai 依赖
- `internal/domains/models/events/tools.go` — 构造函数参数改 `llm.ToolCall`
- `internal/domains/services/agents/base.go` — 全部方法签名换 `llm.*`
- `internal/domains/services/agents/plan.go` / `react.go` — `agent.llm = baseAgent.llm` 改为 `agent.invoker = baseAgent.invoker`
- `internal/domains/services/agents/agent.go` — `createBaseAgent` 按 Provider 选择适配器；抽取 `pickInvoker`
- `internal/domains/services/agents/a2a_executor.go` — 编译适配
- `internal/domains/services/flows/plan_react.go` — 构造 `llm.Invoker` 注入
- `internal/domains/models/app_config.go` — `ModelConfig.Provider` + PO/DO 转换函数
- `internal/infra/models/app_config.go` — `AppConfigPO.Provider` GORM 字段

**清理（死代码）：**
- `tests/mock.go` — 全文件删除（无调用方，仅用于结构示例）
- `internal/domains/models/file/file.go:15-64` — 删除未被调用的 `Convert2UserMessage` 与 `ConvertMessage2QueryAndAttachments`

---

## Task 1：新建领域层抽象 — `llm.Message` / `Tool` / `ToolCall` / `Invoker`

**Files:**
- Create: `internal/domains/models/llm/message.go`
- Create: `internal/domains/models/llm/tool.go`
- Create: `internal/domains/models/llm/invoker.go`

- [ ] **Step 1.1：写 `message.go`**

```go
package llm

type Role string

const (
    RoleSystem    Role = "system"
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleTool      Role = "tool"
)

type ToolCall struct {
    ID        string
    Name      string
    Arguments string
}

type Message struct {
    Role       Role
    Content    string
    ToolCalls  []ToolCall
    ToolCallID string
    Extra      map[string]any
}
```

- [ ] **Step 1.2：写 `tool.go`**

```go
package llm

type Tool struct {
    Name        string
    Description string
    Parameters  map[string]any
}
```

- [ ] **Step 1.3：写 `invoker.go`**

```go
package llm

import "mooc-manus/internal/domains/models/events"

type Invoker interface {
    Invoke(messages []Message, tools []Tool) (Message, error)
    StreamingInvoke(messages []Message, tools []Tool, eventCh chan<- events.AgentEvent) Message
}
```

- [ ] **Step 1.4：单独编译子包**

Run: `go build ./internal/domains/models/llm/...`
Expected: 通过，无输出。

- [ ] **Step 1.5：提交**

```bash
git add internal/domains/models/llm/
git commit -m "feat(llm): 新增 LLM 协议无关的统一消息体与 Invoker 抽象"
```

---

## Task 2：实现 OpenAI Adapter 与双向转换（含 TDD 单测）

**Files:**
- Create: `internal/infra/external/llm/openai_adapter.go`
- Create: `internal/infra/external/llm/openai_adapter_test.go`

**说明**：本任务保持现有 `openai.go` 不改，只新增 adapter 层。Adapter 内部复用 `OpenAiLLM`，并实现 `llm.Invoker`。

- [ ] **Step 2.1：先写转换函数失败单测（红）**

写入 `internal/infra/external/llm/openai_adapter_test.go`：

```go
package llm

import (
    "testing"

    domainllm "mooc-manus/internal/domains/models/llm"
)

func TestToOpenAIMessages_System(t *testing.T) {
    msgs := []domainllm.Message{{Role: domainllm.RoleSystem, Content: "hello"}}
    out := toOpenAIMessages(msgs)
    if len(out) != 1 {
        t.Fatalf("want 1 message, got %d", len(out))
    }
    if role := out[0].GetRole(); role == nil || *role != "system" {
        t.Fatalf("want role=system, got %v", role)
    }
    if c := out[0].GetContent(); c == nil || c.OfString.String() != "hello" {
        t.Fatalf("want content=hello, got %v", c)
    }
}

func TestToOpenAIMessages_User(t *testing.T) {
    msgs := []domainllm.Message{{Role: domainllm.RoleUser, Content: "q"}}
    out := toOpenAIMessages(msgs)
    if role := out[0].GetRole(); role == nil || *role != "user" {
        t.Fatalf("want role=user, got %v", role)
    }
}

func TestToOpenAIMessages_AssistantWithToolCalls(t *testing.T) {
    msgs := []domainllm.Message{{
        Role:    domainllm.RoleAssistant,
        Content: "",
        ToolCalls: []domainllm.ToolCall{
            {ID: "tc-1", Name: "search", Arguments: `{"q":"go"}`},
        },
    }}
    out := toOpenAIMessages(msgs)
    if role := out[0].GetRole(); role == nil || *role != "assistant" {
        t.Fatalf("want role=assistant, got %v", role)
    }
    tcs := out[0].GetToolCalls()
    if len(tcs) != 1 || tcs[0].ID != "tc-1" || tcs[0].Function.Name != "search" {
        t.Fatalf("tool calls mapping failed: %+v", tcs)
    }
}

func TestToOpenAIMessages_Tool(t *testing.T) {
    msgs := []domainllm.Message{{
        Role:       domainllm.RoleTool,
        Content:    "result",
        ToolCallID: "tc-1",
    }}
    out := toOpenAIMessages(msgs)
    if role := out[0].GetRole(); role == nil || *role != "tool" {
        t.Fatalf("want role=tool, got %v", role)
    }
    if id := out[0].GetToolCallID(); id == nil || *id != "tc-1" {
        t.Fatalf("want tool_call_id=tc-1, got %v", id)
    }
}

func TestToOpenAITools(t *testing.T) {
    tools := []domainllm.Tool{{
        Name:        "search",
        Description: "search the web",
        Parameters:  map[string]any{"type": "object"},
    }}
    out := toOpenAITools(tools)
    if len(out) != 1 || out[0].Function.Name != "search" {
        t.Fatalf("tool mapping failed: %+v", out)
    }
}
```

继续在同一文件追加 `fromOpenAIMessage` 的双向测试：

```go
func TestFromOpenAIMessage_TextOnly(t *testing.T) {
    in := openai.ChatCompletionMessage{Content: "hi", Role: "assistant"}
    out := fromOpenAIMessage(in)
    if out.Role != domainllm.RoleAssistant {
        t.Fatalf("want assistant, got %s", out.Role)
    }
    if out.Content != "hi" {
        t.Fatalf("want content=hi, got %s", out.Content)
    }
    if len(out.ToolCalls) != 0 {
        t.Fatalf("want no tool calls")
    }
}

func TestFromOpenAIMessage_WithToolCalls(t *testing.T) {
    in := openai.ChatCompletionMessage{
        Role: "assistant",
        ToolCalls: []openai.ChatCompletionMessageToolCall{
            {
                ID: "tc-1",
                Function: openai.ChatCompletionMessageToolCallFunction{
                    Name:      "search",
                    Arguments: `{"q":"go"}`,
                },
            },
        },
    }
    out := fromOpenAIMessage(in)
    if len(out.ToolCalls) != 1 {
        t.Fatalf("want 1 tool call, got %d", len(out.ToolCalls))
    }
    tc := out.ToolCalls[0]
    if tc.ID != "tc-1" || tc.Name != "search" || tc.Arguments != `{"q":"go"}` {
        t.Fatalf("tool call mapping failed: %+v", tc)
    }
}
```

import 区追加 `"github.com/openai/openai-go"`。文件最终的 import 块大致为：

```go
import (
    "testing"

    "github.com/openai/openai-go"

    domainllm "mooc-manus/internal/domains/models/llm"
)
```

- [ ] **Step 2.2：跑测试看红**

Run: `go test ./internal/infra/external/llm/... -run TestToOpenAIMessages -v`
Expected: 编译失败，提示 `toOpenAIMessages`、`toOpenAITools`、`fromOpenAIMessage` 等未定义。

- [ ] **Step 2.3：写 `openai_adapter.go` 完成最小实现**

```go
package llm

import (
    domainllm "mooc-manus/internal/domains/models/llm"
    "mooc-manus/internal/domains/models"
    "mooc-manus/internal/domains/models/events"
    "mooc-manus/pkg/logger"

    "github.com/openai/openai-go"
    "go.uber.org/zap"
)

type OpenAIAdapter struct {
    llm *OpenAiLLM
}

func NewOpenAIAdapter(cfg models.ModelConfig) *OpenAIAdapter {
    return &OpenAIAdapter{llm: NewOpenAiLLM(cfg)}
}

func (a *OpenAIAdapter) Invoke(messages []domainllm.Message, tools []domainllm.Tool) (domainllm.Message, error) {
    resp, err := a.llm.Invoke(toOpenAIMessages(messages), toOpenAITools(tools))
    if err != nil {
        return domainllm.Message{}, err
    }
    return fromOpenAIMessage(resp), nil
}

func (a *OpenAIAdapter) StreamingInvoke(messages []domainllm.Message, tools []domainllm.Tool, eventCh chan<- events.AgentEvent) domainllm.Message {
    resp := a.llm.StreamingInvoke(toOpenAIMessages(messages), toOpenAITools(tools), eventCh)
    return fromOpenAIMessage(resp)
}

func toOpenAIMessages(messages []domainllm.Message) []openai.ChatCompletionMessageParamUnion {
    out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
    for _, m := range messages {
        switch m.Role {
        case domainllm.RoleSystem:
            out = append(out, openai.SystemMessage(m.Content))
        case domainllm.RoleUser:
            out = append(out, openai.UserMessage(m.Content))
        case domainllm.RoleAssistant:
            if len(m.ToolCalls) == 0 {
                out = append(out, openai.AssistantMessage(m.Content))
                continue
            }
            asst := openai.ChatCompletionAssistantMessageParam{}
            asst.Content.OfString = openai.String(m.Content)
            asst.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, 0, len(m.ToolCalls))
            for _, tc := range m.ToolCalls {
                asst.ToolCalls = append(asst.ToolCalls, openai.ChatCompletionMessageToolCallParam{
                    ID: tc.ID,
                    Function: openai.ChatCompletionMessageToolCallFunctionParam{
                        Name:      tc.Name,
                        Arguments: tc.Arguments,
                    },
                    Type: "function",
                })
            }
            out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
        case domainllm.RoleTool:
            out = append(out, openai.ToolMessage(m.Content, m.ToolCallID))
        default:
            logger.Warn("unknown role when converting to openai message", zap.String("role", string(m.Role)))
        }
    }
    return out
}

func toOpenAITools(tools []domainllm.Tool) []openai.ChatCompletionToolParam {
    if len(tools) == 0 {
        return nil
    }
    out := make([]openai.ChatCompletionToolParam, 0, len(tools))
    for _, t := range tools {
        fn := openai.FunctionDefinitionParam{}
        fn.Name = t.Name
        fn.Description = openai.String(t.Description)
        fn.Parameters = t.Parameters
        out = append(out, openai.ChatCompletionToolParam{Function: fn, Type: "function"})
    }
    return out
}

func fromOpenAIMessage(m openai.ChatCompletionMessage) domainllm.Message {
    out := domainllm.Message{
        Role:    domainllm.RoleAssistant, // OpenAI Chat Completion 响应里只会是 assistant
        Content: m.Content,
    }
    if len(m.ToolCalls) > 0 {
        out.ToolCalls = make([]domainllm.ToolCall, 0, len(m.ToolCalls))
        for _, tc := range m.ToolCalls {
            out.ToolCalls = append(out.ToolCalls, domainllm.ToolCall{
                ID:        tc.ID,
                Name:      tc.Function.Name,
                Arguments: tc.Function.Arguments,
            })
        }
    }
    return out
}
```

- [ ] **Step 2.4：跑测试看绿**

Run: `go test ./internal/infra/external/llm/... -v`
Expected: 7 个用例全部 PASS。

- [ ] **Step 2.5：提交**

```bash
git add internal/infra/external/llm/openai_adapter.go internal/infra/external/llm/openai_adapter_test.go
git commit -m "feat(llm): 新增 OpenAI Adapter 与 Message/Tool 双向转换"
```

---

## Task 3：Anthropic Adapter 骨架

**Files:**
- Create: `internal/infra/external/llm/anthropic_adapter.go`

- [ ] **Step 3.1：写骨架文件**

```go
package llm

import (
    "errors"

    domainllm "mooc-manus/internal/domains/models/llm"
    "mooc-manus/internal/domains/models"
    "mooc-manus/internal/domains/models/events"
)

type AnthropicAdapter struct {
    cfg models.ModelConfig
}

func NewAnthropicAdapter(cfg models.ModelConfig) *AnthropicAdapter {
    return &AnthropicAdapter{cfg: cfg}
}

func (a *AnthropicAdapter) Invoke(messages []domainllm.Message, tools []domainllm.Tool) (domainllm.Message, error) {
    return domainllm.Message{}, errors.New("anthropic adapter not implemented")
}

func (a *AnthropicAdapter) StreamingInvoke(messages []domainllm.Message, tools []domainllm.Tool, eventCh chan<- events.AgentEvent) domainllm.Message {
    close(eventCh)
    return domainllm.Message{}
}
```

- [ ] **Step 3.2：编译单包**

Run: `go build ./internal/infra/external/llm/...`
Expected: 通过。

- [ ] **Step 3.3：用接口契约校验适配器实现完整性**

在 `internal/infra/external/llm/openai_adapter_test.go` 末尾追加：

```go
// 编译期断言：两个适配器都实现了 llm.Invoker
var (
    _ domainllm.Invoker = (*OpenAIAdapter)(nil)
    _ domainllm.Invoker = (*AnthropicAdapter)(nil)
)
```

Run: `go build ./internal/infra/external/llm/...`
Expected: 通过；若任一适配器签名不匹配，编译会失败。

- [ ] **Step 3.4：提交**

```bash
git add internal/infra/external/llm/anthropic_adapter.go internal/infra/external/llm/openai_adapter_test.go
git commit -m "feat(llm): 新增 Anthropic Adapter 接口骨架"
```

---

## Task 4：重构 ChatMemory（含 Compact bug 修复 TDD）

**Files:**
- Modify: `internal/domains/models/memory/memory.go`
- Modify: `internal/domains/models/memory/manager.go`
- Create: `internal/domains/models/memory/memory_test.go`

- [ ] **Step 4.1：先写覆盖 Compact 行为的失败测试**

写入 `internal/domains/models/memory/memory_test.go`：

```go
package memory

import (
    "testing"

    "mooc-manus/internal/domains/models/llm"
)

func TestChatMemory_AddAndGetMessages(t *testing.T) {
    m := NewChatMemory()
    m.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
    m.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "hello"})

    msgs := m.GetMessages()
    if len(msgs) != 2 {
        t.Fatalf("want 2 messages, got %d", len(msgs))
    }
    if msgs[0].Content != "hi" || msgs[1].Content != "hello" {
        t.Fatalf("messages content mismatch: %+v", msgs)
    }
}

func TestChatMemory_Compact_RemovesBrowserToolResult(t *testing.T) {
    m := NewChatMemory()
    // assistant 发起对 browser 工具的调用
    m.AddMessage(llm.Message{
        Role: llm.RoleAssistant,
        ToolCalls: []llm.ToolCall{{
            ID: "tc-1", Name: "browser", Arguments: "{}",
        }},
    })
    // tool 返回的网页内容
    m.AddMessage(llm.Message{
        Role:       llm.RoleTool,
        Content:    "<html>...long page...</html>",
        ToolCallID: "tc-1",
    })

    m.Compact()

    msgs := m.GetMessages()
    var toolMsg *llm.Message
    for i := range msgs {
        if msgs[i].Role == llm.RoleTool {
            toolMsg = &msgs[i]
        }
    }
    if toolMsg == nil {
        t.Fatalf("expected tool message preserved after compact")
    }
    if toolMsg.Content != "removed" {
        t.Fatalf("expected browser tool content compacted to 'removed', got %q", toolMsg.Content)
    }
}

func TestChatMemory_Compact_KeepsOtherToolResults(t *testing.T) {
    m := NewChatMemory()
    m.AddMessage(llm.Message{
        Role: llm.RoleAssistant,
        ToolCalls: []llm.ToolCall{{
            ID: "tc-2", Name: "calculator", Arguments: "{}",
        }},
    })
    m.AddMessage(llm.Message{
        Role:       llm.RoleTool,
        Content:    "42",
        ToolCallID: "tc-2",
    })

    m.Compact()

    msgs := m.GetMessages()
    var toolMsg *llm.Message
    for i := range msgs {
        if msgs[i].Role == llm.RoleTool {
            toolMsg = &msgs[i]
        }
    }
    if toolMsg == nil || toolMsg.Content != "42" {
        t.Fatalf("non-browser/search tool result should remain intact, got %+v", toolMsg)
    }
}
```

- [ ] **Step 4.2：跑测试看红**

Run: `go test ./internal/domains/models/memory/... -v`
Expected: 编译失败，提示 `ChatMemory.AddMessage` 签名仍为 openai 类型。

- [ ] **Step 4.3：重写 `memory.go`**

> **影响分析（前置确认）**：已通过 grep 验证 `GetMessageRole` 仅在 `memory.go` 内自身定义，无任何外部调用方，可安全删除。如未来发现遗漏调用，按编译报错补回即可。

```go
package memory

import (
    "slices"

    "mooc-manus/internal/domains/models/llm"
)

type ChatMemory struct {
    messages        []llm.Message
    toolCallId2Name map[string]string
}

func NewChatMemory() *ChatMemory {
    return &ChatMemory{
        messages:        make([]llm.Message, 0),
        toolCallId2Name: make(map[string]string),
    }
}

func (c *ChatMemory) AddMessage(message llm.Message) {
    c.recordToolCalls(message)
    c.messages = append(c.messages, message)
}

func (c *ChatMemory) AddMessages(messages []llm.Message) {
    if len(messages) == 0 {
        return
    }
    for _, message := range messages {
        c.recordToolCalls(message)
    }
    c.messages = append(c.messages, messages...)
}

func (c *ChatMemory) GetMessages() []llm.Message { return c.messages }

func (c *ChatMemory) GetLastMessage() *llm.Message {
    if len(c.messages) == 0 {
        return nil
    }
    return &c.messages[len(c.messages)-1]
}

func (c *ChatMemory) Rollback() {
    if len(c.messages) > 0 {
        c.messages = c.messages[:len(c.messages)-1]
    }
}

// Compact 把已经被消费的 browser / bing_search 工具结果替换成 "removed"
// 修正：原实现误判 role == "tools"（复数），导致循环体永不执行；新实现使用 llm.RoleTool。
func (c *ChatMemory) Compact() {
    for i := range c.messages {
        if c.messages[i].Role != llm.RoleTool {
            continue
        }
        funcName := c.toolCallId2Name[c.messages[i].ToolCallID]
        if slices.Contains([]string{"browser", "bing_search"}, funcName) {
            c.messages[i].Content = "removed"
        }
    }
}

func (c *ChatMemory) IsEmpty() bool { return len(c.messages) == 0 }

func (c *ChatMemory) recordToolCalls(message llm.Message) {
    if message.Role != llm.RoleAssistant {
        return
    }
    for _, tc := range message.ToolCalls {
        c.toolCallId2Name[tc.ID] = tc.Name
    }
}
```

- [ ] **Step 4.4：同步改 `manager.go`**

把 `messages = make([]openai.ChatCompletionMessageParamUnion, 0, 0)` 改为 `messages = make([]llm.Message, 0)`，并移除 `github.com/openai/openai-go` import，新增 `"mooc-manus/internal/domains/models/llm"`。

- [ ] **Step 4.5：跑测试看绿**

Run: `go test ./internal/domains/models/memory/... -v`
Expected: 3 个用例 PASS。

- [ ] **Step 4.6：提交**

```bash
git add internal/domains/models/memory/
git commit -m "refactor(memory): ChatMemory 切换为 llm.Message；修复 Compact role 拼写 bug（行为变更：原压缩逻辑从未生效，重构后真正生效）"
```

---

## Task 5：重构 Events 层（`ToolEvent` 与构造函数）

**Files:**
- Modify: `internal/domains/models/events/events.go:32-42`
- Modify: `internal/domains/models/events/tools.go`
- Create: `internal/domains/models/events/tools_test.go`

- [ ] **Step 5.1：写覆盖构造函数的失败测试**

写入 `internal/domains/models/events/tools_test.go`：

```go
package events

import (
    "testing"

    "mooc-manus/internal/domains/models"
    "mooc-manus/internal/domains/models/llm"
)

func TestOnToolCallStart_FieldMapping(t *testing.T) {
    tc := llm.ToolCall{ID: "tc-1", Name: "search", Arguments: `{"q":"go"}`}
    ev := OnToolCallStart(tc, "web_provider").(*ToolEvent)

    if ev.ToolCallID != "tc-1" {
        t.Fatalf("ToolCallID mismatch: %s", ev.ToolCallID)
    }
    if ev.FunctionName != "search" {
        t.Fatalf("FunctionName mismatch: %s", ev.FunctionName)
    }
    if ev.FunctionArgs != `{"q":"go"}` {
        t.Fatalf("FunctionArgs mismatch: %s", ev.FunctionArgs)
    }
    if ev.ToolName != "web_provider" {
        t.Fatalf("ToolName mismatch: %s", ev.ToolName)
    }
    if ev.Status != ToolEventStatusCalling {
        t.Fatalf("Status mismatch: %s", ev.Status)
    }
}

func TestOnToolCallComplete_CarriesResult(t *testing.T) {
    tc := llm.ToolCall{ID: "tc-1", Name: "search", Arguments: "{}"}
    result := &models.ToolCallResult{Success: true, Message: "ok"}

    ev := OnToolCallComplete(tc, "web_provider", result).(*ToolEvent)
    if ev.FunctionResult == nil || !ev.FunctionResult.Success {
        t.Fatalf("result not propagated: %+v", ev.FunctionResult)
    }
    if ev.Status != ToolEventStatusCompleted {
        t.Fatalf("status mismatch: %s", ev.Status)
    }
}

func TestOnToolCallFail_StatusFailed(t *testing.T) {
    tc := llm.ToolCall{ID: "tc-1", Name: "search", Arguments: "{}"}
    result := &models.ToolCallResult{Success: false, Message: "boom"}
    ev := OnToolCallFail(tc, "web_provider", result).(*ToolEvent)
    if ev.Status != ToolEventStatusFailed {
        t.Fatalf("status mismatch: %s", ev.Status)
    }
}
```

- [ ] **Step 5.2：跑测试看红**

Run: `go test ./internal/domains/models/events/... -v`
Expected: 编译失败（旧签名仍是 `openai.ChatCompletionMessageToolCall`）。

- [ ] **Step 5.3：改 `events.go` 移除 openai 依赖**

把 `events.go` 内 `ToolEvent` 上方残留注释的 `// todo ToolContent ...` 保留即可，无需改动。结构体本身已经无 openai 类型字段。

确认 `events.go` 顶部 `import` 中没有 `github.com/openai/openai-go`，如有就删掉。

- [ ] **Step 5.4：重写 `tools.go`**

```go
package events

import (
    "time"

    "github.com/google/uuid"

    "mooc-manus/internal/domains/models"
    "mooc-manus/internal/domains/models/llm"
)

func convert2ToolEvent(toolCall llm.ToolCall, toolName string) ToolEvent {
    ev := ToolEvent{}
    ev.ID = uuid.New().String()
    ev.CreatedAt = time.Now()
    ev.Timestamp = time.Now()
    ev.ToolCallID = toolCall.ID
    ev.ToolName = toolName
    ev.FunctionName = toolCall.Name
    ev.FunctionArgs = toolCall.Arguments
    return ev
}

func OnToolCallStart(toolCall llm.ToolCall, toolName string) AgentEvent {
    ev := convert2ToolEvent(toolCall, toolName)
    ev.Type = EventTypeToolCallStart
    ev.Status = ToolEventStatusCalling
    return &ev
}

func OnToolCallComplete(toolCall llm.ToolCall, toolName string, result *models.ToolCallResult) AgentEvent {
    ev := convert2ToolEvent(toolCall, toolName)
    ev.Type = EventTypeToolCallComplete
    ev.Status = ToolEventStatusCompleted
    ev.FunctionResult = result
    return &ev
}

func OnToolCallFail(toolCall llm.ToolCall, toolName string, result *models.ToolCallResult) AgentEvent {
    ev := convert2ToolEvent(toolCall, toolName)
    ev.Type = EventTypeToolCallFail
    ev.Status = ToolEventStatusFailed
    ev.FunctionResult = result
    return &ev
}
```

- [ ] **Step 5.5：跑测试看绿**

Run: `go test ./internal/domains/models/events/... -v`
Expected: 3 个用例 PASS。

- [ ] **Step 5.6：提交**

```bash
git add internal/domains/models/events/
git commit -m "refactor(events): ToolEvent 构造函数切换为 llm.ToolCall 类型"
```

---

## Task 6：重构 Tool 接口与各实现

**Files:**
- Modify: `internal/domains/services/tools/base.go`
- Modify: `internal/domains/services/tools/convert.go`
- 视情况修改：`internal/domains/services/tools/mcp.go` / `custom.go` / `a2a.go` / `load_skill.go` / `builtin.go` / `execute_skill.go`

- [ ] **Step 6.1：改 `base.go` 接口签名**

```go
package tools

import (
    "mooc-manus/internal/domains/models"
    "mooc-manus/internal/domains/models/llm"
)

type Tool interface {
    GetTools() []llm.Tool
    HasTool(funcName string) bool
    Invoke(funcName, funcArgs string) models.ToolCallResult
    Init() error
    ProviderName() string
}
```

`BaseTool.GetTools()` 同步改成：

```go
func (t *BaseTool) GetTools() []llm.Tool {
    params := make([]llm.Tool, 0, len(t.functions))
    for _, function := range t.functions {
        params = append(params, convertDO2Tool(function))
    }
    return params
}
```

- [ ] **Step 6.2：改 `convert.go`**

```go
func convertDO2Tool(do models.ToolFunctionDO) llm.Tool {
    return llm.Tool{
        Name:        do.FunctionName,
        Description: do.FunctionDesc,
        Parameters:  do.Schema.Parameters,
    }
}
```

import 区域同步删 `github.com/openai/openai-go`、新增 `"mooc-manus/internal/domains/models/llm"`。

- [ ] **Step 6.3：编译该子包定位连锁失败**

> **前置 grep 排查**：先确认除了 `BaseTool` 自身以外，是否有自定义 `GetTools()` 实现。
>
> Run: `grep -nE "func \(.*\) GetTools\(\)" internal/domains/services/tools/`
> Expected：目前仅命中 `internal/domains/services/tools/base.go:func (t *BaseTool) GetTools()` 这一处；mcp/custom/a2a/skill 等实现都通过结构体嵌入 `BaseTool` 继承，无自定义。如果 grep 命中其它位置，说明存在自定义实现，需要同步把返回类型改成 `[]llm.Tool`。

Run: `go build ./internal/domains/services/tools/...`
Expected: 列出哪些 mcp/custom/a2a/skill 实现还引用 `openai.ChatCompletionToolParam`。按编译错误逐个修复。

修复原则：
- 自定义的 `GetTools()` 返回值改为 `[]llm.Tool`（按上一步 grep 结果确认数量）。
- 内部任何 `openai.FunctionDefinitionParam` 构造都改用 `llm.Tool{Name/Description/Parameters}`。
- 别处仍只调用 `HasTool / Invoke / Init / ProviderName` 的实现无需改动。

- [ ] **Step 6.4：再次编译子包通过**

Run: `go build ./internal/domains/services/tools/...`
Expected: 通过。

- [ ] **Step 6.5：跑 tools 包既有测试**

Run: `go test ./internal/domains/services/tools/... -v`
Expected: 既有用例 `tool_test.go` / `execute_skill_test.go` 仍 PASS（若失败必须先修复再继续）。

- [ ] **Step 6.6：提交**

```bash
git add internal/domains/services/tools/
git commit -m "refactor(tools): Tool 接口与各实现切换为 llm.Tool"
```

---

## Task 7：重构 BaseAgent（核心）

**Files:**
- Modify: `internal/domains/services/agents/base.go`

- [ ] **Step 7.1：先把结构体与字段切换**

将 `BaseAgent.llm *llm.OpenAiLLM` 改为 `BaseAgent.invoker llm.Invoker`（注意这里的 `llm` 是领域包 `mooc-manus/internal/domains/models/llm`；如果与基础设施 `llm` 包同时导入需取别名 `llmadapter` 处理 NewBaseAgent 调用方）。

```go
import (
    "mooc-manus/internal/domains/models"
    "mooc-manus/internal/domains/models/events"
    "mooc-manus/internal/domains/models/llm"
    "mooc-manus/internal/domains/models/memory"
    "mooc-manus/internal/domains/services/tools"
    "mooc-manus/pkg/logger"
    ...
)

type BaseAgent struct {
    name          string
    systemPrompt  string
    retryInterval int
    agentConfig   models.AgentConfig
    invoker       llm.Invoker
    memory        *memory.ChatMemory
    tools         []tools.Tool
}

func NewBaseAgent(agentConfig models.AgentConfig, invoker llm.Invoker, memory *memory.ChatMemory, tools []tools.Tool, systemPrompt string) *BaseAgent {
    return &BaseAgent{
        agentConfig:   agentConfig,
        invoker:       invoker,
        memory:        memory,
        tools:         tools,
        systemPrompt:  systemPrompt,
        retryInterval: 5,
    }
}
```

- [ ] **Step 7.2：迁移方法签名**

```go
func (a *BaseAgent) GetAvailableTools() []llm.Tool {
    out := make([]llm.Tool, 0)
    for _, t := range a.tools {
        out = append(out, t.GetTools()...)
    }
    return out
}

func (a *BaseAgent) GetMessages() []llm.Message { return a.memory.GetMessages() }

func (a *BaseAgent) AddToMemory(messages []llm.Message) {
    if a.memory.IsEmpty() {
        a.memory.AddMessage(llm.Message{Role: llm.RoleSystem, Content: a.systemPrompt})
    }
    a.memory.AddMessages(messages)
}
```

- [ ] **Step 7.3：迁移 `InvokeToolCalls`**

```go
func (a *BaseAgent) InvokeToolCalls(toolCalls []llm.ToolCall, eventCh chan<- events.AgentEvent) []llm.Message {
    toolMessages := make([]llm.Message, 0, len(toolCalls))
    for _, toolCall := range toolCalls {
        toolCallID := toolCall.ID
        funcName := toolCall.Name
        funcArgs := toolCall.Arguments

        repairedArgs, err := jsonrepair.JSONRepair(funcArgs)
        if err != nil {
            errMsg := fmt.Sprintf("工具调用参数不符合规范，修复失败：%v", err)
            toolMessages = append(toolMessages, llm.Message{
                Role: llm.RoleTool, Content: errMsg, ToolCallID: toolCallID,
            })
            result := models.ToolCallResult{Success: false, Message: errMsg}
            eventCh <- events.OnToolCallFail(toolCall, "", &result)
            continue
        }
        funcArgs = repairedArgs

        tool := a.GetTool(funcName)
        if tool == nil {
            errMsg := fmt.Sprintf("找不到工具%s对应的工具集", funcName)
            toolMessages = append(toolMessages, llm.Message{
                Role: llm.RoleTool, Content: errMsg, ToolCallID: toolCallID,
            })
            result := models.ToolCallResult{Success: false, Message: errMsg}
            eventCh <- events.OnToolCallFail(toolCall, "", &result)
            continue
        }

        eventCh <- events.OnToolCallStart(toolCall, tool.ProviderName())
        result := a.InvokeTool(tool, funcName, funcArgs)
        eventCh <- events.OnToolCallComplete(toolCall, tool.ProviderName(), &result)
        if !result.Success {
            eventCh <- events.OnToolCallFail(toolCall, tool.ProviderName(), &result)
            toolMessages = append(toolMessages, llm.Message{
                Role: llm.RoleTool, Content: "工具调用失败：" + result.Message, ToolCallID: toolCallID,
            })
        } else {
            toolMessages = append(toolMessages, llm.Message{
                Role: llm.RoleTool, Content: models.ConvertToolCallResult2Text(result), ToolCallID: toolCallID,
            })
        }
    }
    return toolMessages
}
```

- [ ] **Step 7.4：迁移 `InvokeLLM` / `StreamingInvokeLLM`**

将旧的 `a.llm.Invoke(...)` 改为 `a.invoker.Invoke(a.GetMessages(), availableTools)`；返回值已是 `llm.Message`，消息分发分支保持不变，只是把 `openai.AssistantMessage(content)` 改为 `llm.Message{Role: llm.RoleAssistant, Content: content}`，把 `message.ToParam()` 改为直接 append `message`（因为 `InvokeLLM` 现在直接返回 `llm.Message`）。

`StreamingInvokeLLM` 同理。

- [ ] **Step 7.5：迁移 `Invoke` / `StreamingInvoke`**

```go
func (a *BaseAgent) Invoke(query string, eventCh chan events.AgentEvent) {
    messages := []llm.Message{{Role: llm.RoleUser, Content: query}}
    ...
}

func (a *BaseAgent) StreamingInvoke(query string, eventCh chan events.AgentEvent) {
    messages := []llm.Message{{Role: llm.RoleUser, Content: query}}
    ...
}
```

注意：`StreamingInvoke` 内的 `messages = a.InvokeToolCalls(toolCalls, eventCh)` 这一行因为类型变化而自然兼容；`toolCalls` 现在是 `message.ToolCalls` 即 `[]llm.ToolCall`。

- [ ] **Step 7.6：删除对 `github.com/openai/openai-go` 的 import**

Run: `grep -n "openai-go" internal/domains/services/agents/base.go`
Expected: 无任何匹配。

- [ ] **Step 7.7：编译验证**

Run: `go build ./internal/domains/services/agents/...`
Expected: 仍会有连锁错误（plan / react / agent.go 还没改）；记录错误以指导 Task 8。`base.go` 本身应已无错。

- [ ] **Step 7.8：提交**

```bash
git add internal/domains/services/agents/base.go
git commit -m "refactor(agents): BaseAgent 切换为 llm.Invoker / llm.Message 抽象"
```

---

## Task 8：上层适配（PlanAgent / ReActAgent / PlanReActFlow / A2AExecutor / agent.go）

**Files:**
- Modify: `internal/domains/services/agents/plan.go`
- Modify: `internal/domains/services/agents/react.go`
- Modify: `internal/domains/services/agents/a2a_executor.go`
- Modify: `internal/domains/services/flows/plan_react.go`
- Modify: `internal/domains/services/agents/agent.go`
- Modify: `internal/domains/models/app_config.go`
- Modify: `internal/infra/models/app_config.go`

**前置：先把 `Provider` 字段加到 `ModelConfig` 与 `AppConfigPO`**（避免 Step 8.4 引用 `appConfig.ModelConfig.Provider` 时报 `unknown field`）。完整的 Provider 选择测试与 `pickInvoker` 抽取留到 Task 9。

- [ ] **Step 8.0：先给 `ModelConfig` 与 `AppConfigPO` 加 `Provider` 字段**

`internal/domains/models/app_config.go`：
```go
type ModelConfig struct {
    Provider    string
    BaseUrl     string
    ApiKey      string
    ModelName   string
    Temperature float64
    MaxTokens   int64
}
```

`internal/infra/models/app_config.go`：
```go
type AppConfigPO struct {
    ID               string    `gorm:"type:varchar(36);primary_key" json:"id"`
    Provider         string    `gorm:"type:varchar(32);not null;default:'openai'" json:"provider"`
    BaseUrl          string    `gorm:"type:varchar(255);not null" json:"baseUrl"`
    // ... 其余字段保持不变
}
```

同步把 `ConvertAppConfigDO2PO` / `ConvertAppConfigPO2DO` 两个转换函数中 `Provider` 字段对应映射加上。

Run: `go build ./internal/domains/models/... ./internal/infra/models/...`
Expected: 通过。

- [ ] **Step 8.1：以全量编译错误为指引**

Run: `go build ./...`
列出所有错误，逐个修复。

预期错误大致集中在：
- `plan.go` / `react.go` 内字段访问（依赖 BaseAgent 字段名是否改动；本次没改 `agent.llm` 之外的字段，但 `BaseAgent.llm` 已改名为 `invoker`，所有写法如 `agent.llm = baseAgent.llm` 需改为 `agent.invoker = baseAgent.invoker`）。
- `plan_react.go` 内 `NewPlanReActFlow(agentConfig, llm, ...)` 签名变更：第二个参数由 `*llm.OpenAiLLM` 改为 `llm.Invoker`（领域包），调用方传入 `llmadapter.NewOpenAIAdapter(...)`。
- `a2a_executor.go` 内 `e.agent.Invoke(...)` 调用不需要改动；只要 BaseAgent 类型本身仍提供 `Invoke` 即可。

- [ ] **Step 8.2：修 `plan.go` 与 `react.go`**

> **前置 grep 找全访问点**：
>
> Run: `grep -nE "\.llm\b" internal/domains/services/agents/`
> Expected：当前命中 `plan.go:30` 与 `react.go:23` 各一处 `agent.llm = baseAgent.llm`。如 grep 输出多于这两条，需要把每一条都迁移到 `invoker`，不要遗漏。

把所有 `agent.llm = baseAgent.llm` 改为 `agent.invoker = baseAgent.invoker`。
其它不变（这两个文件主要使用 BaseAgent 嵌入，方法调用不需要改）。

- [ ] **Step 8.3：修 `plan_react.go`**

把：
```go
func NewPlanReActFlow(agentConfig models.AgentConfig, llm *llm.OpenAiLLM, sessionId string, tools []tools.Tool) BaseFlow {
```
改为：
```go
import (
    domainllm "mooc-manus/internal/domains/models/llm"
    ...
)

func NewPlanReActFlow(agentConfig models.AgentConfig, invoker domainllm.Invoker, sessionId string, tools []tools.Tool) BaseFlow {
    ...
    plannerBaseAgent := agents.NewBaseAgent(agentConfig, invoker, memory.FetchMemory("planner::"+sessionId), tools, "")
    ...
    reActBaseAgent := agents.NewBaseAgent(agentConfig, invoker, memory.FetchMemory("react::"+sessionId), tools, "")
    ...
}
```

如有调用方（搜索 `NewPlanReActFlow`），一并改造（传入 adapter）。

- [ ] **Step 8.4：修 `agent.go` 的 `createBaseAgent`**

把：
```go
openAiLLM := llm.NewOpenAiLLM(appConfig.ModelConfig)
...
return NewBaseAgent(appConfig.AgentConfig, openAiLLM, chatMemory, baseTools, systemPrompt), nil
```
改为：
```go
import (
    domainllm "mooc-manus/internal/domains/models/llm"
    llmadapter "mooc-manus/internal/infra/external/llm"
    ...
)

var invoker domainllm.Invoker
switch appConfig.ModelConfig.Provider {
case "anthropic":
    invoker = llmadapter.NewAnthropicAdapter(appConfig.ModelConfig)
default:
    invoker = llmadapter.NewOpenAIAdapter(appConfig.ModelConfig)
}
...
return NewBaseAgent(appConfig.AgentConfig, invoker, chatMemory, baseTools, systemPrompt), nil
```

> `ModelConfig.Provider` 字段已在 Step 8.0 加入，这里直接引用即可。Task 9 会把 switch 逻辑抽取成 `pickInvoker` 并补充 TDD 测试。

- [ ] **Step 8.5：编译全量**

Run: `go build ./...`
Expected: 通过。如有任何剩余 openai 引用编译错误，按报错位置继续修。

- [ ] **Step 8.6：跑全量已有测试**

Run: `go test ./...`
Expected: 全部 PASS。

- [ ] **Step 8.7：提交**

```bash
git add internal/domains/services/
git commit -m "refactor(agents): 上层智能体与 flow 切换至 llm.Invoker 注入"
```

---

## Task 9：抽取 `pickInvoker` 并补 Provider 选择 TDD

> 字段添加与 PO/DO 转换已在 Task 8.0 完成；本任务只做：把 `createBaseAgent` 内部的 switch 抽成纯函数 `pickInvoker`，并补充 TDD 单测。

**Files:**
- Modify: `internal/domains/services/agents/agent.go`
- Create: `internal/domains/services/agents/agent_provider_test.go`

- [ ] **Step 9.1：写 Provider 选择分支的失败测试**

写入 `internal/domains/services/agents/agent_provider_test.go`：

```go
package agents

import (
    "testing"

    "mooc-manus/internal/domains/models"
    domainllm "mooc-manus/internal/domains/models/llm"
    llmadapter "mooc-manus/internal/infra/external/llm"
)

func TestPickInvoker_DefaultsToOpenAI(t *testing.T) {
    cfg := models.ModelConfig{Provider: ""}
    got := pickInvoker(cfg)
    if _, ok := got.(*llmadapter.OpenAIAdapter); !ok {
        t.Fatalf("default should be OpenAI, got %T", got)
    }
}

func TestPickInvoker_AnthropicBranch(t *testing.T) {
    cfg := models.ModelConfig{Provider: "anthropic"}
    got := pickInvoker(cfg)
    if _, ok := got.(*llmadapter.AnthropicAdapter); !ok {
        t.Fatalf("anthropic branch should return Anthropic adapter, got %T", got)
    }
}

// 编译期断言：pickInvoker 的返回类型实现 domainllm.Invoker
var _ domainllm.Invoker = pickInvoker(models.ModelConfig{})
```

- [ ] **Step 9.2：跑测试看红**

Run: `go test ./internal/domains/services/agents/... -run TestPickInvoker -v`
Expected: 编译失败（`pickInvoker` 未定义）。

- [ ] **Step 9.3：从 `createBaseAgent` 中抽出 `pickInvoker`**

在 `internal/domains/services/agents/agent.go` 中新增：

```go
func pickInvoker(cfg models.ModelConfig) domainllm.Invoker {
    switch cfg.Provider {
    case "anthropic":
        return llmadapter.NewAnthropicAdapter(cfg)
    default:
        return llmadapter.NewOpenAIAdapter(cfg)
    }
}
```

`createBaseAgent` 内部把 Step 8.4 写的 switch 替换为 `invoker := pickInvoker(appConfig.ModelConfig)`。

- [ ] **Step 9.4：跑测试看绿**

Run: `go test ./internal/domains/services/agents/... -run TestPickInvoker -v`
Expected: 2 个用例 PASS。

- [ ] **Step 9.5：提交**

```bash
git add internal/domains/services/agents/agent.go internal/domains/services/agents/agent_provider_test.go
git commit -m "refactor(agents): 抽取 pickInvoker 函数并补充选择分支 TDD"
```

---

## Task 10：清理死代码（`tests/mock.go` 与 file 包遗留 openai 引用）

**目的**：保证 `grep openai-go internal/ tests/` 只剩 `internal/infra/external/llm/openai*.go` 这两个合法位置；让验收标准"BaseAgent 链路不再引用 openai-go"实质达成。

**Files:**
- Delete: `tests/mock.go`（无任何调用方，纯类型示例）
- Modify: `internal/domains/models/file/file.go` — 删除 `Convert2UserMessage` 与 `ConvertMessage2QueryAndAttachments` 两个函数及对 openai-go 的 import

- [ ] **Step 10.1：确认 `tests/mock.go` 无调用方**

Run: `grep -rn "MockOpenAiMessage\|TestMockA2ARequestContext" --include="*.go" .`
Expected: 只命中 `tests/mock.go` 自身。

- [ ] **Step 10.2：删除 `tests/mock.go`**

```bash
git rm tests/mock.go
```

- [ ] **Step 10.3：确认 file 包内两个函数无调用方**

Run: `grep -rn "Convert2UserMessage\|ConvertMessage2QueryAndAttachments" --include="*.go" .`
Expected: 只命中 `internal/domains/models/file/file.go` 自身。

- [ ] **Step 10.4：删除两个函数与 openai import，保留 `File` 结构体**

修改后的 `file.go` 仅保留：

```go
package file

type File struct {
    ID        string
    FileName  string
    FilePath  string
    Key       string
    Extension string
    MimeType  string
    Size      int
}
```

- [ ] **Step 10.5：全量编译 + 全量测试**

Run: `go build ./... && go test ./...`
Expected: 通过。

- [ ] **Step 10.6：grep 兜底验证**

Run: `grep -rn "openai-go" --include="*.go" .`
Expected：仅命中下列三类文件中的 `import` 行（其它命中视为遗漏，需要继续清理）：

1. `internal/infra/external/llm/openai.go` — 底层 SDK 封装（import "github.com/openai/openai-go" 与 "github.com/openai/openai-go/option"）。
2. `internal/infra/external/llm/openai_adapter.go` — 适配器（import "github.com/openai/openai-go"）。
3. `internal/infra/external/llm/openai_adapter_test.go` — 适配器单测（import "github.com/openai/openai-go"）。

补充验证：`grep -rn "openai-go" --include="*.go" internal/domains/`
Expected：无任何输出（领域层完全无 OpenAI SDK 依赖）。

- [ ] **Step 10.7：提交**

```bash
git add tests/mock.go internal/domains/models/file/file.go
git commit -m "chore: 清理死代码（mock.go 与 file 包遗留 openai 引用）"
```

---

## Task 11：端到端回归与验收

**目的**：跑通现有 plan / react / a2a / skill 调用链，确认对外行为无回退。

- [ ] **Step 11.1：单元测试全绿**

Run: `go test ./...`
Expected: PASS。

- [ ] **Step 11.2：go vet 通过**

Run: `go vet ./...`
Expected: 无输出。

- [ ] **Step 11.2.1：依赖清单未引入新外部库**

Run: `go mod tidy && git diff -- go.mod go.sum`
Expected: `go.mod` / `go.sum` 无新增直接依赖（本次重构只做类型迁移，不应引入新的第三方包）。若 `git diff` 显示新增，回头排查是否误引入。

- [ ] **Step 11.3：grep openai-go 确认收敛**

Run: `grep -rn "openai-go" --include="*.go" internal/domains/`
Expected: 无匹配（领域层完全无 OpenAI 依赖）。

- [ ] **Step 11.4：grep 验证 BaseAgent 已切换抽象**

Run: `grep -n "OpenAiLLM\|openai\." internal/domains/services/agents/base.go`
Expected: 无匹配。

- [ ] **Step 11.5：手动启动服务，回归 chat / plan / react**

依赖项目现有启动方式（参考 `.harness/AGENTS.md` 与 `api/routers/route.go`）。
- 调用 `POST /chat` 或对应接口跑一次普通对话，确认 SSE 事件流正常。
- 触发 `CreatePlan` 流程，确认计划生成。
- 触发 `ReAct` 步骤执行，确认工具调用正常。

如果暂无端到端测试环境，在 PR 描述里写明"已 go test + go vet，端到端依赖人工冒烟"。

- [ ] **Step 11.6：核对验收清单**

逐条勾选 spec 第 9 节验收标准：

- [ ] `BaseAgent` 结构体不再引用 `github.com/openai/openai-go` 包。
- [ ] `memory.ChatMemory` 内部存储类型为 `[]llm.Message`。
- [ ] `tools.Tool.GetTools()` 返回 `[]llm.Tool`。
- [ ] `events.ToolEvent` 不再引用 OpenAI 工具调用类型。
- [ ] `OpenAIAdapter` 实现完整，OpenAI 协议下回归通过。
- [ ] `AnthropicAdapter` 接口齐全，调用返回 not-implemented 错误。
- [ ] 单元测试覆盖 Message ↔ openai.* 双向转换核心路径。
- [ ] `go build ./...` 全量编译通过。
- [ ] `go test ./...` 全量测试通过。

- [ ] **Step 11.7：合并前提交一次总结性 commit（如有 cleanup）**

```bash
git status
# 如有零碎清理，单独提交：
git add ...
git commit -m "chore: LLM 协议抽象重构收尾清理"
```

---

## 风险与回滚

- **回滚单位**：按 Task 划分的 commit 顺序回退即可（`git revert` 或 `git reset --hard <task-N-的前一个 commit>`）。
- **高风险点**：
  - Task 4 修复 `Compact` 拼写 bug 后，记忆中 browser / bing_search 工具结果会被替换为 `"removed"`，可能改变长对话的上下文。如果回归发现 LLM 推理质量下降，可在 Task 4 commit 内单独 revert `Compact` 行为变更（保留类型切换）。
  - Task 8 中 `NewPlanReActFlow` 签名变更涉及外部调用方，务必 `grep NewPlanReActFlow` 把所有调用点改全。
- **未覆盖**：Anthropic Adapter 调用方若真的把 `ModelConfig.Provider="anthropic"` 配上线，会立刻返回 not-implemented 错误。属于预期行为。

---

## 实施完成后的下一步

`subagent-driven-development` / `executing-plans` 任意一种执行方式都可用：
- 推荐 `subagent-driven-development`：每个 Task 派一个独立 subagent，主会话只做评审与下一步分发。
- 若你希望一气呵成在主会话内执行，则用 `executing-plans`。