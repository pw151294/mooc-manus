---
rule_id: R-46-prompt
severity: critical
---

# 提示词管理（PromptManager）

后端所有 system prompt / plan / react / a2a / sre 模板由 `internal/domains/models/prompts/` 下的 `PromptManager` 全局单例统一加载（参考 `manager.go`），通过 `//go:embed` 把模板文件烘焙进二进制。本规则锁定模板加载方式、外部插槽 escape 与 Plan 模板持久化字段，配合 R-31-untrusted（详见 mooc-manus-all/.harness/rules/31-untrusted-content.md）形成 prompt injection 防线。

## 禁止行为

1. **禁止绕过 PromptManager 加载提示词**
   - 禁止 Domain Service 内 `os.ReadFile("prompts/xxx.md")` 临时读取
   - 禁止从数据库 / HTTP 拉取后直接当系统提示词拼接
   - 禁止 hard-code 长 prompt 字符串散落在 `agents/*.go`

2. **禁止把外部内容裸拼进系统提示词模板**
   - 任何来自 R-31 列举的不可信源（MCP 响应 / A2A 返回 / 用户上传 skill 元信息 / 历史对话 / HTTP body）在 inject 模板前必须经 escape
   - 禁止：`fmt.Sprintf(systemPrompt, userInput)` 直接拼用户输入
   - 禁止：把 Skill 描述、Tool 描述等 LLM 可见字段直传给模板而不做转义

3. **禁止持久化未受控的 Plan 字段**
   - Plan 模板与生成的 plan 结构（参考 `internal/domains/models/agents/plan.go` 与 `prompts/plans/`）字段集合固定；不得在数据库写入未声明字段
   - 不持久化用户可控、可被未来用作 prompt 注入的自由文本字段（如 `rawUserHint`）入 plan 表

## 要求行为

1. **全局单例模式**
   - `PromptManager` 走 `sync.Once` 单例（见 `manager.go::init`）
   - 所有提示词来源走 `Get*Prompt()` 接口（`GetSystemPrompt` / `GetPlanSystemPrompt` / `GetPlanCreatePrompt` / `GetPlanUpdatePrompt` / `GetReActSystemPrompt` / `GetExecutionPrompt` / `GetSummarizePrompt` / `GetA2ASystemPrompt` / `GetSreSystemPrompt`）

2. **模板插槽 escape（与 R-31 配合）**
   - 模板内插槽统一占位符（如 `${query}` / `${attachments}` / `${skills_section}`）
   - inject 前对外部内容做：
     - 去除控制字符 / NUL / 终端转义
     - 去除"ignore previous instructions"等可疑 token（记录到总仓 `.harness/retro/ai-error-log.md` 但不阻断）
     - 限长（避免上下文溢出 + 简单的 injection 规避）
   - 参考 `docs/skill-system-prompt-injection-implementation.md` §3 `buildSkillsSystemPrompt` 的实现

3. **Plan 模板持久化字段约定**
   - Plan 表 / DO 必填：`PlanID` / `Title` / `Steps[]`（每 Step 含 `StepID` / `Description` / `Status`）/ `CreatedAt` / `UpdatedAt`
   - 字段命名与命名规范一致（R-41-go）；JSON camelCase / DB 蛇形小写
   - 不持久化模板正文本身（模板走 `//go:embed`）

4. **新增模板的流程**
   - 在 `internal/domains/models/prompts/<sub>/` 下放模板文件 → `manager.go` 添加 `//go:embed` 变量 → 添加 `Get*Prompt()` 访问器 → 在 Agent 内通过访问器读取

## Agent 行为

- 检测 `os.ReadFile` / `embed.FS` 读 prompt 类文件却不经 PromptManager → 拒绝
- 检测 `fmt.Sprintf(prompt, externalContent)` / `strings.Replace(prompt, placeholder, externalRaw)` 缺少 escape → 标记 critical
- 新增外部数据源（MCP server / 数据库新表 / 新文件上传）→ 强制 dispatch `prompt-template-reviewer`（见 R-31 §Agent 行为）

## 可验证性

- 静态：
  - `grep -rn "os.ReadFile" internal/domains/services/agents/` 仅允许出现在文件下载 / SkillExecutor 路径
  - `grep -rn "fmt.Sprintf" internal/domains/services/agents/` 人工审查每处是否含外部内容直拼
  - `prompts/` 目录新增模板必须同步 manager.go 的 `//go:embed`（`pre-commit` hook 校验）
- 单测：
  - 构造含 `<|im_start|>` / `ignore previous instructions` / NUL 的输入，断言 escape 后不含原 token
  - PromptManager 并发读访问安全（`sync.Mutex` 已在 `manager.go` 提供）
- `prompt-template-reviewer` 子代理：扫描 PR 是否引入新外部插槽且缺 escape
