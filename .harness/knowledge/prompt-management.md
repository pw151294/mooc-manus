# 提示词管理（PromptManager）

> 后端所有系统 / 流程提示词在二进制内烘焙，通过 `PromptManager` 单例统一访问。强约束见 `R-46-prompt`；本文聚焦"机制如何工作 + plans 子包的双角色"。

## 单例机制（`internal/domains/models/prompts/manager.go`）

```go
//go:embed sre/sre_system.md
var sreSystemPrompt string
//go:embed system
var systemPrompt string
//go:embed plans/plan_system
var planSystemPrompt string
//go:embed plans/plan_create.txt
var planCreatePrompt string
//go:embed plans/plan_update
var planUpdatePrompt string
//go:embed react/react_system
var reActSystemPrompt string
//go:embed react/execution
var executionPrompt string
//go:embed react/summarize
var summarizePrompt string
//go:embed a2a/system.txt
var a2aSystemPrompt string
```

- **`//go:embed` 把模板编译进二进制** —— 部署时不需要随包带模板文件，杜绝"运行时被替换"造成的 prompt injection 入口。
- **`PromptManager` 通过 `sync.Once + init()` 初始化**：

  ```go
  var pm *PromptManager
  var once sync.Once

  func init() {
      once.Do(func() {
          pm = &PromptManager{
              systemPrompt:      systemPrompt,
              planSystemPrompt:  planSystemPrompt,
              // ...
          }
      })
  }
  ```

  使用 `init()` 而非显式工厂，让"提示词在程序启动时已就绪"成为编译期事实，调用方无需做空指针保护。
- **所有 getter 内 `pm.Lock()`** —— 虽然字段只读，仍加锁，是为给"运行时热替换"留出扩展空间（如 admin 接口动态注入新模板，无须重新编译）；R-46 §"Plan 模板持久化字段" 也限定可变范围。

## 当前已注册的 9 个 getter

| 函数 | 模板路径 | 调用方 |
|------|---------|--------|
| `GetSystemPrompt` | `system/` 子目录 | `PlanAgent.NewPlanAgent`（注入到 `systemPrompt` 字段，目前未直接使用，预留全局通用 system 入口） |
| `GetPlanSystemPrompt` | `plans/plan_system` | `PlanAgent` 系统提示词 |
| `GetPlanCreatePrompt` | `plans/plan_create.txt` | `PlanAgent.CreatePlan` 模板 |
| `GetPlanUpdatePrompt` | `plans/plan_update` | `PlanAgent.UpdatePlan` 模板 |
| `GetReActSystemPrompt` | `react/react_system` | `NewReActAgent` 系统提示词 |
| `GetExecutionPrompt` | `react/execution` | `ReActAgent.ExecuteStep` 拼装模板 |
| `GetSummarizePrompt` | `react/summarize` | `ReActAgent.Summarize` 调用模板 |
| `GetA2ASystemPrompt` | `a2a/system.txt` | `A2ADomainServiceImpl.A2AChat` 入口 Agent 的系统提示词 |
| `GetSRESystemPrompt` | `sre/sre_system.md` | SRE 业务线系统提示词 |

> 注意：仅 9 个 getter；任何新模板必须先在 `manager.go` 加 `//go:embed` 与对应 `Get*Prompt()`，再加 R-46 §"新增模板的流程"。

## 模板插槽（placeholder）

Agent 拼装提示词时用 `strings.ReplaceAll(template, placeholder, value)`。占位符常量集中在 `internal/domains/services/agents/constants.go`：

```go
const messagePlaceHolder = "{message}"
const attachmentsPlanHolder = "{attachments}"
const planPlaceHolder = "{plans}"
const stepPlaceHolder = "{step}"
const languagePlaceHolder = "{language}"
```

R-46 §"模板插槽 escape" 规定凡是把"外部内容"（MCP 响应 / A2A 返回 / Skill 描述 / 用户输入）注入这些插槽前，必须先经 escape（去控制字符 / 去 `<|im_start|>` 等可疑 token / 限长）。`buildSkillsSystemPrompt`（`agents/agent.go` 末段）是当前唯一对 Skill 元信息做拼接的入口，新增类似场景必须复用该 escape 路径或显式补审计。

## plans 子包（与 PromptManager 平行）

`internal/domains/models/prompts/plans/` 同时承担两个职责，容易混淆：

1. **模板源文件**：`plan_system` / `plan_create.txt` / `plan_update` 三个 embed 目标（被 `manager.go` 引用）。
2. **运行时 PlanManager 单例**（`plans/manager.go`）：进程内保存"用户最近一次生成 / 更新过的 Plan"。

```go
type PlanManager struct {
    sync.Mutex
    id2Plan map[string]agents.Plan
    id2Step map[string]agents.Step
}

func SaveOrUpdate(plan agents.Plan)
func DeletePlanById(id string)
func GetPlanById(id string) (agents.Plan, bool)
func GetStepById(id string) (agents.Step, bool)
```

特点：

- `SaveOrUpdate` 在写入新 Plan 前先删旧 Plan 的 step 反向索引，避免"减少步骤后 id2Step 残留"。
- `DeletePlanById` 同样先清子 step 再删主 Plan。
- 这是个 **内存级缓存**，不写库；进程重启即丢。持久化由 R-46 §"Plan 模板持久化字段约定" 限定走 PO 字段集，但当前代码尚未串通到 Repository，是后续 spec 的事。

## 并发与安全

- `PromptManager` getter 在 `Lock()` 下读 string，O(1)；不会成为热路径瓶颈。
- `PlanManager` 写多读多，同样 `sync.Mutex` 串行化。若后续 plan 流量上来，可改 `sync.RWMutex`，但目前体量没必要。
- R-46 §"Agent 行为" 明确：禁止 `os.ReadFile` 临时读模板、禁止 hard-code 长 prompt、禁止外部内容裸拼。检测路径见同节"可验证性"。

## 模板演进协议

实质改动（例如 `plan_create.txt` 输出格式变更、新增插槽、引入新业务线模板）应同时：

1. 改对应 embed 文件 + getter；
2. 同步更新 Agent 侧 `strings.ReplaceAll` 占位符；
3. 如果改了 Plan 输出 JSON 结构 → 需同步 `agents.ConvertMessage2Plan / ConvertMessage2UpdatedPlan` 解析（`PlanAgent` 内）；
4. 视情况落 ADR（涉及前后端契约或持久化字段时强制）。

## 与父仓 / 跨仓 rule 的交叉

- 与 `mooc-manus-all/.harness/rules/31-untrusted-content.md`（R-31-untrusted）协同：任何来自不可信源的字符串都要 escape。
- 与 `R-43-agent` 协同：PlanAgent 系统提示词替换走的是 `prompts.GetPlanSystemPrompt`，不允许在 `NewPlanAgent` 内 hard-code。
