# 智能体自动化评测机制 — 设计规格

- 文档编号: `2026-07-16-agent-evaluation-design`
- 归档路径: `docs/superpowers/specs/2026-07-16-agent-evaluation-design.md`
- 作者: 项目团队
- 状态: DRAFT (2026-07-16)

---

## 0. 摘要

为量化 mooc-manus 编程智能体的任务完成准确率、稳定性、资源消耗与执行效率，本文档设计并规格化一套**标准化、可持久化、可批量执行、可观测**的自动化评测体系。评测范围聚焦智能体核心工具能力：**文件读写、文件编辑、bash 脚本执行**（NATIVE 三件套）。

本方案以下述硬约束落地：

- **零新中间件**：队列复用现有 Redis（Asynq），DB 复用现有 PostgreSQL，追踪复用现有 `ai_span`。
- **严格 DDD 四层**：Handler → Application → Domain → Repository，全部走 `route.go:InitRouter` 统一初始化。
- **禁止直接批量开 goroutine**：所有并发经 Asynq 队列消费。
- **沙盒复用**：复用 NATIVE workspace `${WorkspaceBaseDir}/${conversationId}/${messageId}`。

---

## 1. 顶层架构

```
                       ┌───────────────────────────┐
                       │  Frontend (mooc-manus-web) │
                       └────────────┬──────────────┘
                                    │  REST
                       ┌────────────▼──────────────┐
                       │ api/handlers/eval.go      │
                       └────────────┬──────────────┘
                                    │
                       ┌────────────▼──────────────┐
                       │ EvaluationApplicationSvc  │
                       └────────────┬──────────────┘
             enqueue                │                     query
      ┌─────────────────┐           ▼           ┌──────────────────┐
      │ Asynq  Enqueuer │◀───┬─EvaluationDomain─┤ Repository 层    │
      └────────┬────────┘    │                  └────────┬─────────┘
               │             ▼                           │
     ┌─────────▼────────┐   (复用) BaseAgentDomainSvc    ▼
     │ Asynq Server     │──────────┬─────────────  PostgreSQL
     │  worker pool     │          │           (5 表 eval_* + ai_span)
     │  (topic: eval)   │          ▼
     └─────────┬────────┘   InternalChatRunner ── 复用 NATIVE workspace
               │            (内部 eventCh close 阻塞等待)
               ▼
     ┌──────────────────┐  执行验证脚本 (独立进程/独立超时)
     │ InstanceExecutor │  ──▶ 聚合 ai_span (root latency + Σ token)
     │                  │  ──▶ 落 eval_result + 推进双状态机
     └──────────────────┘
               ▲
     ┌─────────┴──────────┐
     │ InspectorScheduler │  每 30s / 60s / 5min 三种巡检
     └────────────────────┘
```

### 1.1 主流程复用点（源码定位）

- `AgentHandler.Chat` — `api/handlers/agent.go:24`
- `BaseAgentApplicationServiceImpl.Chat` — `internal/applications/services/agent.go:111`
- `BaseAgentDomainService.Chat` — `internal/domains/services/agents/agent.go:65`
- `BaseAgent.StreamingInvokeLLM` — `internal/domains/services/agents/base.go:536`
- `startLLMCallSpan` — `internal/domains/services/agents/base.go:475`（需要补 token tag）
- `AiSpanPO` — `internal/infra/models/ai_span.go`
- NATIVE workspace 隔离约定 — `${native.workspace_base_dir}/${conversationId}/${messageId}`
- `CleanupMessage` — `internal/applications/services/agent.go:62-82`

### 1.2 关键复用与不复用

| 事项 | 决策 |
|---|---|
| HTTP 层 | **不复用** `/api/agent/chat`。评测走内部 domain 调用，避免 HTTP 往返与 SSE 开销 |
| eventCh close 语义 | **复用**。评测层新开一个内部 eventCh，交给 domain 层，close 即视为智能体收敛 |
| Sandbox workspace | **完全复用** NATIVE 三件套的目录隔离 (§4.4) |
| Trace `ai_span` | **完全复用**。仅补齐 LLM_CALL span 的 token tag (§4.1) |
| CleanupMessage | **复用**。评测流程结束（含验证脚本）后走同样清理入口 |

---

## 2. 数据层设计（5 张新表 + 复用 ai_span）

### 2.1 `eval_case`（评测用例）

物理删除。删除前置检查：若存在 `eval_run_instance` 引用本 case 且引用它的 task 未进入终态，返回 409。task 进入终态之后允许物理删除；已终态实例的 `eval_run_instance.case_snapshot`（jsonb）里冻结了用例三要素，删除 case 不影响历史结果可回溯。

**用例三要素的职责边界**（B1 说明）：

- `init_script`：负责在智能体工作目录 (`${WorkspaceBaseDir}/${conversationId}/${messageId}`) 内完成**文件系统初始化**（写入待编辑的源文件、准备 fixture、下载数据等）。文件读写/编辑类用例的输入文件全部由 init_script 落地，无需额外的 `input_context` 字段。
- `task_prompt`：给到智能体的评测指令。
- `verify_script`：验证智能体输出，`exit_code=0` 通过。可访问 init_script 建立的文件与智能体产出文件。

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | `uuid pk` | |
| `name` | `varchar(128)` | UNIQUE |
| `description` | `text` | 可选 |
| `init_script` | `text` | nullable |
| `task_prompt` | `text` | NOT NULL |
| `verify_script` | `text` | NOT NULL；exit_code=0 通过 |
| `tags` | `jsonb` | `["file-io","bash","edit"]` 便于筛选 |
| `created_at / updated_at` | `timestamp` | |

索引：`UNIQUE(name)`、`GIN(tags)`（供标签过滤）。

### 2.2 `eval_task`（父任务）

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | `uuid pk` | |
| `name` | `varchar(128)` | |
| `status` | `varchar(24)` | 见 §3.1 |
| `total_count` | `int` | M×N |
| `succeeded_count` | `int` | passed=true 汇总 |
| `failed_count` | `int` | FAILED/TIMEOUT/passed=false 汇总 |
| `running_count` | `int` | 非终态汇总 |
| `case_ids` | `jsonb` | 选中的 M 个 case_id |
| `agent_config_snapshot_ids` | `jsonb` | N 个 snapshot_id |
| `created_by` | `varchar(64)` | 预留 |
| `created_at / started_at / finished_at` | `timestamp` | |

