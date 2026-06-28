---
rule_id: R-42-llm
severity: high
---

# LLM 协议抽象（Message / Tool 值对象）

LLM 调用必须通过 `internal/domains/models/llm/` 下的 `Message` / `Tool` / `ToolCall` 值对象与统一 `Invoker` 接口（`internal/domains/models/invoker/invoker.go`）。具体 SDK 调用封装在 infrastructure 层。

## 禁止行为

1. **禁止 domains 直接 import LLM 厂商 SDK**
   - 禁止：`internal/domains/services/agents/*.go` 出现
     ```go
     import "github.com/sashabaranov/go-openai"
     import "github.com/anthropics/anthropic-sdk-go"
     import "github.com/tmc/langchaingo/llms/openai"
     ```
   - Domain 层仅 import `internal/domains/models/llm` 与 `internal/domains/models/invoker`

2. **禁止泄漏 SDK 数据结构**
   - 不得让 SDK 的 `openai.ChatCompletionMessage` / `anthropic.MessageParam` 出现在 Domain Service / Application Service 签名中
   - 转换在 adapter 内完成；out: SDK 类型，in: `llm.Message`

3. **禁止跨过 Invoker 直接调用 HTTP**
   - 禁止：`internal/domains/services/` 内出现 `http.Post("https://api.openai.com/...")` 等直连
   - 所有 LLM 调用必须经 `Invoker.Invoke(ctx, request) (response, error)`

## 要求行为

1. **值对象集中**（参考 `internal/domains/models/llm/message.go` 与 `tool.go`）
   - `llm.Message { Role, Content, ToolCalls, ToolCallID, Extra }`
   - `llm.ToolCall { ID, Name, Arguments }`
   - `llm.Tool { Name, Description, Parameters }`
   - 新增字段需保持厂商无关（如多模态扩展走 `Extra map[string]any`）

2. **Adapter 位于 infrastructure 层**
   - 各厂商实现位于 `internal/infra/external/llm/`（如 `openai_invoker.go` / `anthropic_invoker.go`）
   - 每个 Adapter 实现 `invoker.Invoker` 接口，做 `llm.Message ↔ SDK type` 的双向转换
   - DI 在 `api/routers/route.go::InitRouter` 装配，按模型 ID 路由到对应 Invoker
   - **adapter 层例外**：adapter 文件位于 `internal/infra/external/llm/`，可 import LLM SDK；静态检查的 deny-list 仅作用于 `internal/domains/`。

3. **正例**
   ```go
   // internal/domains/services/agents/base.go
   import (
       "mooc-manus/internal/domains/models/invoker"
       "mooc-manus/internal/domains/models/llm"
   )

   func (s *BaseAgentDomainServiceImpl) Chat(ctx context.Context, msgs []llm.Message) (*llm.Message, error) {
       return s.invoker.Invoke(ctx, invoker.Request{Messages: msgs, Tools: s.tools})
   }
   ```

   ```go
   // internal/infra/external/llm/openai_invoker.go
   func (a *OpenAIInvoker) Invoke(ctx context.Context, req invoker.Request) (*invoker.Response, error) {
       sdkMsgs := convertToOpenAIMessages(req.Messages) // adapter 边界
       // ... 调 openai SDK
   }
   ```

## Agent 行为

- 检测到 `internal/domains/` 文件 import openai-go / anthropic-go / langchaingo 等 SDK → 拒绝并要求新建 / 复用 adapter
- 新增 LLM 厂商接入 spec → 强制走"先扩 `llm.Message`（必要时） → 写 adapter → DI 注册"三步
- 看到 `*openai.ChatCompletion*` / `*anthropic.MessageParam*` 类型出现在 Domain Service 签名 → 标记 blocker

## 可验证性

- 静态检查：
  - `grep -rn "github.com/sashabaranov/go-openai" internal/domains/` 应为空
  - `grep -rn "github.com/anthropics" internal/domains/` 应为空
  - `grep -rn "github.com/tmc/langchaingo" internal/domains/` 应为空
- 单测：Domain Service 测试用 `invoker.Invoker` 的 mock，覆盖工具调用 / 普通对话 / 错误三种分支
- `ddd-layer-checker` 子代理：将 LLM SDK import 视为 infrastructure 依赖
