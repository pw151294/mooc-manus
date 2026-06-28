# 智能体对话框架重构设计文档：LLM 协议抽象

- **日期**：2026-06-28
- **作者**：项目团队
- **状态**：Draft（待评审）
- **关联代码**：`internal/domains/services/agents/base.go`、`internal/domains/models/memory/`、`internal/infra/external/llm/`

---

## 1. 背景

当前 `BaseAgent` 与大模型对话的整条链路都强耦合在 OpenAI 协议上：

- `BaseAgent.llm` 字段类型为 `*llm.OpenAiLLM`，只能调用 OpenAI SDK。
- `BaseAgent.memory` 内部以 `[]openai.ChatCompletionMessageParamUnion` 存储历史会话。
- 所有 LLM 交互方法签名（`InvokeLLM`/`StreamingInvokeLLM`/`InvokeToolCalls`/`AddToMemory`/`GetMessages`/`GetAvailableTools`）均使用 OpenAI 类型。
- `tools.Tool.GetTools()` 直接返回 `[]openai.ChatCompletionToolParam`。
- `events.ToolEvent` 构造函数参数为 `openai.ChatCompletionMessageToolCall`。

这导致框架无法接入 Anthropic（Claude）等其他厂商的对话协议。本次重构目标：抽象出厂商无关的统一消息体与调用接口，让 `BaseAgent` 可以同时承载多种 LLM 协议。

---

## 2. 目标与非目标

### 2.1 目标

1. 抽象出统一的 `Message` / `ToolCall` / `Tool` 结构体，作为框架内大模型交互的唯一消息载体。
2. 抽象出 `Invoker` 接口，定义 `Invoke` 和 `StreamingInvoke` 两个核心方法，由各厂商适配器实现。
3. `BaseAgent`、`ChatMemory`、`tools.Tool`、`events` 全链路切换为统一消息体；不再引用 `github.com/openai/openai-go`。
4. 完整实现 OpenAI 适配器，保证所有现有功能（plan / react / a2a / skill）继续可用。
5. 为 Anthropic 适配器预留接口骨架（方法签名齐全，函数体留空），后续独立任务再补全。
6. 通过 `ModelConfig.Provider` 字段在运行期选择适配器，支持配置驱动的协议切换。

### 2.2 非目标

- Anthropic 适配器的实际 SDK 调用实现。
- 模型配置在 API 层、前端层的暴露与切换。
- A2A 工具协议层面的双轨支持。
- 多模态、thinking blocks、prompt caching 等厂商特性的具体落地（仅通过 `Extra` 字段预留通道）。

---

## 3. 现状梳理

### 3.1 BaseAgent 当前形态

`internal/domains/services/agents/base.go:21-29`：

```go
type BaseAgent struct {
    name          string
    systemPrompt  string
    retryInterval int
    agentConfig   models.AgentConfig
    llm           *llm.OpenAiLLM
    memory        *memory.ChatMemory
    tools         []tools.Tool
}
```

关键方法的 OpenAI 耦合点：

- `GetAvailableTools() []openai.ChatCompletionToolParam`
- `GetMessages() []openai.ChatCompletionMessageParamUnion`
- `AddToMemory(messages []openai.ChatCompletionMessageParamUnion)`
- `InvokeToolCalls(toolCalls []openai.ChatCompletionMessageToolCall, ...) []openai.ChatCompletionMessageParamUnion`
- `InvokeLLM(messages []openai.ChatCompletionMessageParamUnion) (openai.ChatCompletionMessage, error)`
- `StreamingInvokeLLM(messages []openai.ChatCompletionMessageParamUnion, ...) openai.ChatCompletionMessage`

### 3.2 Memory 当前形态

`internal/domains/models/memory/memory.go:9-12`：

```go
type ChatMemory struct {
    messages        []openai.ChatCompletionMessageParamUnion
    toolCallId2Name map[string]string
}
```

### 3.3 Tool 当前形态

`internal/domains/services/tools/base.go:9-15`：