索引：`(status, created_at DESC)` 支持列表分页 + 状态筛选。

### 2.3 `eval_run_instance`（M×N 运行实例）

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | `uuid pk` | |
| `task_id` | `uuid` | fk (ON DELETE CASCADE) |
| `case_id` | `uuid` | 逻辑关联，**不设 FK 约束**（case 物理删） |
| `case_snapshot` | `jsonb` | 冻结用例三要素 + name + tags |
| `agent_config_snapshot_id` | `uuid` | fk |
| `status` | `varchar(24)` | 见 §3.2 |
| `attempt` | `int` | 从 1 递增，重试自增 |
| `conversation_id` | `varchar(64)` | 每次 attempt 重置为新 uuid |
| `message_id` | `varchar(64)` | 每次 attempt 重置 |
| `trace_id` | `varchar(64)` | 首个 AGENT_ROOT span 的 trace_id |
| `queued_at / started_at / finished_at / heartbeat_at` | `timestamp` | |
| `deadline_at` | `timestamp` | 每次 attempt 起跑时 = `started_at + instance_total_timeout`；供巡检器直接比较，不依赖 attempt 期间的配置变更 |
| `worker_id` | `varchar(64)` | 处理该实例的 worker 标识 |
| `error_message` | `text` | 排队/初始化阶段的失败摘要，超过 4KB 截断 + `\n[truncated]` |

约束与索引：
- `UNIQUE (task_id, case_id, agent_config_snapshot_id)` — 天然幂等
- `INDEX (task_id, status)`
- `INDEX (status, heartbeat_at)` — 巡检查询 `status IN (INITIALIZING,RUNNING,VERIFYING) AND heartbeat_at < ?`（B-N2 复合索引）
- `INDEX (status, queued_at)` — 供 Enqueue 兜底
- `FK task_id → eval_task(id) ON DELETE CASCADE`
- `FK agent_config_snapshot_id → eval_agent_snapshot(id) ON DELETE RESTRICT`（snapshot 由 task 删除时一并清理）

### 2.4 `eval_result`（1:1 to instance）

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | `uuid pk` | |
| `instance_id` | `uuid` | fk UNIQUE (ON DELETE CASCADE) |
| `passed` | `bool` | |
| `verify_exit_code` | `int` | |
| `verify_stdout` | `text` | 截断 64KB |
| `verify_stderr` | `text` | 截断 64KB |
| `error_log` | `text` | 智能体侧错误 + 初始化脚本错误 |
| `prompt_tokens` | `bigint` | 见 §4 |
| `completion_tokens` | `bigint` | |
| `total_tokens` | `bigint` | |
| `agent_latency_ms` | `bigint` | AGENT_ROOT.latency_ms |
| `finished_at` | `timestamp` | |

### 2.4.1 结果字段截断与外键约束（B-Blk11 明确）

- `verify_stdout / verify_stderr / error_log`：单字段上限 64KB，超过时截断并追加 `\n[truncated]`
- `FK instance_id → eval_run_instance(id) ON DELETE CASCADE`

### 2.5 `eval_agent_snapshot`（配置快照 —— 唯一额外新增表）

冻结 `appConfig` 关键字段，保证智能体配置多天后变更不影响历史评测复盘。

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | `uuid pk` | |
| `source_app_config_id` | `varchar(64)` | 反向溯源到活的 appConfig |
| `name` | `varchar(128)` | 快照瞬间的 appConfig 名 |
| `model` | `varchar(64)` | e.g. `gpt-4o` |
| `system_prompt` | `text` | |
| `tools_config` | `jsonb` | 冻结的 tools 定义 |
| `mcp_config` | `jsonb` | 冻结的 mcp 定义 |
| `a2a_config` | `jsonb` | 冻结的 a2a 定义 |
| `created_at` | `timestamp` | |

**唯一新增表的动机**：`appConfig` 是活对象，用户会修改。评测报告若在几天后被复查，若无快照则模型/prompt/工具集全部漂移，评测结果失去可比性。

### 2.6 表间外键 & 级联删除顺序（B-Blk3 明确）

```
eval_task (id) ──┐
                 ├── eval_run_instance (task_id) ON DELETE CASCADE
                 │       └── eval_result (instance_id) ON DELETE CASCADE
                 └── (无 FK) case_ids / agent_config_snapshot_ids 冗余在 task 的 jsonb 里

eval_case (id) ── (无 FK, case 物理删) ← eval_run_instance.case_id 逻辑关联，靠 case_snapshot 保历史
eval_agent_snapshot (id) ── eval_run_instance.agent_config_snapshot_id  ON DELETE RESTRICT
```

**删除路径**：

- 删单 case (`DELETE /api/eval/cases/{id}`)：前置查 `eval_run_instance WHERE case_id=? AND task.status NOT IN ('SUCCEEDED','PARTIAL_FAILED')`，非空则 409。通过检查后直接物理删 case。
- 删单实例：DB 级 CASCADE 自动删 `eval_result`；handler 内**同事务** 重算并 UPDATE task 的 4 个 count；同事务内 CAS 推进 task 状态（可能触发 RUNNING→SUCCEEDED/PARTIAL_FAILED）。
- 删单任务：DB 级 CASCADE 自动删 instance + result；handler 内**同事务** DELETE 关联 `eval_agent_snapshot`（agent_snapshot 与 task 1:N 关系，snapshot 不复用）。

---

## 3. 双状态机设计（重点难点 1）

### 3.1 `eval_task` 状态枚举（4 个）

| 状态 | 语义 |
|---|---|
| `PENDING` | 刚创建，子实例尚未全部入队 |
| `RUNNING` | ≥1 条实例开始执行且未全部终态 |
| `SUCCEEDED` | 所有实例终态且全部 `passed=true` |
| `PARTIAL_FAILED` | 所有实例终态、存在 `passed=false` 或 `FAILED/TIMEOUT` |

