# LLM 协议抽象（Message / Tool / Invoker）

> 后端 LLM 调用统一走"厂商无关的值对象 + `Invoker` 接口"，具体 SDK 封装在 adapter 层。强约束见 `R-42-llm`；决策依据见 `mooc-manus-all/.harness/retro/decisions/ADR-0001-llm-protocol-abstraction.md`。本文聚焦"代码层面是怎么落的"。

## 三件套位置

```
internal/domains/models/
├── llm/
│   ├── message.go        # Role / ToolCall / Message
│   └── tool.go           # Tool
└── invoker/
    └── invoker.go        # Invoker 接口
internal/infra/external/llm/
├── openai_adapter.go     # OpenAI 实现
└── anthropic_adapter.go  # Anthropic 实现（骨架）
```

## llm.Message（`message.go`）

```go
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
    Arguments string  // 厂商无关：始终是 JSON 字符串，由 Agent 侧 jsonrepair 后再解析
}

type Message struct {
    Role       Role
    Content    string
    ToolCalls  []ToolCall      // assistant 决定要调的工具
    ToolCallID string          // tool 响应时回填，匹配上一条 assistant 的 ToolCall.ID
    Extra      map[string]any  // 厂商专属字段的逃生舱（多模态 / thinking / cache-control 等）
}
```

设计决定（来自 ADR-0001 §"决策" 第 1、4 条）：

- 字段保持厂商无关；任何只在某 SDK 存在的语义（如 Anthropic 的 thinking blocks、OpenAI 的 audio / image 部分）都走 `Extra map[string]any`，避免一上来就把抽象切死。
- `Arguments` 始终是 JSON 字符串（不是 `map[string]any`）。理由：(a) 与 OpenAI / Anthropic 的 wire format 对齐，少一次反序列化；(b) LLM 偶尔会返回坏 JSON，统一在 `BaseAgent.InvokeToolCalls` 用 `jsonrepair` 修复。

## llm.Tool（`tool.go`）

```go
type Tool struct {
    Name        string
    Description string
    Parameters  map[string]any  // JSON Schema，结构由 ToolFunctionDO 决定
}
```

工具 → `llm.Tool` 的实际转换路径：`tools.BaseTool.GetTools()` → `convertDO2Tool(function ToolFunctionDO)`（`internal/domains/services/tools/convert.go`）。这一步在 domain 层完成，仍是厂商无关的；adapter 再把 `llm.Tool` 转成 SDK 的 tool schema。

## invoker.Invoker（`invoker.go`）

```go
type Invoker interface {
    Invoke(messages []llm.Message, tools []llm.Tool) (llm.Message, error)
    StreamingInvoke(messages []llm.Message, tools []llm.Tool, eventCh chan<- events.AgentEvent) llm.Message
}
```

通道生命周期约定（注释固化）：`StreamingInvoke` 的 `eventCh` 由调用方创建并传入；**adapter 在流结束（或出错上报后）调用 `close(eventCh)`**，调用方不要自行 close —— 与 R-45 + Agent 侧的 `for event := range llmEventCh` 模式契合。

## Adapter 装配（`PickInvoker`）

`internal/domains/services/agents/agent.go::PickInvoker`：

```go
func PickInvoker(cfg models.ModelConfig) invoker.Invoker {
    switch cfg.Provider {
    case "anthropic":
        return llmadapter.NewAnthropicAdapter(cfg)
    default:
        return llmadapter.NewOpenAIAdapter(cfg)
    }
}
```

- 路由依据是 `ModelConfig.Provider` 字段（ADR-0001 §"决策" 第 5 条），空字符串或未知值回退 OpenAI（"为新厂商失败的兜底"）。
- 由于 R-42 的 deny-list 仅作用于 `internal/domains/`，`agent.go` 这里 import `llmadapter` 是合法的—— DI 入口属于 domain service 但调用的是 adapter 工厂构造函数，无需触碰 SDK 类型。

## 与 ChatMemory / 事件流的接缝

- ChatMemory 内的 `messages []llm.Message` 也用值对象（详见 `memory-lifecycle.md`），保证 memory ↔ adapter 之间无 SDK 类型穿透。
- `BaseAgent.GetAvailableTools()` 聚合所有 `tools.Tool.GetTools()`，输出 `[]llm.Tool` 给 invoker；与 R-44 §"统一 tools.Tool 接口" 一致。
- 事件层的 `ToolEvent.ToolCallID / FunctionName / FunctionArgs` 都源自 `llm.ToolCall`（见 `events/tools.go::convert2ToolEvent`），无 SDK 字段。

## 新增 LLM 厂商的最少改动

依 R-42 §"Adapter 位于 infrastructure 层"：

1. 评估 `llm.Message` 字段是否够用；不够则在 `message.go` 加字段或走 `Extra`。
2. 在 `internal/infra/external/llm/` 新建 `<vendor>_adapter.go`，实现 `Invoker` 接口；做 `llm.Message ↔ SDK` 双向转换。
3. `PickInvoker` 增加 `case "<vendor>"`。
4. 不需要改 `BaseAgent` / `ChatMemory` / `events` —— 这是抽象成功的检验标准。
5. 视情况落新 ADR（若引入了 `Message` 字段升级 → supersede ADR-0001）。

## 静态约束（R-42 §"可验证性"）

```
grep -rn "github.com/sashabaranov/go-openai" internal/domains/   # 应为空
grep -rn "github.com/anthropics"             internal/domains/   # 应为空
grep -rn "github.com/tmc/langchaingo"        internal/domains/   # 应为空
```

CI 在 `ddd-layer-checker` 子代理与 pre-commit hook 双轨执行这三条 grep。adapter 文件因为位于 `internal/infra/external/llm/`，不会被命中。

## 历史回顾

完整背景见 ADR-0001 §"背景" / §"实施 / 跟进"。关键里程碑 commit（在子仓 mooc-manus 内）：

- `216ea38` — feat(llm) 统一消息体抽象诞生
- `739be68` — feat(invoker) `Invoker` 接口诞生
- `236dc1d` — `ChatMemory` 切换为 `llm.Message`
- `5523244` — `BaseAgent` 切换为 `invoker.Invoker`

总仓侧锚点 commit `f9c1823`。
