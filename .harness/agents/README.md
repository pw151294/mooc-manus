# 后端 Agents 索引

本目录定义 `mooc-manus`（后端）层级的 harness subagent。Agent 是带固定 frontmatter + 检查清单 + 检查 prompt 的 markdown 文件，供 CI / pre-commit / 人工 review 时按需调用。

## 设计原则

- **一个 agent 聚焦一条 rule**：保证职责单一；多条 rule 的复合校验由 workflow 编排。
- **路径绝对化 + 与实际目录一致**：所有检查 prompt 中提到的代码路径以仓库根目录为参考点，并使用真实目录名（`internal/applications/` / `internal/infra/`，而非历史文档中可能出现的 `interfaces/` / `infrastructure/`）。
- **可机器执行 + 可人工复核**：检查清单是给 reviewer 的 mental model，检查 prompt 是直接可投喂 subagent 的工作指令。

## 可用 agents

| Agent | 关联 Rule | 适用场景 |
|---|---|---|
| `ddd-layer-checker` | R-40-ddd | 后端 Handler / Application / Domain / Infra 任一层文件变更 |
| `llm-protocol-checker` | R-42-llm | `internal/domains/` 涉及 LLM 调用变更；新增/修改 `internal/infra/external/llm/*_adapter.go` |
| `prompt-template-reviewer` | R-46-prompt | `internal/domains/models/prompts/` 下模板变更；PromptManager 调用点新增 |

## 调用方式（v1.0）

当前 v1.0 不在 manifest.yaml 的 `execution.agents` 字段中登记 agents（延后到 v1.1）。调用方式：
- **CI**：由 `.harness/scripts/validate-harness.sh` 在文件触发条件命中后执行检查 prompt（接入见 Phase 10 / Phase 12）。
- **人工 / IDE**：把对应 agent 的"检查 Prompt"段落复制到任意 subagent 会话，附 diff + import 段即可执行。

## v1.1 规划

- 在 manifest.yaml `execution.agents` 中正式登记 agents 与触发条件
- 新增 `tool-registration-checker`（R-44） / `memory-boundary-checker`（R-47） / `skill-executor-checker`（R-48）
- 与 `.harness/workflows/` 联动，由 spec→plan→implement 流程自动 dispatch

## 添加新 agent 的步骤

1. 在本目录创建 `<name>.md`，含三段：frontmatter / 检查清单 / 检查 prompt。
2. frontmatter 必备字段：`name / description / when_to_use / inputs / outputs`。
3. 检查清单引用至少一条 `rule_id`（保证可追溯）。
4. 更新本 README 的"可用 agents"表格。
5. 单独 commit：`feat(harness): agents/<name>.md`。