**流转**

```
PENDING ──enqueue done──▶ RUNNING
PENDING ──all terminal (edge case)──▶ SUCCEEDED / PARTIAL_FAILED
RUNNING ──all passed──▶ SUCCEEDED
RUNNING ──all terminal & any failed──▶ PARTIAL_FAILED
SUCCEEDED / PARTIAL_FAILED ──retry failed──▶ RUNNING
```

**推进 SQL**（在实例每次进入终态后触发；单一 SELECT + 单一 UPDATE，同事务）

```sql
SELECT
  COUNT(*) FILTER (WHERE status IN ('PASSED','FAILED','TIMEOUT','CANCELED')) AS terminal,
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE status='PASSED') AS passed
FROM eval_run_instance
WHERE task_id = ?;
```

- `terminal < total` → `RUNNING`
- `terminal = total AND passed = total` → `SUCCEEDED`
- `terminal = total AND passed < total` → `PARTIAL_FAILED`

`succeeded_count / failed_count / running_count` 冗余列**同事务原子更新**，避免列表页统计走全表扫。

### 3.2 `eval_run_instance` 状态枚举（8 个）

| 状态 | 语义 | 是否终态 |
|---|---|---|
| `PENDING` | 刚生成、未入队；或重试后重置 | 否 |
| `QUEUED` | 已入 Asynq queue | 否 |
| `INITIALIZING` | worker 已取任务，正在跑 init_script + agent 准备 | 否 |
| `RUNNING` | 智能体正在 StreamingInvoke | 否 |
| `VERIFYING` | 智能体收敛，正在跑 verify_script | 否 |
| `PASSED` | verify exit_code=0 | 是 |
| `FAILED` | verify != 0 / agent 报错 / init 失败 | 是 |
| `TIMEOUT` | 总耗时超 `instance_total_timeout` | 是 |
| `CANCELED` | 预留位（第一版不主动写入） | 是 |

**流转白名单**

```
PENDING ─enqueue ok─▶ QUEUED ─worker take─▶ INITIALIZING
INITIALIZING ─agent start─▶ RUNNING
INITIALIZING ─init_script fail─▶ FAILED
RUNNING ─agent done─▶ VERIFYING
RUNNING ─agent error─▶ FAILED
RUNNING ─timeout─▶ TIMEOUT
VERIFYING ─exit=0─▶ PASSED
VERIFYING ─exit≠0─▶ FAILED
VERIFYING ─verify timeout─▶ FAILED
(INITIALIZING | RUNNING | VERIFYING) ─inspector heartbeat stale─▶ TIMEOUT
(FAILED | TIMEOUT) ─retry api─▶ PENDING (attempt+1, new conv/msg id)
```

**实现载体**：`internal/domains/services/evaluation/state_machine.go` 导出两个白名单函数：

```go
func TransitTask(from, to TaskStatus) error       // 非法组合返回 error
func TransitInstance(from, to InstanceStatus) error
```

所有 repo 层写入统一走这两个函数守门；任何 `status = "PASSED"` 裸赋值一律 code review 打回。

**推进方式**：全程 CAS —— `UPDATE eval_run_instance SET status=? WHERE id=? AND status=?`；返回 `rows_affected=0` 视为竞态失败，worker 记录 warn log 并放弃当前流转（此时说明有并发方已推进，通常是巡检器）。

### 3.3 `FAILED` 与 `TIMEOUT` 状态语义边界（B-Blk4 澄清）

Reviewer 建议增加一个通用 FAILED —— 已有。这里明确 FAILED / TIMEOUT 判定口径：

- `FAILED`：可归因失败 —— init_script 非 0 退出、智能体主动报 error 事件、verify_script 非 0 退出、worker panic 被 recover。**passed=false 且验证脚本能执行完毕的用例走 FAILED，不走 TIMEOUT**。
- `TIMEOUT`：仅两种触发路径 —— 巡检器发现 `deadline_at < now()` 强推、Asynq 20min 硬超时兜底。
- 具体分类落 `eval_result.error_log`；上游筛选按 status 即可，不需要另加 sub-status。

---

## 4. 指标采集补强 —— LLM token 落地 + 聚合算法

### 4.1 现状与缺口

- `base.go:475 startLLMCallSpan` 只写 `llm.messages_count / llm.tools_count / llm.tool_calls_count`，**没有 token tag**。
- `OpenAiLLM.StreamingInvoke`（`openai.go:58`）内部 `openai.ChatCompletionAccumulator.Usage` 已带 PromptTokens/CompletionTokens/TotalTokens，但未回传到 domain 层。
- `AnthropicAdapter.StreamingInvoke` 同理。

### 4.2 补强方案（最小侵入 —— 改 3 处）

**① `internal/domains/models/llm/usage.go`（新增）**

```go
type Usage struct {
    PromptTokens     int64
    CompletionTokens int64
    TotalTokens      int64
}
```

**② `internal/domains/models/invoker/invoker.go`（扩容签名）**

```go
type Invoker interface {
    Invoke(messages []llm.Message, tools []llm.Tool) (llm.Message, llm.Usage, error)
    StreamingInvoke(messages []llm.Message, tools []llm.Tool, eventCh chan<- events.AgentEvent) (llm.Message, llm.Usage)
}
```

**破坏性变更影响面详评（B-Blk5）**

调研 (`grep -RIn "\.StreamingInvoke\|\.Invoke(" internal/`) 得到调用点分布：

| 位置 | 用途 | 迁移成本 |
|---|---|---|
| `internal/domains/services/agents/base.go` (2 处) | `StreamingInvokeLLM` / `InvokeLLM` | 主目标，接住 Usage 写 span |
| `internal/domains/services/agents/react.go` / `plan.go` | 走 base.go 封装，**不直接调 Invoker** | 无 |
| 单测 mock：`internal/domains/services/agents/*_test.go` | mock Invoker 实现 | 3-5 处，机械修改 |
| 无其他外部调用点（Invoker 只在 domain/agents 内部使用） | | |