```go
type Tool interface {
    GetTools() []openai.ChatCompletionToolParam
    HasTool(funcName string) bool
    Invoke(funcName, funcArgs string) models.ToolCallResult
    Init() error
    ProviderName() string
}
```

### 3.4 Events 当前形态

`internal/domains/models/events/events.go:32-42` 与 `internal/domains/models/events/tools.go` 中，`ToolEvent` 及其构造函数 `OnToolCallStart/Complete/Fail` 均接收 `openai.ChatCompletionMessageToolCall` 类型参数。

### 3.5 LLM 层当前形态

`internal/infra/external/llm/openai.go` 内 `OpenAiLLM.Invoke` 与 `OpenAiLLM.StreamingInvoke` 直接返回 `openai.ChatCompletionMessage`。

### 3.6 上层调用链

- `PlanAgent`/`ReActAgent` 嵌入 `BaseAgent`，继承其方法，未直接操作 OpenAI 类型。
- `PlanReActFlow.NewPlanReActFlow` 直接构造 `*llm.OpenAiLLM` 并注入 BaseAgent。
- `BaseAgentDomainServiceImpl.createBaseAgent` 通过 `llm.NewOpenAiLLM(appConfig.ModelConfig)` 构造 LLM 实例。

---

## 4. 设计方案

### 4.1 包结构与命名

```
internal/domains/models/llm/         # 领域层 - 统一消息抽象
  ├── message.go                     # Message / ToolCall / Role
  ├── tool.go                        # Tool 定义
  └── invoker.go                     # Invoker 接口

internal/infra/external/llm/         # 基础设施层 - 厂商适配器
  ├── openai.go                      # 保留：底层 SDK 封装
  ├── openai_adapter.go              # 新增：OpenAI Invoker 适配器
  └── anthropic_adapter.go           # 新增：Anthropic Invoker 接口骨架
```

调用方式：`llm.Message{...}` / `llm.Tool{...}` / `llm.ToolCall{...}` / `llm.Invoker`。

注：与基础设施层包 `internal/infra/external/llm` 同名，但导入路径不同，调用方按需用别名区分（约定：领域包用 `llm`，基础设施包用 `llmadapter` 或在调用处用别名）。

### 4.2 统一消息体（最小公约数 + 扩展字段）

```go
// internal/domains/models/llm/message.go
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
    Arguments string // JSON 字符串
}

type Message struct {
    Role       Role
    Content    string         // 文本内容
    ToolCalls  []ToolCall     // assistant 角色专属
    ToolCallID string         // tool 角色专属：工具响应 ID
    Extra      map[string]any // 厂商特性扩展（图片/thinking/cache_control 等）
}
```

设计要点：

- 核心字段仅覆盖"文本 + 工具调用"这一最小公约数。
- `Extra` 字段以 `map[string]any` 透传厂商特性，由对应适配器在转换时识别。
- 不在领域层暴露任何厂商类型；所有 OpenAI / Anthropic 类型转换全部封装在适配器内部。

### 4.3 Tool 抽象

```go
// internal/domains/models/llm/tool.go
type Tool struct {
    Name        string
    Description string
    Parameters  map[string]any // JSON Schema
}
```

### 4.4 Invoker 接口

```go
// internal/domains/models/llm/invoker.go
type Invoker interface {
    Invoke(messages []Message, tools []Tool) (Message, error)
    StreamingInvoke(messages []Message, tools []Tool, eventCh chan<- events.AgentEvent) Message
}
```

**channel 生命周期约定（与现有 `OpenAiLLM.StreamingInvoke` 一致）**：

- `eventCh` 由调用方（BaseAgent）创建并传入。
- 适配器负责在流式接收过程中向 `eventCh` 写入事件，并在流结束（或出错上报后）调用 `close(eventCh)`。
- Anthropic 骨架实现因为无任何事件可写，可直接调用 `close(eventCh)` 后返回零值；这与"适配器负责 close"的总约定一致。

