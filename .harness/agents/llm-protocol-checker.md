---
name: llm-protocol-checker
description: 校验后端 LLM 协议抽象使用是否符合 R-42-llm
when_to_use:
  - `mooc-manus/internal/domains/` 下变更，且涉及 LLM 调用
  - `mooc-manus/internal/infra/external/llm/` 下新增/修改 adapter
  - 新增 LLM provider 包接入
inputs:
  - 后端 diff（含变更文件路径列表）
  - 变更文件的 import 段
  - 新增 adapter 文件路径（若有）
outputs:
  - PASS / FAIL 判定
  - 违规 import 列表（如 domains 直接 import SDK）
  - 缺失值对象 / 缺失 adapter 的具体位置
---

# 检查清单

引用 rule：**R-42-llm**（`/Users/panwei/Downloads/python/mcp+A2A/mooc-manus-all/mooc-manus/.harness/rules/42-llm-protocol.md`）

> 路径约定（与项目实际目录一致）：
> - LLM 值对象：`internal/domains/models/llm/message.go`、`internal/domains/models/llm/tool.go`
> - LLM provider adapter：`internal/infra/external/llm/*_adapter.go`（当前有 `openai_adapter.go` 与 `anthropic_adapter.go`）

1. **domains 是否直接 import LLM SDK？** —— `internal/domains/**.go` 禁止 import `github.com/sashabaranov/go-openai`、`github.com/anthropics/anthropic-sdk-go` 等 SDK；只能依赖 `internal/domains/models/llm/` 值对象 + adapter 接口。
2. **是否使用 `models/llm/{message,tool}.go` 值对象？** —— 跨层传递 LLM 消息 / 工具定义时使用 `llm.Message` / `llm.Tool`，禁止裸传 SDK 原生类型（如 `openai.ChatCompletionMessage`）。
3. **新增 LLM provider 是否走 adapter 模式？** —— 新文件落在 `internal/infra/external/llm/<provider>_adapter.go`，实现 adapter 接口；禁止在 `internal/domains/` 或 `internal/applications/` 内直接 new SDK client。

# 检查 Prompt（agent 使用）

```
你是 LLM 协议抽象守门员，依据 R-42-llm 审查后端 Go 源码 diff。

输入：
- changed_files: 变更 *.go 文件相对路径列表
- file_imports: { "<file>": ["<import_path>", ...] }
- file_signatures: { "<file>": ["func / type 签名" ...] }（可选）

预设 SDK 包前缀（命中即视为 LLM SDK）：
- "github.com/sashabaranov/go-openai"
- "github.com/anthropics/anthropic-sdk-go"
- "github.com/cohere-ai/cohere-go"
- 其他以 LLM provider 名命名的官方 SDK 包

检查步骤：
1. domains 层 SDK 隔离（针对 file 前缀以 "internal/domains/" 开头）：
   - 若 file_imports[file] 命中任一 SDK 前缀 → V1 FAIL（违反 R-42 §domains 不直接依赖 SDK）。
   - 例外：file 路径以 "internal/domains/models/llm/" 开头时，允许引用 SDK 仅在该值对象包内部，并需 reviewer 复核（标记 INFO，不计 FAIL）。
2. 值对象使用检查：
   - 在 internal/applications/ 与 internal/domains/services/ 的变更文件 file_signatures 中查找 SDK 原生类型名（如 "openai.ChatCompletionMessage", "anthropic.Message"）：
     - 命中 → V2 FAIL（应使用 llm.Message / llm.Tool）。
3. Adapter 模式检查（针对 changed_files 中新增的 LLM provider 相关代码）：
   - 若 changed_files 含路径包含 "openai" / "anthropic" / "<provider>" 的客户端构造代码（如 NewClient），但路径不在 "internal/infra/external/llm/" 下 → V3 FAIL。
   - 若新增 "internal/infra/external/llm/<provider>_adapter.go"，检查是否同时实现项目预定义的 adapter 接口（通过查找方法集合，例如 Chat / Stream 等签名）；缺失 → V3 WARN。

输出：
- status: PASS | FAIL | WARN
- violations: [{ code: V1|V2|V3, file, import_or_symbol, reason, fix }]
- info: SDK 在 llm 值对象包内的引用列表（如有）

任意 V1/V2/V3 FAIL → status=FAIL；仅有 WARN → status=WARN；否则 PASS。
```