**兼容策略**：不保留旧签名过渡。一次性改，Go 编译器强制帮忙抓漏（漏改直接 build fail）。这比双签名带来的长期腐化更划算。

**风险清单**：
1. SDK 不返回 usage（如 OpenAI streaming 早期版本 / Anthropic 某些模型）→ Adapter 内部填 0 + `logger.Warn`，不阻断。
2. span tag 脱敏正则误杀（现有 `token|password|secret|authorization`）→ 实施阶段第 1 步先加单测 `TestUsageTagNotMasked`，若命中改用命名 `llm.io.prompt_units/completion_units/total_units`。以单测结果为准，本 spec 描述用 `llm.usage.*`。

**③ Adapter 侧填充 Usage**

- `OpenAIAdapter`: `OpenAiLLM.StreamingInvoke` 内部把 `acc.Usage` 一并返回；`openai.CompletionUsage → llm.Usage` 直接映射。
- `AnthropicAdapter`: SDK 最终响应带 `Usage.InputTokens / OutputTokens`；`Total = Input + Output` 组合出来。
- SDK 未返回 usage 时填 0 + 打 warn log，不中断。

**④ Span 写入**（`base.go: finalizeLLMSpanSuccess`）

```go
func (a *BaseAgent) finalizeLLMSpanSuccess(llmSpan *tracing.Span, toolCallsCount int, usage llm.Usage) {
    llmSpan.SetTag("llm.tool_calls_count", toolCallsCount)
    llmSpan.SetTag("llm.usage.prompt_tokens",     usage.PromptTokens)
    llmSpan.SetTag("llm.usage.completion_tokens", usage.CompletionTokens)
    llmSpan.SetTag("llm.usage.total_tokens",      usage.TotalTokens)
    llmSpan.AddLog("INFO", "llm.stream.completed", nil)
}
```

**脱敏正则兼容性验证**：现有 span 敏感脱敏正则 `api_key|token|password|secret|authorization`（`span.go:80`）会命中 tag key 里的 `token` 字串，实施阶段需先在单元测试里验证 `SetTag("llm.usage.prompt_tokens", 123)` 是否被误脱敏。若被误脱敏，退化命名为 `llm.io.prompt_units` / `completion_units` / `total_units`。以命名冲突为准，最终决定由实施阶段验证锁定，本 spec 用 `llm.usage.*` 表意。

### 4.3 评测指标聚合算法

`TraceAggregator` 按 `conversation_id` 拉全部 span，一次遍历得到全部指标：

```go
func (a *TraceAggregator) Aggregate(conversationID string) (Metrics, error) {
    spans, err := a.spanRepo.ListByConversation(conversationID)
    if err != nil { return Metrics{}, err }
    var m Metrics
    for _, s := range spans {
        switch s.SpanType {
        case tracing.SpanTypeAgentRoot:
            m.AgentLatencyMs = s.LatencyMs
        case tracing.SpanTypeLLMCall:
            m.PromptTokens     += tagInt64(s.Tags, "llm.usage.prompt_tokens")
            m.CompletionTokens += tagInt64(s.Tags, "llm.usage.completion_tokens")
            m.TotalTokens      += tagInt64(s.Tags, "llm.usage.total_tokens")
        }
    }
    return m, nil
}
```

**边界处理**

| 场景 | 处理 |
|---|---|
| 无 AGENT_ROOT span | `AgentLatencyMs = 0`；`error_log = "agent_root_span_missing"`；实例照走终态、只是指标降级 |
| 多个 AGENT_ROOT | 取 `latency_ms` 最大值 + 打 warn |
| LLM_CALL 缺 usage tag | 该 span 贡献 0；不做本地估算 |
| Span 异步落库延迟 | 聚合前调 `tracer.Flush()` + 等 200ms，避开尾端 span 尚在缓冲区（Tracer 默认 flush_interval=5s） |

### 4.4 验证脚本执行 —— 安全边界

**不复用智能体的 `bashExec` 工具**——`bashExec` 走全局 `bash_concurrency` 闸门，与评测混用互相阻塞。新建独立 `VerifyScriptRunner`：

- 独立协程 + 独立超时（默认 60s，可 case 级覆写）
- 工作目录 = 智能体主任务的同一个 `${WorkspaceBaseDir}/${conversationId}/${messageId}`（验证脚本要看到智能体产出的文件）
- 写入 `verify_script` 到 workspace 内 `.verify.sh` (`0700`)
- `exec.CommandContext(ctx, "bash", ".verify.sh")` + stdout/stderr Pipe，各截断 64KB
- **环境变量白名单**：仅 `PATH / HOME / LANG=C.UTF-8`（LANG 明确指定 UTF-8 防止中文乱码 —— N8）；显式清空 `OPENAI_API_KEY / ANTHROPIC_API_KEY / AZURE_*` 等敏感 env，防止 case 作者反向渗出 key
- 退出后**不清理 workspace** —— 由既有 `CleanupMessage()` 走标准回收路径

**关于"智能体可能 `rm -rf` 恶意破坏 workspace"（B-Blk6 回应）**：

Reviewer 提议在 `/tmp/eval-verify-{run_id}` 独立隔离目录跑 verify。**驳回**理由：

1. 评测的核心语义就是"看智能体在自己 workspace 里的产出"。搬到独立目录做验证，就要在两个目录间做文件同步，本身就有一致性风险。
2. 智能体已经在 `${WorkspaceBaseDir}/${conversationId}/${messageId}` 里被 NATIVE 三件套限制（`sensitive_path_deny_list` + `bash_command_deny_list` 见 `config.go:54`），路径穿透早已由 NATIVE 层守好。
3. 智能体故意 `rm -rf ./*` 只会破坏它**自己的 workspace**，导致 verify 判 FAILED —— 这本来就是评测想暴露的行为，不应该被 verify 屏蔽掉。
4. 顶层宿主机由 NATIVE `bash_command_deny_list` + Docker (若部署时启用) 兜底。

**结论**：workspace 隔离在 NATIVE 层完成，Verify 层不重复隔离，只做进程/env/超时隔离。此项已明确说清，不改设计。