### 4.5 OpenAI 适配器

```go
// internal/infra/external/llm/openai_adapter.go
type OpenAIAdapter struct {
    llm *OpenAiLLM // 复用现有 SDK 封装
}

func NewOpenAIAdapter(cfg models.ModelConfig) *OpenAIAdapter {
    return &OpenAIAdapter{llm: NewOpenAiLLM(cfg)}
}

func (a *OpenAIAdapter) Invoke(messages []llm.Message, tools []llm.Tool) (llm.Message, error) {
    openaiMsgs := toOpenAIMessages(messages)
    openaiTools := toOpenAITools(tools)
    resp, err := a.llm.Invoke(openaiMsgs, openaiTools)
    if err != nil {
        return llm.Message{}, err
    }
    return fromOpenAIMessage(resp), nil
}

func (a *OpenAIAdapter) StreamingInvoke(messages []llm.Message, tools []llm.Tool, eventCh chan<- events.AgentEvent) llm.Message {
    // 类似转换 + 流式接收
}
```

转换函数职责：

- `toOpenAIMessages([]llm.Message) []openai.ChatCompletionMessageParamUnion`：按 Role 分别构造 `SystemMessage / UserMessage / AssistantMessage / ToolMessage`。
- `toOpenAITools([]llm.Tool) []openai.ChatCompletionToolParam`：构造 `FunctionDefinitionParam`。
- `fromOpenAIMessage(openai.ChatCompletionMessage) llm.Message`：把 content 与 tool_calls 还原到 `llm.Message`。

### 4.6 Anthropic 适配器骨架

```go
// internal/infra/external/llm/anthropic_adapter.go
type AnthropicAdapter struct {
    cfg models.ModelConfig
}

func NewAnthropicAdapter(cfg models.ModelConfig) *AnthropicAdapter {
    return &AnthropicAdapter{cfg: cfg}
}

func (a *AnthropicAdapter) Invoke(messages []llm.Message, tools []llm.Tool) (llm.Message, error) {
    return llm.Message{}, errors.New("anthropic adapter not implemented")
}

func (a *AnthropicAdapter) StreamingInvoke(messages []llm.Message, tools []llm.Tool, eventCh chan<- events.AgentEvent) llm.Message {
    close(eventCh)
    return llm.Message{}
}
```

骨架要求：

- 结构体、构造函数、方法签名齐全，可被 `createBaseAgent` 引用。
- `Invoke` 返回 `errors.New("anthropic adapter not implemented")`。
- `StreamingInvoke` 按 4.4 节 channel 生命周期约定 close 通道后返回零值（骨架阶段无任何事件可写）。

### 4.7 ChatMemory 重构

```go
// internal/domains/models/memory/memory.go
type ChatMemory struct {
    messages        []llm.Message
    toolCallId2Name map[string]string
}

func (c *ChatMemory) AddMessage(message llm.Message) { ... }
func (c *ChatMemory) AddMessages(messages []llm.Message) { ... }
func (c *ChatMemory) GetMessages() []llm.Message { ... }
func (c *ChatMemory) Compact() { ... }
```

- 角色判断从 `*message.GetRole() == "assistant"` 改为 `message.Role == llm.RoleAssistant`。
- `Compact` 的判断条件改为 `message.Role == llm.RoleTool`，借助 `toolCallId2Name` 查名字。

**附带 Bug 修复**：现有 `Compact` 判断的字符串是 `"tools"`（复数），而 OpenAI 协议里 tool 消息的 role 实际是 `"tool"`（单数），导致循环体永远不会执行、压缩逻辑形同虚设。重构后统一使用 `llm.RoleTool`（= `"tool"`），该判断将真正生效。属于行为变更，已在改动清单中标注。

### 4.8 Tool 接口重构

```go
// internal/domains/services/tools/base.go
type Tool interface {
    GetTools() []llm.Tool
    HasTool(funcName string) bool
    Invoke(funcName, funcArgs string) models.ToolCallResult
    Init() error
    ProviderName() string
}
```

