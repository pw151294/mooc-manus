# DDD 三层正反例

> 从 mooc-manus 现有代码中提取的真实正例（符合 R-40-ddd / R-42-llm）与反例（违反分层 / SDK 渗透），用于新成员快速校准品味。强约束见 `R-40-ddd`；本文是配套读物。

每个例子都给出：场景 → 现实代码片段 → 为什么对/错 → 修复方向。

---

## 正例 1：SkillProvider 三层闭环

**场景**：导入 git 仓库 / zip 包作为 Skill provider，落库后返回 DTO。

**Application 层**（`internal/applications/services/skill_provider.go`）：

```go
type SkillProviderApplicationServiceImpl struct {
    providerDomainSvc domainSvc.SkillProviderDomainService
}

func (s *SkillProviderApplicationServiceImpl) ImportGit(req dtos.ImportGitRequest) (dtos.SkillProviderInfo, error) {
    do := dtos.ConvertImportGitRequest2DO(req)            // DTO → DO 在 dtos 包
    id, err := s.providerDomainSvc.Create(do)             // 调 Domain Service
    if err != nil {
        return dtos.SkillProviderInfo{}, err
    }
    do.SkillProviderID = id
    return dtos.ConvertSkillProviderDO2Info(do, 0), nil   // DO → DTO 返回
}
```

**为什么对**：

- Application 仅持有 `SkillProviderDomainService` 接口指针，未 `import "internal/infra/repositories"` —— 符合 R-40 §"禁止 interfaces 直接 import infrastructure"。
- DTO ↔ DO 转换函数集中在 `dtos/` 包（命名 `Convert*Request2DO` / `Convert*DO2Info`），符合 R-40 §"三态模型转换链路"。
- 错误透传，未额外裹一层无信息量的 wrapper，与 R-41-go-conventions 一致。

**仿写要点**：新增 application service 时，构造函数只接收 domain service 接口；DTO → DO 在 `dtos/<domain>.go` 内集中放置；Repository 永远在 domain service 内部。

---

## 正例 2：LLM 协议抽象在 BaseAgent 的落地

**场景**：BaseAgent 调用 LLM 并执行工具调用，全程使用厂商无关的 `llm.Message` / `llm.Tool`。

**Domain 层**（`internal/domains/services/agents/base.go`）：

```go
import (
    "mooc-manus/internal/domains/models/events"
    "mooc-manus/internal/domains/models/invoker"   // ← 只 import domain 模型
    "mooc-manus/internal/domains/models/llm"
    "mooc-manus/internal/domains/models/memory"
    "mooc-manus/internal/domains/services/tools"
)

type BaseAgent struct {
    invoker  invoker.Invoker     // 接口而非 SDK 实例
    memory   *memory.ChatMemory  // 值对象级存储
    tools    []tools.Tool
    // ...
}

func (a *BaseAgent) InvokeLLM(messages []llm.Message) (llm.Message, error) {
    a.AddToMemory(messages)
    availableTools := a.GetAvailableTools()
    return a.invoker.Invoke(a.GetMessages(), availableTools)  // 经接口调
}
```

**为什么对**：

- `import` 列表里完全没有 `github.com/sashabaranov/go-openai` / `github.com/anthropics/...` —— 符合 R-42 §"禁止 domains 直接 import LLM 厂商 SDK"。
- 通过 `invoker.Invoker` 接口调，不直连 HTTP / SDK；adapter 的具体实例由 `PickInvoker` 在 DI 阶段决定（`agent.go::PickInvoker`，详见 `llm-protocol-abstraction.md`）。
- `[]llm.Message` 和 `[]llm.Tool` 都是值对象，进入 adapter 才会被转成 SDK 类型；adapter 是 R-42 §"Adapter 位于 infrastructure 层" 明确允许的例外。

**仿写要点**：domain service 内 LLM 相关字段一律用 `invoker.Invoker` + `llm.*` 值对象；新增字段从 `Extra map[string]any` 开始落地，必要时再升格为强类型（参考 ADR-0001 §"中性 / 待观察"）。

