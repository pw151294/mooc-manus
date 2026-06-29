# Playbooks 索引（mooc-manus 后端）

可重放的后端实现剧本。所有涉及"加 Agent / 加工具步骤 / 接 MCP / 换 LLM SDK / 换 Repository 实现"的动作走这里。规则正文见 `rules/`，本目录仅以 R-XX 短码引用。

## 何时进这里

- 需要第 5 种 Agent（如 SwarmAgent / WorkflowAgent）→ `add-new-agent-type.md`
- 现有 ReActAgent 流程要加一步（如 reflection / critique）→ `add-react-agent-step.md`
- 接一个新的 MCP server 暴露给 Agent 用 → `integrate-new-mcp-server.md`
- 接入新 LLM 厂商（如 Gemini / Bedrock）→ `extend-llm-provider.md`
- 把某个 GORM Repository 换成 Mongo / Redis → `migrate-repository-impl.md`

## 后端剧本清单

| 剧本 | 适用 | 关联 rule |
|---|---|---|
| `add-new-agent-type.md` | 新增 Agent 实现（第 5 种） | R-43 / R-45 / R-47 |
| `add-react-agent-step.md` | 在 ReActAgent 内加新工具步骤 | R-43 / R-44 / R-45 |
| `integrate-new-mcp-server.md` | 注册新 MCP server / 工具 | R-44 / R-48 |
| `extend-llm-provider.md` | 接入新 LLM SDK adapter | R-42（+ ADR-0001） |
| `migrate-repository-impl.md` | 替换 Repository 实现 | R-40 |

## 通用规约

- 每份剧本结构：**前置条件 / 步骤 / 常见坑 / 验证 / Agent 行为**
- 所有代码示例引用真实路径（如 `internal/domains/services/agents/react.go`），未存在的路径会被 review 退回
- 子仓内 commit message 走 `feat(<scope>): ...` / `fix(<scope>): ...` 约定
- 子仓 commit & push 后由总仓单独升级指针（见总仓 `playbooks/upgrade-submodule.md`）

## 与其他目录

- 总仓跨仓剧本见 `mooc-manus-all/.harness/playbooks/`
- 详细架构背景见 `knowledge/`（`agent-internals.md` / `tool-invocation-flow.md` / `event-driven-model.md` / `llm-protocol-abstraction.md`）
- DDD 分层约束（R-40）是所有剧本的隐式前提；进入应用层 / 领域层 / 基础设施层时分别注意
- LLM 协议抽象的设计决策见 ADR-0001（`specs/INDEX.md`）