`BaseTool.GetTools()` 与 `convertDO2Tool` 同步改为返回 `llm.Tool`。

### 4.9 Events 重构

`events.ToolEvent` 字段移除对 `openai.ChatCompletionMessageToolCall` 的依赖：

```go
type ToolEvent struct {
    BaseEvent
    Timestamp      time.Time
    ToolCallID     string
    ToolName       string
    FunctionName   string
    FunctionArgs   string
    FunctionResult *models.ToolCallResult
    Status         ToolEventStatus
}

func OnToolCallStart(toolCall llm.ToolCall, toolName string) AgentEvent { ... }
func OnToolCallComplete(toolCall llm.ToolCall, toolName string, result *models.ToolCallResult) AgentEvent { ... }
func OnToolCallFail(toolCall llm.ToolCall, toolName string, result *models.ToolCallResult) AgentEvent { ... }
```

字段映射（从 `llm.ToolCall` 到 `ToolEvent`）：

| ToolEvent 字段 | 取值来源 |
|---------------|---------|
| `ToolCallID` | `toolCall.ID` |
| `FunctionName` | `toolCall.Name`（对应原 OpenAI 的 `Function.Name`） |
| `FunctionArgs` | `toolCall.Arguments`（对应原 OpenAI 的 `Function.Arguments`） |
| `ToolName` | 由调用方传入的 `toolName` 参数 |

说明：`llm.ToolCall` 已经把 OpenAI 嵌套的 `Function.Name / Function.Arguments` 扁平化为 `Name / Arguments`，转换函数直接平铺访问即可，无需额外二级解包。

### 4.10 BaseAgent 重构

```go
type BaseAgent struct {
    name          string
    systemPrompt  string
    retryInterval int
    agentConfig   models.AgentConfig
    invoker       llm.Invoker
    memory        *memory.ChatMemory
    tools         []tools.Tool
}

func (a *BaseAgent) GetAvailableTools() []llm.Tool
func (a *BaseAgent) GetMessages() []llm.Message
func (a *BaseAgent) AddToMemory(messages []llm.Message)
func (a *BaseAgent) InvokeToolCalls(toolCalls []llm.ToolCall, eventCh chan<- events.AgentEvent) []llm.Message
func (a *BaseAgent) InvokeLLM(messages []llm.Message) (llm.Message, error)
func (a *BaseAgent) StreamingInvokeLLM(messages []llm.Message, eventCh chan<- events.AgentEvent) llm.Message
```

- 构造 system 消息：`llm.Message{Role: llm.RoleSystem, Content: a.systemPrompt}`。
- 构造 user 消息：`llm.Message{Role: llm.RoleUser, Content: query}`。
- 构造 assistant 消息：`llm.Message{Role: llm.RoleAssistant, Content: content, ToolCalls: toolCalls}`。
- 构造 tool 消息：`llm.Message{Role: llm.RoleTool, Content: result, ToolCallID: toolCallID}`。

`InvokeToolCalls` 中：

- 不再调用 `openai.ToolMessage(...)`，直接构造 `llm.Message{Role: RoleTool, ...}`。
- 上报事件使用 `llm.ToolCall` 类型。

### 4.11 配置扩展

```go
// internal/domains/models/app_config.go
type ModelConfig struct {
    Provider    string  // "openai" | "anthropic"
    BaseUrl     string
    ApiKey      string
    ModelName   string
    Temperature float64
    MaxTokens   int64
}
```

`createBaseAgent` 改造：

```go
var invoker llm.Invoker
switch appConfig.ModelConfig.Provider {
case "anthropic":
    invoker = adapter.NewAnthropicAdapter(appConfig.ModelConfig)
default: // "openai" 或空值
    invoker = adapter.NewOpenAIAdapter(appConfig.ModelConfig)
}
```

**持久化层处理范围（明确边界）**：