---

## 反例：Agent 内硬编码工具名 + 直 new 工具

**场景**（构造的反面教材，来自 R-43 / R-44 警示集合）：开发者想"快速加个新工具"，跳过 ToolProvider 路径直接在 Agent 内塞工具，并在调用处用字符串比较选分支。

```go
// BAD: 设想中的违例代码，不存在于当前仓但是常见诱惑
func (a *BaseAgent) extendWithSqlTool() {
    a.tools = append(a.tools, &CustomTool{ // ← 在 Agent 内直接 new Tool，违反 R-44
        name: "exec_sql",
    })
}

func (a *BaseAgent) routeToolCall(name string, args string) string {
    if name == "exec_sql" {                // ← 字符串硬比，违反 R-44 §"禁止硬编码"
        return runSQL(args)
    }
    if name == "load_skill" {
        return loadSkill(args)
    }
    return ""
}
```

**为什么错**：

- **R-44 §"禁止在 Agent / Application Service 内硬编码工具名"** 明文禁止 `if toolName == "..."`。
- **R-44 §"禁止跳过 ToolProvider 直接 new 一堆工具塞 Agent"** 明文禁止 `agent.tools = append(...)`。
- **R-40 §"禁止跨层调用"** 也被穿透：业务分支沉到了 domain 内部，迭代时容易把 Repository / HTTP client 也 drag 进 Agent。
- **R-42 §"禁止跳过 Invoker 直接调用 HTTP"** 类似地禁止 `http.Post(...)` 直连，扩展到此处就是"不要让 Agent 内突然冒出执行细节"。

**修复方向**：

1. 把"新工具"建模为 `ToolFunctionDO`（domain model）+ `ToolProviderDO` 入库。
2. `tools.InitTools(providers, proId2Funcs, srvCfgs)` 按 `ProviderType` 路由到 `NewMcpTool` / `NewCustomTool` / `NewA2ATool`（`tools/base.go`），自动接入 Agent。
3. 工具执行细节走 `tools.Tool.Invoke(funcName, args)` 多态调用 —— 完全不需要在 Agent 内写 `if name == ...`。
4. LLM 看到的能力清单由 `BaseAgent.GetAvailableTools()` 聚合 `[]llm.Tool` 自动生成。

可参考的现有正例：`internal/domains/services/tools/mcp.go` / `custom.go` / `a2a.go` 三类工具的统一接口实现。

---

## 反例（情境化）：日志泄漏 conversationId 原文

**场景**（来自 R-47-memory 警示）：写日志时图方便直接打了 conversationId。

```go
// BAD
logger.Info("chat begin", zap.String("conversationId", req.ConversationId))
```

**为什么错**：

- R-47 §"禁止 conversationId 出现在低信任面" 明文禁止 `zap.String("conversationId", id)` 直打。
- 父仓 R-32-secrets（`mooc-manus-all/.harness/rules/32-secrets-handling.md`）把 conversationId 列入 secret 级别字段。

**修复方向**：

```go
// GOOD（待落地的统一 mask helper）
logger.Info("chat begin", zap.String("conversationId", maskID(req.ConversationId)))
```

其中 `maskID` 保留前 8 位 + `...`。检索剩余泄漏点：

```
grep -rn "zap.String(\"conversationId\"," .
```

---

## 何时该把例子升格为新 rule

- 同一类反例在 PR review 出现 ≥ 3 次 → 升格为新 rule（参见 `mooc-manus-all/.harness/rules/00-rule-authoring-meta.md`）。
- 某一类正例被反复借鉴 / 复制 → 在 `ddd-examples.md` 增条目，避免每个新人都要重读源码才找到模板。

## 与 archive 的对应

本文取代了 `.harness/archive/AGENTS-pre-harness-v1.md` §2.1 + §3.1 ~ §3.4 中关于"分层职责"的部分散落示例。原文以"哪一层做什么"为主线，本文聚焦"在真实代码里长什么样 + 反面如何识别"。