**判分逻辑**：`exit_code == 0 → passed=true`；其他一律 false，stdout/stderr 落库。

---

## 5. 高并发批量评测架构（重点难点 3 —— Asynq 完整设计）

### 5.1 选型：Asynq（Redis 后端）

- 项目已有 Redis，零新组件；
- 内建：**持久化**、**unique task**、**retry with backoff**、**scheduled tasks**、**archive (DLQ)**、**Web UI**；
- 单 task = 1 条 Redis Hash + zset 索引，宕机重启天然恢复；
- Go 原生 API，改造量最小。

### 5.2 Topic / Queue 设计

| Queue | Priority | Concurrency | 用途 |
|---|---|---|---|
| `eval:default` | 5 | 8 | 常规评测任务 |
| `eval:high` | 10 | 4 | 用户显式触发的重试任务 |
| `eval:dlq` | 0 | 0 (不消费) | 归档失败任务（人肉排查） |

- Asynq server 单进程共用一个 goroutine pool（total concurrency=12）；按 priority 加权轮询。
- 单一 task type：`eval:run_instance`；payload 只带 `instance_id`（真实配置由 worker 反查 DB，避免大 payload）。

### 5.3 消息体 & 幂等

```go
// internal/infra/mq/payload.go
type RunInstancePayload struct {
    InstanceID string `json:"instance_id"`
    Attempt    int    `json:"attempt"`
    EnqueuedAt int64  `json:"enqueued_at"`
}
```

Enqueue 参数：

```go
info, err := client.Enqueue(
    asynq.NewTask("eval:run_instance", payload),
    asynq.Queue(queueName),          // eval:default / eval:high
    asynq.Unique(24*time.Hour),       // 防重复：task_type + payload SHA256
    asynq.MaxRetry(0),                // 我们自管重试
    asynq.Timeout(20*time.Minute),    // 业务超时 15min + 5min 缓冲
    asynq.Retention(72*time.Hour),    // 完成的 task 保留 3 天供排查
)
```

**幂等 key** = Asynq 默认 SHA256(task_type + payload bytes)。`Attempt` 是 payload 一部分，重试后 attempt+1 → 幂等 key 自然不同。

**幂等 TTL 覆盖失败重投场景（B-Blk7）**：`Unique(24h)` 意味着 24h 内**同一 (instance_id, attempt)** 无法重复入队。这刚好是我们要的语义 —— attempt 只有由用户显式触发或巡检器 CAS 推进 PENDING 时才 +1，attempt 换了幂等 key 就换了，Enqueue 冲突不会发生。若 Enqueue 因网络抖动被客户端自动重试且 Redis ACK 丢失，Asynq 客户端内建重试会靠幂等键去重（第二次 Enqueue 返回 `ErrDuplicateTask` 但 task 已在队列），业务侧忽略此错误即可。

### 5.4 消费策略 & 限流削峰

**5.4.1 case 级令牌桶**（防止某个 case 独占 worker pool）

```
Redis key: eval:concurrency:case:{case_id}
桶大小: 4 (config 可覆写)
实现: INCR + EXPIRE (Lua 脚本)
```

`ProcessTask` 入口：

1. INCR 令牌 → 超过阈值 → `return asynq.RetryTaskError(...)`，Asynq 30s 后重派（不消耗 attempt）
2. defer DECR

**不阻塞 goroutine 等令牌**（否则 worker pool 被单个热点 case 吃满）。

**5.4.2 削峰**

- Task 创建时不做限流（M×N Enqueue 每条几 ms，很轻）；
- 消费端由 concurrency + case 令牌桶天然削峰；
- 拿不到令牌的 task 走 Asynq scheduled 队列（zset 扫描线程，无 goroutine 泄漏）。

### 5.5 失败重试策略

**Asynq 层不自动重试**（`MaxRetry(0)`）。原因：

- 智能体失败多为永久性（prompt 不当、模型能力不足）；
- 无脑重试浪费 token；
- 用户显式重试语义更清晰；
- 巡检兜底孤儿实例。

**Worker 异常分类**：

| 异常 | 处理 |
|---|---|
| 拿不到 case 令牌 | `RetryTaskError`, 30s 后重派（不算 attempt） |
| Redis/DB 短暂故障 | `RetryTaskError`, 60s 后重派，最多 3 次 |
| init_script 失败 | `SkipRetry` → status=FAILED，写 error_log |
| 智能体总超时 | `SkipRetry` → status=TIMEOUT |
| verify_script 失败 (exit≠0) | `SkipRetry` → passed=false（业务正常，不属异常） |
| panic | defer recover → status=FAILED + stack trace，Asynq 上抛进入 DLQ |

### 5.6 宕机恢复

- **未取任务**：留在 Redis，Asynq server 重启自动恢复消费。
- **已取未完成**：Asynq 用 active list（zset by heartbeat），worker 心跳 5s 一次，超 30s 无更新→ 自动迁回 pending 重派。
- **业务侧孤儿**：worker 每 15s 调 `InstanceRepo.UpdateHeartbeat`；`InstanceHeartbeatSweeper` cron 每 30s 扫**同时满足**`status IN (INITIALIZING,RUNNING,VERIFYING) AND heartbeat_at < now - 90s AND deadline_at < now()` 三条件的实例强制推 TIMEOUT（B-Blk8：deadline_at 是绝对时间戳，长 LLM 调用只要还没到总超时上限就不会被误杀；90s 心跳阈值仅用于剔除 worker 崩溃场景，非用于超时判定）。
- **Redis 持久化**：项目 Redis 已开 RDB。建议部署侧开 AOF (`appendonly yes / appendfsync everysec`) 双重保险。此项属部署清单，非代码改动。

### 5.7 定时巡检 & 一致性

**引入** `github.com/robfig/cron/v3`（Go 生态事实标准）。在 `route.go: InitRouter` 内启动单例 Scheduler。