- 本次会修改 `internal/infra/models/app_config.go` 的 `AppConfigPO` 结构体，新增 `Provider string` 字段（GORM 标签：`gorm:"type:varchar(32);not null;default:'openai'"`）；同步更新 `ConvertAppConfigDO2PO` / `ConvertAppConfigPO2DO` 两个转换函数。
- 数据库迁移脚本（如 SQL 文件、GORM AutoMigrate 调用调整等）**不在本次范围**：依赖项目现有的 schema 演进机制（若启用了 GORM AutoMigrate，新字段会在应用启动时自动加列）。
- 历史数据中 `Provider` 为空字符串时，`createBaseAgent` 的 `default` 分支会回退到 OpenAI 适配器，保证向后兼容。

---

## 5. 改动清单

| 文件 | 改动 | 说明 |
|------|------|------|
| `internal/domains/models/llm/message.go` | 新增 | `Message` / `ToolCall` / `Role` |
| `internal/domains/models/llm/tool.go` | 新增 | `Tool` |
| `internal/domains/models/llm/invoker.go` | 新增 | `Invoker` 接口 |
| `internal/infra/external/llm/openai_adapter.go` | 新增 | OpenAI 完整实现 + 双向转换 |
| `internal/infra/external/llm/anthropic_adapter.go` | 新增 | Anthropic 接口骨架 |
| `internal/infra/external/llm/openai.go` | 保留 | 作为 OpenAI Adapter 的底层 SDK 封装 |
| `internal/domains/models/memory/memory.go` | 重构 | 内部存储改为 `[]llm.Message`；`Compact` 角色判断字符串从 `"tools"` 修正为 `llm.RoleTool`（行为变更：原压缩逻辑因字符串拼写错误从未生效，重构后真正生效） |
| `internal/domains/models/memory/manager.go` | 重构 | 同步替换类型 |
| `internal/domains/services/agents/base.go` | 重构 | 所有方法签名改为 `llm.*` |
| `internal/domains/services/tools/base.go` | 重构 | `Tool.GetTools()` 返回 `[]llm.Tool` |
| `internal/domains/services/tools/convert.go` | 重构 | `convertDO2Tool` 返回 `llm.Tool` |
| `internal/domains/services/tools/*.go` | 适配 | mcp/custom/a2a 工具实现同步改造 |
| `internal/domains/models/events/events.go` | 重构 | `ToolEvent` 字段不再依赖 openai 类型 |
| `internal/domains/models/events/tools.go` | 重构 | 构造函数参数改为 `llm.ToolCall` |
| `internal/domains/services/agents/plan.go` | 适配 | 无方法签名改动 |
| `internal/domains/services/agents/react.go` | 适配 | 无方法签名改动 |
| `internal/domains/services/flows/plan_react.go` | 适配 | 改为构造 `llm.Invoker` 注入 BaseAgent |
| `internal/domains/services/agents/agent.go` | 适配 | `createBaseAgent` 按 Provider 选择适配器 |
| `internal/domains/models/app_config.go` | 扩展 | `ModelConfig.Provider`、PO/DO 转换函数同步 |
| `internal/infra/models/app_config.go` | 扩展 | 结构体新增 `Provider string` GORM 字段（默认 `'openai'`）；数据库迁移依赖现有 schema 演进机制，本次不写独立迁移脚本 |

---

## 6. 实施顺序

按依赖拓扑分阶段执行：

1. **新建领域抽象层**：`llm.Message` / `Tool` / `ToolCall` / `Invoker`。
2. **新建 OpenAI 适配器**：实现 `llm.Invoker`，封装双向转换函数。
3. **新建 Anthropic 适配器骨架**：方法体 panic / 返回错误。
4. **重构 Memory 层**：内部存储替换为 `llm.Message`。
5. **重构 Tool 体系**：`Tool` 接口及所有实现（mcp / custom / a2a / skill）切换签名。
6. **重构 Events 层**：`ToolEvent` 与构造函数适配。
7. **重构 BaseAgent**：切换所有方法签名，构造统一消息体。
8. **适配上层**：`PlanAgent` / `ReActAgent` / `PlanReActFlow` / `agent.go` 编译修复。
9. **配置扩展**：`ModelConfig.Provider` + `createBaseAgent` Provider 分支。

