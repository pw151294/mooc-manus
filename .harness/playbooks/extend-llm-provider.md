# 接入新 LLM SDK Adapter

把 OpenAI / Anthropic 之外的新厂商 SDK（如 Gemini、Bedrock、Qwen）接进来。架构由 ADR-0001（LLM 协议抽象）锁定：Domain 仅依赖 `llm.Message` / `llm.Tool` 值对象与 `invoker.Invoker` 接口；SDK 调用封装在 `internal/infra/external/llm/`。关联 R-42（LLM 协议）、R-40（DDD 分层）。

## 前置条件

1. 已读 ADR-0001（`specs/INDEX.md` 索引下的 LLM 协议抽象决策）与 R-42 全文
2. 知道目标 SDK 的 Go 包路径（如 `cloud.google.com/go/vertexai/genai`）
3. 厂商 SDK 与既有两种（OpenAI / Anthropic）的差异点（流式 / 工具调用 / 多模态）整理成对照表
4. 阅读现成 `internal/infra/external/llm/openai_adapter.go` 与 `anthropic_adapter.go` 作模板

## 步骤

```bash
cd /path/to/mooc-manus-all/mooc-manus
git switch -c feat/llm-<provider>
```

### 1. 新建 Adapter 文件

`internal/infra/external/llm/<provider>_adapter.go`：

```go
package llm

import (
    "context"
    "mooc-manus/internal/domains/models/invoker"
    "mooc-manus/internal/domains/models/llm"
    // ⚠️ R-42 第 1 条：SDK import 仅允许在 infra 层
    sdk "<vendor sdk path>"
)

type <Provider>Adapter struct { client *sdk.Client; model string }

func New<Provider>Adapter(apiKey, model string) (invoker.Invoker, error) { /* init */ }

func (a *<Provider>Adapter) Invoke(ctx context.Context, req invoker.Request) (invoker.Response, error) {
    sdkReq := toSDK(req.Messages, req.Tools)          // llm.Message → SDK 类型
    resp, err := a.client.Generate(ctx, sdkReq)
    if err != nil { return invoker.Response{}, err }
    return invoker.Response{Message: fromSDK(resp)}, nil  // SDK → llm.Message
}
```

转换函数私有（`toSDK` / `fromSDK`）放同文件；**禁止把 SDK 类型暴露到接口签名**（R-42 第 2 条）。

### 2. 流式（如需要）

`invoker.Invoker` 接口若已声明 `Stream(ctx, req) (<-chan invoker.Chunk, error)`，按既有 openai_adapter 实现。把 SDK 的 token stream 转 `llm.Message` 增量推送。

### 3. 测试（参考 `openai_adapter_test.go`）

`<provider>_adapter_test.go`：

- 用 httptest.Server 或 SDK 提供的 fake client mock 远端
- 测：normal text / tool call / streaming / error mapping
- 关键断言：返回的 `llm.Message` 字段映射正确（Role / Content / ToolCalls.Arguments JSON）

### 4. DI 装配

在 `api/routers/route.go::InitRouter`（或更细分的 DI 函数）按 `appConfig.ModelProvider` 路由：

```go
switch cfg.Provider {
case "openai":    inv = llmadapter.NewOpenAIAdapter(...)
case "anthropic": inv = llmadapter.NewAnthropicAdapter(...)
case "<provider>": inv = llmadapter.New<Provider>Adapter(...)
}
```

如果 `AppConfig` DTO / Entity 未含新 provider 枚举值 → 在 `internal/domains/models/` 与 `applications/dtos/app_config.go` 同步扩枚举，并更新前端类型（R-20）。

### 5. 构建 & commit

```bash
go build ./... && go test ./internal/infra/external/llm/...
go vet ./...
git add -A
git commit -m "feat(llm): 接入 <provider> adapter（实现 invoker.Invoker）"
git push -u origin feat/llm-<provider>
```

## 常见坑

1. **SDK 类型泄漏到 domain**：在 service 入参出现 `genai.Content` → R-42 第 2 条违反；adapter 文件内吃掉所有 SDK 类型。
2. **跳过 Invoker 直接 HTTP**：嫌 SDK 麻烦改 `http.Post` 调 vendor API → R-42 第 3 条违反。
3. **工具 schema 漂移**：每厂商 tool schema 不同（OpenAI `function.parameters` / Anthropic `input_schema`），转换在 `toSDK` 完成，对外保持 `llm.Tool.Parameters`。
4. **错误丢上下文 / 流式断连不 close**：错误包 `%w` 带 request_id；stream channel 在错误路径也要 close，否则上游 SSE 永不发 `done`。
5. **未更新 ADR**：加 adapter 不更新 ADR-0001 的 provider 列表 → 后人无溯源。

## 验证

```bash
go build ./...
go test ./internal/infra/external/llm/... -v
go vet ./...
HARNESS_ROOT=.harness ./.harness/scripts/validate-harness.sh

# 端到端
# 1. AppConfig 切换到新 provider
# 2. 触发 BaseAgent 简单对话，断言事件流正常
# 3. 触发 ReActAgent + tool call，断言 ToolCall.Arguments 是 valid JSON
```

## Agent 行为

- 接到"接新 LLM" → 先检查 ADR-0001 是否需要更新；如果厂商行为与既有架构有偏差（如不支持工具调用），先开新 ADR
- 看到 PR 修改 `internal/domains/` 内 import 加了 SDK 包 → 直接 reject（R-42 第 1 条）
- 看到接口签名出现 SDK 类型 → reject（R-42 第 2 条）
- 看到 adapter 内 `http.Post` 而非 SDK 调用 → 提示考虑用官方 SDK；若坚持 HTTP 直连，确保不在 domain 层（R-42 第 3 条）
- 测试只测 happy path → 提示补 error / stream cancel / tool args malformed 场景
- ⚠️ 注意 R-40：adapter 文件落在 infra 层；domain service 仍只 import `llm` / `invoker`