| Job | 频率 | 作用 |
|---|---|---|
| `InstanceHeartbeatSweeper` | `*/30 * * * * *` (30s) | 扫孤儿实例 → TIMEOUT |
| `TaskStatusReconciler` | `*/60 * * * * *` (60s) | 全量任务态一致性巡检（防漏推进）；同时兜底 `status=PENDING` 但队列无任务的实例重新 Enqueue |
| `AsynqDeadTaskArchiver` | `0 */5 * * * *` (5min) | 扫 Asynq DLQ，同步实例状态=FAILED，避免 DB 与队列脑裂 |

**多副本部署选主**：第一版单副本即可；扩容多副本时改用 `redislock` 保证 cron job 只有 1 副本执行。第一版不做，UPDATE 都是幂等 SQL 即使多副本并跑也不会脏写。

### 5.8 部署拓扑（单进程）

```
┌────────────────────────────────────────────────┐
│  mooc-manus binary                             │
│   ├─ Gin HTTP server (:8080)                   │
│   ├─ Asynq Server (concurrency=12)             │
│   ├─ Cron Scheduler (robfig/cron/v3)           │
│   └─ Global Tracer (existing)                  │
└────────────────────────────────────────────────┘
                    ▲
                    │ Redis (existing)
                    │   ├─ db 0: 现有缓存
                    │   └─ db 1: Asynq 队列 + 令牌桶 + 心跳
                    │
                    ▼
             PostgreSQL (existing)
              ├─ 5 张 eval_* 表
              └─ ai_span (复用)
```

Asynq 也可拆独立进程 `cmd/eval-worker/main.go`；第一版单进程合体，后续扩容再拆。

### 5.9 Redis DB 号选择（B-Blk12 明确）

- 项目现有 Redis 使用 `db 0`（`config/config.go: RedisConfig` 默认 DB=0）。
- 评测 Asynq **独立占用 `db 1`**，与业务缓存物理隔离。
- 若未来运维发现 db 0 与 db 1 冲突（例如引入其他中间件），实施阶段配置项 `asynq.redis_db` 一处可切换，不涉及代码改动。
- 部署校验：启动时 `asynq_client.go` 应打 info log `Asynq connected redis db=1`，并检查该 db 下无残留 `mooc:*` 业务前缀 key（若有则告警）。

---

## 6. API 接口清单（15 个）

所有接口挂 `/api/eval/*`；路由注册走 `api/routers/route.go` 的 `InitRouter`；响应统一 `{"code":0,"message":"ok","data":...}`。

### 6.1 评测用例

| Method | Path | 说明 |
|---|---|---|
| POST | `/api/eval/cases/upload-content` | multipart 上传，返回解析后的 UTF-8 文本；上限 256KB |
| POST | `/api/eval/cases` | JSON 创建；`init_script/task_prompt/verify_script` 直接是文本 |
| GET | `/api/eval/cases` | 分页 + name 模糊 + tags 筛选 |
| GET | `/api/eval/cases/{id}` | 详情 |
| PUT | `/api/eval/cases/{id}` | 更新；不影响已生成的 instance（case_snapshot 已冻结） |
| DELETE | `/api/eval/cases/{id}` | 物理删；若有 task 未终态且引用本 case → 409 |

**上传两阶段协议**：前端先 `upload-content` 拿文本，用户可编辑后一并走 `POST /cases`；后端不存 blob，只存文本。手动/上传互斥在前端表单层保证，后端只校验"三要素文本非空"。

### 6.2 评测任务

| Method | Path | 说明 |
|---|---|---|
| POST | `/api/eval/tasks` | body: `{name, case_ids:[...], agent_config_ids:[...]}`；M×N 展开、冻结 snapshot、批量入库 + 批量 Enqueue |
| GET | `/api/eval/tasks` | 分页 + status 筛选 + name 模糊 + 时间区间 |
| GET | `/api/eval/tasks/{id}` | 任务详情（含 4 count 汇总 + 状态） |
| POST | `/api/eval/tasks/{id}/retry` | 整任务重试；仅重投 FAILED/TIMEOUT，入 `eval:high` |
| DELETE | `/api/eval/tasks/{id}` | 级联删 task + instance + result + snapshot |

**Task 创建事务边界**：单个 DB 事务里 INSERT 1 task + M×N instance + N snapshot；事务提交后再逐条 Enqueue。Enqueue 失败的实例保持 PENDING，由 `TaskStatusReconciler` cron 兜底补 Enqueue。

### 6.3 运行实例

| Method | Path | 说明 |
|---|---|---|
| GET | `/api/eval/tasks/{id}/instances` | 任务下实例列表：分页 + status/case_id/snapshot_id 筛选 |
| GET | `/api/eval/instances/{id}` | 实例详情（含 result + trace_id） |
| GET | `/api/eval/instances/{id}/trace` | 转发到既有 `/api/trace/spans?conversation_id=...` |
| POST | `/api/eval/instances/{id}/retry` | 单实例重试；入 `eval:high` |
| DELETE | `/api/eval/instances/{id}` | 物理删单实例 + result；重算 task 计数 |

### 6.4 元数据

| Method | Path | 说明 |
|---|---|---|
| GET | `/api/eval/agent-configs` | 转发既有 appConfig 列表，方便前端选智能体 |

---

## 7. 代码模块规划（严格 DDD 四层）

