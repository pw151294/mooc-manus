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
| `worker_id` | `varchar(64)` | 处理该实例的 worker 标识 |
| `error_message` | `text` | 排队/初始化阶段的失败摘要 |

约束与索引：
- `UNIQUE (task_id, case_id, agent_config_snapshot_id)` — 天然幂等
- `INDEX (task_id, status)`
- `INDEX (heartbeat_at)` — 供孤儿巡检
- `INDEX (status, queued_at)` — 供 Enqueue 兜底

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

**破坏性变更**，但 Invoker 实现仅 2 个（`OpenAIAdapter` / `AnthropicAdapter`）、调用点仅 2 处（`base.go: StreamingInvokeLLM` / `InvokeLLM`）。测试 mock 预估 3-5 处，一次改完。

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
- **环境变量白名单**：仅 `PATH / HOME / LANG`；显式清空 `OPENAI_API_KEY / ANTHROPIC_API_KEY / AZURE_*` 等敏感 env，防止 case 作者反向渗出 key
- 退出后**不清理 workspace** —— 由既有 `CleanupMessage()` 走标准回收路径

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
- **业务侧孤儿**：worker 每 15s 调 `InstanceRepo.UpdateHeartbeat`；`InstanceHeartbeatSweeper` cron 每 30s 扫 `heartbeat_at < now - 90s` 强制推 TIMEOUT。
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

1. **M1 数据层与迁移**：5 张表 + PO/Repository + AutoMigrate
2. **M2 状态机 + Domain skeleton**：`state_machine.go` + `EvaluationDomainService` 接口 + `evaluation` DO
3. **M3 Invoker Usage 补强**：改接口 + 2 个 adapter + `finalizeLLMSpanSuccess` + 单测
4. **M4 Asynq 基础设施**：`internal/infra/mq/*` + route.go 注册 Asynq server
5. **M5 InstanceExecutor + VerifyRunner + TraceAggregator**：核心执行链路
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

**End of Spec**







