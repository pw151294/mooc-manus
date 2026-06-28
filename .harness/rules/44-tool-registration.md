---
rule_id: R-44-tool
severity: medium
---

# 工具注册（ToolProvider）

后端工具分三类：**Skill 内置工具**（loadSkill / executeSkill）、**MCP 工具**（接 mcp-go client）、**A2A 工具**（接远端 agent）。所有工具最终都以 `tools.Tool` 形式装配到 Agent 的 `tools []tools.Tool`，注册流程必须经 ToolProvider 抽象，不得在 Agent 内硬编码。

## 禁止行为

1. **禁止在 Agent / Application Service 内硬编码工具名**
   - 禁止：`if toolName == "load_skill" { ... }`、`if name == "mcp_xxx_get" { ... }`
   - 工具识别走 `tools.Tool.Name()` 接口或 `models.ToolFunctionDO.FunctionName`，不靠字符串硬比

2. **禁止跳过 ToolProvider 直接 new 一堆工具塞 Agent**
   - 禁止：`agent.tools = append(agent.tools, &CustomTool{...})` 散落在各处
   - 三类工具统一通过：
     - Skill 类：`tools.SkillTools(...)`（参见 `internal/domains/services/tools/builtin.go`）
     - MCP 类：`ToolProviderDomainService` + `ToolFunctionDomainService` 加载已注册 provider
     - A2A 类：`A2ADomainService` 内部封装

3. **禁止跨类型边界混用**
   - Skill 工具不得通过 MCP provider 路径注册
   - MCP 工具不得通过 SkillTools 工厂构造
   - A2A 工具不得作为 Skill 出现在 LLM `tools` 列表

## 要求行为

1. **三类工具边界**

   | 类型 | 工厂 / 注册入口 | 触发条件 | 生命周期 |
   |------|----------------|---------|---------|
   | Skill 内置 | `tools.SkillTools(skillRepo, versionRepo, storage, executor, skillRefs, messageId)` | `ChatRequest.SkillRefs` 非空 | 单次消息（messageId 绑定） |
   | MCP | `ToolProviderDomainService.LoadByProviderId` + `convertDO2Tool` | Agent 配置引用对应 provider | Provider 生命周期内复用 |
   | A2A | `A2ADomainService.A2AChat` 包装 | Agent 配置指向 a2a server | 每次对话一次连接 |

2. **统一 `tools.Tool` 接口**
   - Tool 需实现 `Name() / Description() / Parameters() / Invoke(ctx, args) (string, error)`
   - 转换为 LLM 协议：`convertDO2Tool` 输出 `llm.Tool`（详见 R-42）

3. **DI 装配位置**
   - 在 `api/routers/route.go::InitRouter` 集中装配 `ToolProviderDomainService` / `ToolFunctionDomainService` / `SkillExecutor` 等依赖
   - Agent 工厂（如 `NewBaseAgent`）入参里接受已组装好的 `[]tools.Tool` 切片，不在 Agent 内重新发现
   - 不引入 Wire / Fx 等 DI 框架（参考 `.harness/AGENTS.md` §2.3）

## Agent 行为

- 用户要"加个新工具" → 先判定属于 Skill / MCP / A2A 哪一类，引导走对应注册流程
- 检测到 Agent 内 `if toolName == "..."` 硬编码 → 拒绝并改走能力声明（Tool.Parameters）
- 看到 `agent.tools = append(...)` 散落多处 → 收拢到统一工厂

## 可验证性

- 单测：`internal/domains/services/tools/tool_test.go` 等覆盖每类工具的注册路径
- 静态：
  - `grep -rn "toolName ==" internal/domains/services/agents/` 应为空
  - `grep -rn "&CustomTool{" internal/domains/services/agents/` 应为空
  - Skill / MCP / A2A 三类工具的 `Init()` 与 `Invoke()` 单测覆盖率 ≥ 80%
- `pre-commit` hook：检测在 Agent 包内新增 `Tool` 类型直接 new 时给出 warning