```
mooc-manus/
├── api/
│   └── handlers/
│       └── eval.go                                # 15 个 handler 方法
├── internal/
│   ├── applications/
│   │   ├── services/eval.go                       # EvaluationApplicationServiceImpl
│   │   └── dtos/eval.go                           # 请求/响应 DTO + PO↔DO↔DTO 转换
│   ├── domains/
│   │   ├── services/evaluation/
│   │   │   ├── service.go                         # EvaluationDomainService 接口
│   │   │   ├── service_impl.go                    # 实现（编排 executor/aggregator/state_machine）
│   │   │   ├── state_machine.go                   # 两套状态机白名单
│   │   │   ├── executor.go                        # InstanceExecutor：跑单条 instance 全流程
│   │   │   ├── internal_chat_runner.go            # 复用 BaseAgentDomainService + eventCh close 阻塞
│   │   │   ├── verify_runner.go                   # VerifyScriptRunner
│   │   │   ├── trace_aggregator.go                # 按 conversation_id 聚合
│   │   │   └── snapshot.go                        # AppConfig → Snapshot 冻结
│   │   └── models/
│   │       ├── evaluation/                        # DO
│   │       │   ├── case.go
│   │       │   ├── task.go
│   │       │   ├── run_instance.go                # + Status 枚举常量
│   │       │   ├── result.go
│   │       │   └── agent_snapshot.go
│   │       └── llm/usage.go                       # ← 新增 Usage 值对象
│   ├── domains/models/invoker/invoker.go          # ← 接口签名扩容
│   ├── domains/services/agents/base.go            # ← 补 token span 写入
│   └── infra/
│       ├── models/                                # GORM PO
│       │   ├── eval_case.go
│       │   ├── eval_task.go
│       │   ├── eval_run_instance.go
│       │   ├── eval_result.go
│       │   └── eval_agent_snapshot.go
│       ├── repositories/
│       │   ├── eval_case_repository.go
│       │   ├── eval_task_repository.go
│       │   ├── eval_run_instance_repository.go
│       │   ├── eval_result_repository.go
│       │   └── eval_agent_snapshot_repository.go
│       ├── mq/                                    # 新增基础设施
│       │   ├── asynq_client.go                    # 单例 client
│       │   ├── asynq_server.go                    # server + mux
│       │   ├── task_types.go                      # 常量
│       │   └── payload.go
│       ├── scheduler/                             # 新增基础设施
│       │   ├── cron.go                            # robfig/cron/v3 单例
│       │   └── jobs.go                            # 3 个 cron job
│       └── external/llm/
│           ├── openai.go                          # ← 补 Usage 返回
│           ├── openai_adapter.go                  # ← 补 Usage 返回
│           └── anthropic_adapter.go               # ← 补 Usage 返回
```

### 7.1 数据库迁移

- 位置：`mooc-manus/data/migrations/`（对齐既有目录；若无则使用 GORM AutoMigrate，与项目现行方式一致，实施阶段最终确认）
- 5 张新表 SQL 一次落库
- 索引：`eval_run_instance(task_id, status)`、`eval_run_instance(heartbeat_at)`、`eval_task(status, created_at DESC)`、`eval_case(name UNIQUE)`、`eval_case GIN(tags)`

### 7.2 配置补充（`config/config.toml`）

```toml
[evaluation]
enabled = true
worker_concurrency_default = 8
worker_concurrency_high = 4
case_concurrency_limit = 4
instance_total_timeout_sec = 900        # 15min
verify_script_timeout_sec = 60
verify_output_cap_bytes = 65536         # 64KB
heartbeat_interval_sec = 15
heartbeat_stale_threshold_sec = 90
cron_sweeper_interval_sec = 30
cron_reconciler_interval_sec = 60
cron_dlq_archive_interval_sec = 300
upload_content_max_bytes = 262144       # 256KB

[asynq]
redis_addr = "127.0.0.1:6379"           # 复用现有 Redis
redis_password = ""
redis_db = 1                             # 独立 DB 号，避免污染现有缓存
```

### 7.3 破坏性变更影响面

| 位置 | 改动 | 影响 |
|---|---|---|
| `invoker.Invoker` 接口 | 返回值 +`llm.Usage` | 2 个 adapter、`BaseAgent` 2 个调用点、单测 mock 3-5 处 |
| `finalizeLLMSpanSuccess` | 参数 +`usage llm.Usage` | 仅内部调用点 |
| span tag key `llm.usage.*` | 新增 3 个 tag | 需验证脱敏正则不误杀（实施阶段单测锁定） |

其余全部新增文件，不影响任何现有 API。

---

## 8. 测试策略

### 8.1 单测

- `state_machine_test.go`：穷举合法/非法流转组合
- `trace_aggregator_test.go`：缺 root / 多 root / 缺 usage tag 三种分支
- `verify_runner_test.go`：exit=0 / exit≠0 / 超时 / stdout 截断
- `snapshot_test.go`：AppConfig 深拷贝到 Snapshot 完整性
- LLM adapter 单测：mock SDK 返回带 Usage / 不带 Usage 两种情况

### 8.2 集成测

- `executor_integration_test.go`：sqlite in-memory + mock Invoker，跑一条 case 完整 PENDING → PASSED 生命周期
- `task_lifecycle_test.go`：M=2, N=2 生成 4 条 instance，验证 4 count 字段一致性、状态机推进正确

### 8.3 压测

- 一次性 Enqueue 500 条 instance，观察：
  - Worker pool concurrency=12 + case 令牌桶=4 稳态
  - `pprof/goroutine` 前后差 < 20（无泄漏）
  - Redis QPS 峰值合理（< 2000）
  - PG 无长事务锁等待

### 8.4 E2E

详见独立文档 `docs/superpowers/plans/2026-07-16-agent-evaluation-e2e.md`（实施计划阶段生成）。

---

## 9. 里程碑（供实施计划分解）

1. **M1 数据层与迁移**：5 张表 + PO/Repository + AutoMigrate（含 FK/CASCADE/复合索引）
2. **M2 状态机 + Domain skeleton**：`state_machine.go` + `EvaluationDomainService` 接口 + `evaluation` DO
3. **M3 Invoker Usage 补强**：改接口 + 2 个 adapter + `finalizeLLMSpanSuccess` + `TestUsageTagNotMasked` 单测
4. **M4 核心执行链路**：`InstanceExecutor + VerifyRunner + TraceAggregator + Snapshot` （**顺序调整（B-Blk10）**：把执行链路提前于 Asynq 集成，保证 M5 Asynq worker 有可用的 `ExecuteInstance` 入口，避免空壳）
5. **M5 Asynq 基础设施**：`internal/infra/mq/*` + route.go 注册 Asynq server + worker 调用 M4 的 ExecuteInstance
6. **M6 Application + Handler + Routes**：15 个 API 落地
7. **M7 Cron 巡检 3 个 job**：`internal/infra/scheduler/*`
8. **M8 集成测 + 压测**
9. **M9 E2E 验证 + 文档收尾**

---

## 10. 关键决策记录（Decision Log）