**关键依赖关系**：BaseAgent 的方法签名依赖 `llm.Message` 与重构后的 Memory / Tool / Events 类型，因此阶段 7（BaseAgent）必须在阶段 4-6 全部完成之后执行，不能并行。

**编译校验节奏**：

- 阶段 1-3 新增独立包/文件，未触碰现有依赖，`go build ./internal/domains/models/llm/... ./internal/infra/external/llm/...` 可单独通过；但 `go build ./...` 在阶段 4-7 过程中可能因混合类型暂时无法通过。
- 阶段 8 完成后 `go build ./...` 必须全量通过。
- 阶段 9 完成后再次确认 `go build ./...` 通过并跑一遍现有回归。

---

## 7. 错误处理

- 适配器内部转换失败应通过 `error` 返回，不 panic。
- `Anthropic.Invoke` 返回 `errors.New("anthropic adapter not implemented")`，调用方按现有重试 / 失败上报路径处理。
- `Anthropic.StreamingInvoke` 因为不产生任何事件，按 4.4 节 channel 生命周期约定，直接 `close(eventCh)` 后返回零值，避免调用方阻塞。
- `Extra` 字段缺失或类型不匹配时，适配器按"忽略+日志告警"处理，不影响主流程。

---

## 8. 测试

新增单元测试至少覆盖以下转换路径：

- `toOpenAIMessages` / `fromOpenAIMessage` 双向转换（含 system / user / assistant + tool_calls / tool 四种 Role）。
- `toOpenAITools` 字段映射正确性。
- `ChatMemory.AddMessage / GetMessages / Compact` 行为不退化。
- `events.OnToolCallStart/Complete/Fail` 在新签名下构造结果正确。

集成验证：

- 现有 plan / react / a2a / skill 流程在 OpenAI 协议下端到端跑通（手动 + 现有集成测试）。

---

## 9. 验收标准

- [ ] `BaseAgent` 结构体不再引用 `github.com/openai/openai-go` 包。
- [ ] `memory.ChatMemory` 内部存储类型为 `[]llm.Message`。
- [ ] `tools.Tool.GetTools()` 返回 `[]llm.Tool`。
- [ ] `events.ToolEvent` 不再引用 OpenAI 工具调用类型。
- [ ] `OpenAIAdapter` 实现完整，所有现有功能在 OpenAI 协议下通过端到端验证。
- [ ] `AnthropicAdapter` 接口齐全，方法体返回 not-implemented 错误或直接 close 通道。
- [ ] 单元测试覆盖 Message ↔ openai.* 双向转换核心路径。
- [ ] `go build ./...` 全量编译通过。
- [ ] 现有 plan / react / skill 调用链回归验证通过。

---

## 10. 非目标 / 后续工作

- Anthropic 适配器实际 SDK 调用实现（独立任务）。
- 多模态、thinking blocks、prompt caching 等厂商特性落地。
- A2A 工具按厂商协议双轨支持。
- Provider 配置在 API 层 / 前端层的暴露与切换。
- 独立的 `AppConfigPO` 数据库迁移脚本（本次依赖现有 schema 演进机制；结构体改动本身在范围内，详见 4.11 节）。

---

## 11. 风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| 双向转换覆盖不全导致信息丢失 | 高 | 单元测试覆盖 4 种 Role 的双向转换；`Extra` 字段透传未识别字段 |
| 流式事件 close 时机不一致 | 中 | 在 Invoker 接口文档化 close 责任由适配器承担 |
| 上层调用站点遗漏 | 中 | 通过 `go build ./...` 全量编译确认；grep `openai-go` 验证最终结果 |
| `Provider` 字段历史数据为空 | 低 | `createBaseAgent` 默认走 OpenAI 分支 |