| # | 决策 | 备选 | 理由 |
|---|---|---|---|
| D1 | MQ 用 Asynq (Redis 后端) | Redis Streams / NATS / Kafka | 零新组件；开箱内建 unique/retry/DLQ/UI |
| D2 | 沙盒复用 NATIVE workspace | 独立 Docker 容器 / subdir 隔离 | 与主流程完全对齐；零改造 |
| D3 | 等待智能体收敛 = 内部 eventCh close + 总超时兜底 | HTTP SSE / 同步 Invoke | 避免 HTTP 往返；保留流事件供 debug |
| D4 | 上传两阶段协议 | 单阶段 multipart / blob URI | 后端仅存文本，存储模型干净 |
| D5 | 总耗时=AGENT_ROOT.latency_ms；Σ token from LLM_CALL | 壁钟 / 两者都存 | 语义纯净；避免混入非智能体耗时 |
| D6 | 重试就地重跑、保留 attempt 历史 | 新建实例 / 多行 result | 前端进度口径不变；表结构最简 |
| D7 | 表前缀统一 `eval_`；`eval_case` 物理删；`case_snapshot` 冻结用例 | evaluation_ 前缀 / 软删 | 用户明确要求；用 snapshot 解耦历史可回溯 |

---

## 11. 风险与缓解

| 风险 | 缓解 |
|---|---|
| span tag `llm.usage.*` 被脱敏正则误杀 | 实施阶段单测锁定；如误杀改名 `llm.io.*_units` |
| worker 心跳未更新但 task 仍在跑（假 TIMEOUT） | 心跳周期 15s、阈值 90s（6× 冗余）；Asynq 层 20min 硬超时兜底 |
| M×N 过大（如 20×20=400）Enqueue 慢 | Enqueue 分批（每批 50）；事务外并行 |
| 巡检器多副本重复扫 | 第一版单副本；SQL 幂等；未来 redislock 选主 |
| 智能体死循环耗尽 token | 现有 `circuitBreaker` + `MaxIterations` 已在 BaseAgent 层生效；评测无额外风险 |
| 用户上传 verify 脚本恶意扫描宿主机 | env 白名单 + 独立 timeout；不给 SUDO；workspace 隔离 |

---

## 12. Reviewer 反馈响应清单（2026-07-16 Round 1）

**接受并已改（Blocker）**：

- B-Blk1 `eval_case` 输入上下文：**驳回** —— 输入文件全部由 `init_script` 建立，无需新增字段。已在 §2.1 明确职责边界。
- B-Blk2 实例级超时：**采纳** —— §2.3 增 `deadline_at`；巡检器改比较 `deadline_at < now()` 而非 heartbeat 阈值。
- B-Blk3 物理删除外键一致性：**采纳** —— §2.6 新增"表间外键 & 级联删除顺序"节，声明 CASCADE 关系与事务边界。
- B-Blk4 `FAILED` 状态语义：**采纳** —— §3.1/§3.2 状态机中 `FAILED` 早已列入；§3.3 明确 FAILED vs TIMEOUT 判定口径。
- B-Blk5 Invoker 破坏性变更影响面：**采纳** —— §4.2 增"破坏性变更影响面详评"表 + 兼容策略讨论 + 风险清单。
- B-Blk6 验证脚本同目录安全：**驳回** —— §4.4 已加详细驳回理由（workspace 隔离在 NATIVE 层）。
- B-Blk7 Asynq 幂等键：**部分采纳** —— Unique + Attempt 组合已在设计中；§5.3 补充"幂等 TTL 覆盖失败重投场景"段说清语义。
- B-Blk8 心跳周期对长 LLM 请求误判：**采纳** —— §5.6 加入 `deadline_at < now()` 与心跳阈值 AND 合取判定，防止长 LLM 调用被误杀。
- B-Blk9 Application 层用例编排器：**驳回** —— `EvaluationApplicationService` 已在 §1、§7 明确存在，Handler → App → Domain → Repo 严格四层，reviewer 误读。
- B-Blk10 M4/M5 里程碑顺序：**采纳** —— §9 交换 M4/M5。
- B-Blk11 error_log 长度截断：**采纳** —— §2.4.1 明确 64KB 截断。
- B-Blk12 Redis DB 冲突：**采纳** —— §5.9 明确 db 号选择规则与启动检查。

**接受并已改（Nice-to-have）**：

- N1 `case.category` 枚举：**驳回** —— 本 spec 用 jsonb `tags` 更灵活，非枚举。
- N2 `eval_run_instance` 复合索引：**采纳** —— §2.3 索引改 `(status, heartbeat_at)`。
- N3 OpenAI streaming usage 可能为空：**采纳** —— §4.2 风险清单第 1 条。
- N4 span_ids BIGINT[] vs jsonb：**驳回** —— `eval_result` 表中无 `span_ids` 字段，跨表按 `conversation_id` 查询 `ai_span` 即可。
- N5 巡检强杀 Asynq 任务残留：**采纳到实施计划** —— Worker 每 3s 轮询自身 `run_instance.status`，发现被巡检器改成 TIMEOUT 主动退出，避免 goroutine 泄漏。写进 §5.5 worker 循环细节。
- N6 Swagger：**采纳到实施计划** —— 项目已有 swaggo，M6 里程碑扩展"补 Swagger 注解"。
- N7 定期清理 job：**驳回** —— 数据保留策略非本次范围；未来运维需求可加。
- N8 LANG=C.UTF-8：**采纳** —— §4.4 已改。
- N9 Asynq MaxRetry：**驳回** —— MaxRetry(0) + 巡检兜底是有意设计；瞬时 Redis 故障由 Asynq client 内建重连处理。
- N10 `paused` 状态：**驳回** —— 本次不设计运维暂停；用户手删即可。

追加实施计划要求：

- 补 N5：Worker 主循环内加"每 3s SELECT status FROM eval_run_instance WHERE id=? "，若发现被外部改成 TIMEOUT 则主动 return，避免 goroutine 泄漏。这是 §5.6 与 N5 的合并要求。
- 补 N6：M6 里程碑扩展 Swagger 注解。

---

**End of Spec**







