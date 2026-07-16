# 智能体自动化评测机制 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 mooc-manus 编程智能体落地"自动化评测机制"完整能力：数据层 5 表、双状态机、LLM_CALL span token 补齐、Asynq 队列、Cron 巡检、15 个管理 API。

**Architecture:** 严格 DDD 四层（Handler → Application → Domain → Repository）。评测运行走 `Asynq(Redis)` 队列解耦，禁止直接 `go func()`；智能体执行复用现有 `BaseAgentDomainService` + NATIVE workspace 隔离；验证脚本走独立 `VerifyScriptRunner`；指标从既有 `ai_span` 聚合，`LLM_CALL` span 需先补齐 token usage tag。

**Tech Stack:**
- Go 1.22 + Gin + GORM v2 + PostgreSQL 15 + Redis 7
- `github.com/hibiken/asynq` v0.24+（评测队列）
- `github.com/robfig/cron/v3`（定时巡检）
- 复用既有：`go.uber.org/zap`、`viper`、既有 `tracing.Tracer`

**Spec 依据:** `docs/superpowers/specs/2026-07-16-agent-evaluation-design.md`（已 APPROVED）

**里程碑总览（9 个）**：M1 数据层 → M2 状态机 → M3 Invoker Usage 补强 → M4 核心执行链路 → M5 Asynq 基础设施 → M6 API 落地 → M7 Cron 巡检 → M8 集成测/压测 → M9 E2E + 文档

**通用护栏（每个任务都必须遵守）**：

1. 所有对话/注释/日志/commit message 中文。
2. 路由**统一在** `api/routers/route.go: InitRouter` 注册，禁止散落。
3. 分层严格：Handler 只做参数校验和响应；Application 做 DTO 转换、事务、事件编排；Domain 做业务逻辑；Repository 做 DB IO。
4. Repository 层永远只操作 `PO`；Application 层做 `PO ↔ DO ↔ DTO`；Domain 层只见 `DO`。
5. 每个 Task 结束必须 `git commit`，commit message 用 `feat(eval): xxx` / `test(eval): xxx` / `refactor(llm): xxx` 前缀。
6. 每写一段实现前先写失败测试（TDD）；单测覆盖率目标 > 70%。
7. 遇到红线（改子仓、跨层、绕过 route.go）立即停下报警。

---

## 文件结构总览

新增文件：

```
api/handlers/eval.go
internal/applications/services/eval.go
internal/applications/dtos/eval.go
internal/domains/models/evaluation/{case,task,run_instance,result,agent_snapshot,status}.go
internal/domains/models/llm/usage.go
internal/domains/services/evaluation/{service,service_impl,state_machine,executor,internal_chat_runner,verify_runner,trace_aggregator,snapshot}.go
internal/infra/models/{eval_case,eval_task,eval_run_instance,eval_result,eval_agent_snapshot}.go
internal/infra/repositories/{eval_case,eval_task,eval_run_instance,eval_result,eval_agent_snapshot}_repository.go
internal/infra/mq/{asynq_client,asynq_server,task_types,payload}.go
internal/infra/scheduler/{cron,jobs}.go
```

修改文件（破坏性）：

```
internal/domains/models/invoker/invoker.go      # 接口签名扩容
internal/infra/external/llm/openai.go           # 补 Usage 返回
internal/infra/external/llm/openai_adapter.go
internal/infra/external/llm/anthropic_adapter.go
internal/domains/services/agents/base.go        # finalizeLLMSpanSuccess 补 tag
api/routers/route.go                            # 注册 eval 路由 + Asynq server + Cron
config/config.go / config/config.toml           # [evaluation] + [asynq] 段
```

---

## M1 · 数据层与迁移

### Task 1.1: 定义 Domain Object（DO）与状态枚举常量

**Files:**
- Create: `internal/domains/models/evaluation/status.go`
- Create: `internal/domains/models/evaluation/case.go`
- Create: `internal/domains/models/evaluation/task.go`
- Create: `internal/domains/models/evaluation/run_instance.go`
- Create: `internal/domains/models/evaluation/result.go`
- Create: `internal/domains/models/evaluation/agent_snapshot.go`

- [ ] **Step 1: 写 `status.go` 枚举常量**

```go
package evaluation

type TaskStatus string
type InstanceStatus string

const (
    TaskStatusPending        TaskStatus = "PENDING"
    TaskStatusRunning        TaskStatus = "RUNNING"
    TaskStatusSucceeded      TaskStatus = "SUCCEEDED"
    TaskStatusPartialFailed  TaskStatus = "PARTIAL_FAILED"
)

const (
    InstanceStatusPending      InstanceStatus = "PENDING"
    InstanceStatusQueued       InstanceStatus = "QUEUED"
    InstanceStatusInitializing InstanceStatus = "INITIALIZING"
    InstanceStatusRunning      InstanceStatus = "RUNNING"
    InstanceStatusVerifying    InstanceStatus = "VERIFYING"
    InstanceStatusPassed       InstanceStatus = "PASSED"
    InstanceStatusFailed       InstanceStatus = "FAILED"
    InstanceStatusTimeout      InstanceStatus = "TIMEOUT"
    InstanceStatusCanceled     InstanceStatus = "CANCELED" // 预留
)

func (s InstanceStatus) IsTerminal() bool {
    switch s {
    case InstanceStatusPassed, InstanceStatusFailed, InstanceStatusTimeout, InstanceStatusCanceled:
        return true
    }
    return false
}
```

- [ ] **Step 2: 写其余 5 个 DO struct**

对齐 spec §2.1-2.5 字段。示例 `case.go`:

```go
package evaluation

import "time"

type Case struct {
    ID           string
    Name         string
    Description  string
    InitScript   string   // 可空
    TaskPrompt   string
    VerifyScript string
    Tags         []string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

其余 `Task/RunInstance/Result/AgentSnapshot` 依 spec §2 字段一一映射；时间字段用 `time.Time`；jsonb 字段用 `map[string]any` 或强类型 slice。`RunInstance.CaseSnapshot` 用 `Case` 值类型内嵌。

- [ ] **Step 3: Commit**

```bash
git add internal/domains/models/evaluation/
git commit -m "feat(eval): 新增 evaluation 域模型 DO 与状态枚举 (M1.1)"
```

---

### Task 1.2: 定义 GORM PO 与 AutoMigrate

**Files:**
- Create: `internal/infra/models/eval_case.go`
- Create: `internal/infra/models/eval_task.go`
- Create: `internal/infra/models/eval_run_instance.go`
- Create: `internal/infra/models/eval_result.go`
- Create: `internal/infra/models/eval_agent_snapshot.go`
- Modify: `internal/infra/storage/postgres.go`（**PlanReview-Blk3 修复**：项目目前没有集中 AutoMigrate 入口，各表靠外部 SQL 建。本任务在 `InitStorage()` 尾部新增集中式 AutoMigrate 段，只处理评测 5 表；不动其它历史表，规避对 `ai_span` 等既有表的隐式变更。）

- [ ] **Step 1: 写 5 张 PO struct，严格按 spec §2 字段与索引**

关键点：
- 所有时间字段用 `time.Time`，GORM 默认映射为 `TIMESTAMPTZ` —— 与 `AiSpanPO` 保持一致（spec §2 时区约定）
- `Tags` / `CaseSnapshot` / `CaseIDs` / `AgentConfigSnapshotIDs` / `ToolsConfig` / `MCPConfig` / `A2AConfig` 用 `datatypes.JSON`（`gorm.io/datatypes`）
- 外键：`RunInstance.TaskID` 加 GORM tag `gorm:"index;constraint:OnDelete:CASCADE"`；`RunInstance.AgentConfigSnapshotID` 加 `gorm:"constraint:OnDelete:RESTRICT"`；`Result.InstanceID` 加 `gorm:"uniqueIndex;constraint:OnDelete:CASCADE"`
- 复合索引 `(status, heartbeat_at)`：`gorm:"index:idx_status_heartbeat"` 双列同名索引名合并
- `Case.Name` 加 `gorm:"uniqueIndex"`；`Case.Tags` 加 `gorm:"type:jsonb"` + GIN 手动 CREATE INDEX（在 migrate 后置 hook 里追加）

示例 `eval_run_instance.go`：

```go
package models

import (
    "time"
    "gorm.io/datatypes"
)

type EvalRunInstancePO struct {
    ID                      string         `gorm:"type:uuid;primaryKey"`
    TaskID                  string         `gorm:"type:uuid;index:idx_task_status,priority:1;constraint:OnDelete:CASCADE"`
    CaseID                  string         `gorm:"type:uuid"` // 逻辑关联，无 FK
    CaseSnapshot            datatypes.JSON `gorm:"type:jsonb"`
    AgentConfigSnapshotID   string         `gorm:"type:uuid;constraint:OnDelete:RESTRICT"`
    Status                  string         `gorm:"type:varchar(24);index:idx_task_status,priority:2;index:idx_status_heartbeat,priority:1"`
    Attempt                 int
    ConversationID          string         `gorm:"type:varchar(64)"`
    MessageID               string         `gorm:"type:varchar(64)"`
    TraceID                 string         `gorm:"type:varchar(64)"`
    QueuedAt                *time.Time
    StartedAt               *time.Time
    FinishedAt              *time.Time
    HeartbeatAt             *time.Time     `gorm:"index:idx_status_heartbeat,priority:2"`
    DeadlineAt              *time.Time
    WorkerID                string         `gorm:"type:varchar(64)"`
    ErrorMessage            string         `gorm:"type:text"`
}

func (EvalRunInstancePO) TableName() string { return "eval_run_instance" }
```

**唯一约束**：在 `TableName()` 之外用 GORM tag `uniqueIndex:uk_task_case_snap`（三列同名合并）：

```go
    TaskID                string `gorm:"...;uniqueIndex:uk_task_case_snap,priority:1"`
    CaseID                string `gorm:"...;uniqueIndex:uk_task_case_snap,priority:2"`
    AgentConfigSnapshotID string `gorm:"...;uniqueIndex:uk_task_case_snap,priority:3"`
```

- [ ] **Step 2: 在 `storage/postgres.go: InitStorage()` 尾部追加集中式 AutoMigrate**

```go
// 落在 sqlDB = db 之后
if err := db.AutoMigrate(
    &models.EvalCasePO{},
    &models.EvalTaskPO{},
    &models.EvalAgentSnapshotPO{},  // instance FK 依赖 snapshot，snapshot 先建
    &models.EvalRunInstancePO{},
    &models.EvalResultPO{},         // result FK 依赖 instance
); err != nil {
    return fmt.Errorf("eval AutoMigrate: %w", err)
}
```

**建表顺序**：snapshot / task 无外键出边，先建；`eval_run_instance` FK 到 snapshot + task，中间建；`eval_result` FK 到 instance，最后建。GORM 会自动按依赖排序，此处显式列顺序仅为可读性。

- [ ] **Step 3: GIN 索引 post-hook**

在 AutoMigrate 之后紧邻处，追加一次性 SQL：

```go
db.Exec(`CREATE INDEX IF NOT EXISTS idx_eval_case_tags_gin ON eval_case USING GIN (tags)`)
```

- [ ] **Step 4: 启动跑一次 verify 表结构**

```bash
go build ./... && ./mooc-manus --dry-run-migrate 2>&1 | head -50
```

（若项目无 dry-run 选项，直接 `go run main.go` 起服务，观察 GORM 日志里 CREATE TABLE 语句是否 5 张表齐全，然后立即 Ctrl-C。）

- [ ] **Step 5: Commit**

```bash
git add internal/infra/models/ <migrate 文件>
git commit -m "feat(eval): 新增 5 张 eval_* PO 与 AutoMigrate 注册 (M1.2)"
```

---

### Task 1.3: 定义 Repository 接口（放在 `internal/infra/repositories/` 内，与既有习惯对齐）

**架构说明（PlanReview-Blk1 修复）**：项目现状是接口 + 实现全部放在 `internal/infra/repositories/`（对齐 `app_config.go:12` 现有做法）。本计划遵循现状，不在 `internal/domains/repositories/` 建新目录以免风格分裂；接口与实现放同 package，仅通过文件分离。

**Files:**
- Create: `internal/infra/repositories/eval_case.go`（接口 + 结构，同文件；实现在 Task 1.4）
- Create: `internal/infra/repositories/eval_task.go`
- Create: `internal/infra/repositories/eval_run_instance.go`
- Create: `internal/infra/repositories/eval_result.go`
- Create: `internal/infra/repositories/eval_agent_snapshot.go`

- [ ] **Step 1: 定义 5 个接口**

只列出关键方法，其余 CRUD 按需增补。所有方法收/发 DO，不见 PO。

```go
package repositories

import (
    "context"
    "mooc-manus/internal/domains/models/evaluation"
)

type EvalCaseRepository interface {
    Create(ctx context.Context, c *evaluation.Case) error
    Get(ctx context.Context, id string) (*evaluation.Case, error)
    List(ctx context.Context, filter CaseListFilter, page, size int) ([]*evaluation.Case, int64, error)
    Update(ctx context.Context, c *evaluation.Case) error
    Delete(ctx context.Context, id string) error
    ExistsRunningReferences(ctx context.Context, caseID string) (bool, error) // 供删除前置校验
}

type CaseListFilter struct {
    NameLike string
    Tags     []string
}
```

`EvalRunInstanceRepository` 关键方法：

```go
    ListStaleInstances(ctx context.Context, before time.Time) ([]*evaluation.RunInstance, error)
    UpdateHeartbeat(ctx context.Context, id, workerID string, now time.Time) error
    CASStatus(ctx context.Context, id string, from, to evaluation.InstanceStatus) (bool, error) // rows_affected > 0
```

`EvalTaskRepository` 关键方法：

```go
    RecountAndTransit(ctx context.Context, taskID string) error // §3.1 一次 SQL + CAS 推进
```

- [ ] **Step 2: Commit**

```bash
git add internal/infra/repositories/eval_*.go
git commit -m "feat(eval): 新增 5 个 Repository 接口定义 (M1.3)"
```

---

### Task 1.4: Repository GORM 实现 + 转换函数 + 单测

**Files:**
- Modify: 1.3 创建的 5 个文件，追加对应的 `*Impl` 结构体与方法
- Create: `internal/infra/repositories/eval_converter.go`（PO ↔ DO 双向转换）
- Test: `internal/infra/repositories/eval_run_instance_test.go`（选最复杂的做示范，其余 CRUD 类的走集成测覆盖）

- [ ] **Step 1: 写 `eval_converter.go`**

每张表两个函数：`caseToDO(po) *evaluation.Case` / `caseToPO(do) *models.EvalCasePO`；同类处理 5 张表。JSON 字段用 `json.Marshal/Unmarshal`。

- [ ] **Step 2: 写 5 个 Repository GORM 实现**

`New*Repository(db *gorm.DB) EvalXxxRepository` 工厂；每个方法内 `r.db.WithContext(ctx).Table(...)`。

**关键实现细节**：

- `EvalRunInstanceRepositoryImpl.CASStatus`：

```go
result := r.db.WithContext(ctx).Model(&models.EvalRunInstancePO{}).
    Where("id = ? AND status = ?", id, string(from)).
    Update("status", string(to))
return result.RowsAffected > 0, result.Error
```

- `EvalTaskRepositoryImpl.RecountAndTransit`：单一事务 + `SELECT COUNT(*) FILTER(...)` 原生 SQL；根据结果 CAS UPDATE task。

```go
return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    var terminal, total, passed int64
    err := tx.Raw(`
        SELECT
          COUNT(*) FILTER (WHERE status IN ('PASSED','FAILED','TIMEOUT','CANCELED')) AS terminal,
          COUNT(*) FILTER (WHERE status='PASSED') AS passed,
          COUNT(*) AS total
        FROM eval_run_instance WHERE task_id = ?
    `, taskID).Row().Scan(&terminal, &passed, &total)
    if err != nil { return err }

    var newStatus string
    switch {
    case terminal < total:                                 newStatus = "RUNNING"
    case terminal == total && passed == total:             newStatus = "SUCCEEDED"
    default:                                                newStatus = "PARTIAL_FAILED"
    }
    now := time.Now()
    upd := map[string]any{
        "status": newStatus,
        "succeeded_count": passed,
        "failed_count": terminal - passed,
        "running_count": total - terminal,
    }
    if newStatus == "SUCCEEDED" || newStatus == "PARTIAL_FAILED" {
        upd["finished_at"] = &now
    }
    return tx.Model(&models.EvalTaskPO{}).Where("id=?", taskID).Updates(upd).Error
})
```

- `EvalRunInstanceRepositoryImpl.ListStaleInstances`：只查非终态 + heartbeat 落后 + deadline 已过（同时满足三条件）。

- [ ] **Step 3: 写 CAS 竞态单测**

```go
func TestCASStatusRacesOnce(t *testing.T) {
    db := setupTestPG(t) // 或 sqlite in-memory + gorm
    repo := NewEvalRunInstanceRepository(db)
    id := seedInstance(t, db, evaluation.InstanceStatusRunning)

    var wg sync.WaitGroup
    results := make([]bool, 2)
    for i := 0; i < 2; i++ {
        i := i
        wg.Add(1)
        go func() {
            defer wg.Done()
            ok, err := repo.CASStatus(context.Background(), id,
                evaluation.InstanceStatusRunning, evaluation.InstanceStatusVerifying)
            require.NoError(t, err)
            results[i] = ok
        }()
    }
    wg.Wait()

    won := 0
    for _, r := range results { if r { won++ } }
    assert.Equal(t, 1, won, "只应有一方 CAS 成功")
}
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/infra/repositories/... -run TestCASStatus -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/repositories/
git commit -m "feat(eval): Repository GORM 实现 + CAS 竞态单测 (M1.4)"
```

---

## M2 · 状态机 + Domain Skeleton

### Task 2.1: state_machine.go 白名单函数（TDD）

**Files:**
- Create: `internal/domains/services/evaluation/state_machine.go`
- Test: `internal/domains/services/evaluation/state_machine_test.go`

- [ ] **Step 1: 先写测试文件**

```go
package evaluation

import (
    "testing"
    ev "mooc-manus/internal/domains/models/evaluation"
)

func TestTransitInstance(t *testing.T) {
    cases := []struct {
        from, to ev.InstanceStatus
        wantErr  bool
    }{
        // 合法
        {ev.InstanceStatusPending, ev.InstanceStatusQueued, false},
        {ev.InstanceStatusQueued, ev.InstanceStatusInitializing, false},
        {ev.InstanceStatusInitializing, ev.InstanceStatusRunning, false},
        {ev.InstanceStatusInitializing, ev.InstanceStatusFailed, false},
        {ev.InstanceStatusRunning, ev.InstanceStatusVerifying, false},
        {ev.InstanceStatusRunning, ev.InstanceStatusFailed, false},
        {ev.InstanceStatusRunning, ev.InstanceStatusTimeout, false},
        {ev.InstanceStatusVerifying, ev.InstanceStatusPassed, false},
        {ev.InstanceStatusVerifying, ev.InstanceStatusFailed, false},
        {ev.InstanceStatusInitializing, ev.InstanceStatusTimeout, false}, // 巡检器
        {ev.InstanceStatusVerifying, ev.InstanceStatusTimeout, false},
        {ev.InstanceStatusFailed, ev.InstanceStatusPending, false},  // 重试
        {ev.InstanceStatusTimeout, ev.InstanceStatusPending, false}, // 重试
        // 非法
        {ev.InstanceStatusPending, ev.InstanceStatusRunning, true},
        {ev.InstanceStatusPassed, ev.InstanceStatusFailed, true},
        {ev.InstanceStatusPassed, ev.InstanceStatusPending, true}, // Passed 不允许重试
    }
    for _, c := range cases {
        err := TransitInstance(c.from, c.to)
        if (err != nil) != c.wantErr {
            t.Errorf("Transit(%s→%s) got err=%v want err=%v", c.from, c.to, err, c.wantErr)
        }
    }
}

func TestTransitTask(t *testing.T) {
    cases := []struct {
        from, to ev.TaskStatus
        wantErr  bool
    }{
        {ev.TaskStatusPending, ev.TaskStatusRunning, false},
        {ev.TaskStatusPending, ev.TaskStatusSucceeded, false},         // edge case: 全部瞬间终态
        {ev.TaskStatusPending, ev.TaskStatusPartialFailed, false},
        {ev.TaskStatusRunning, ev.TaskStatusSucceeded, false},
        {ev.TaskStatusRunning, ev.TaskStatusPartialFailed, false},
        {ev.TaskStatusSucceeded, ev.TaskStatusRunning, false},        // 重试
        {ev.TaskStatusPartialFailed, ev.TaskStatusRunning, false},    // 重试
        {ev.TaskStatusPending, ev.TaskStatusPending, true},           // 自环非法
        {ev.TaskStatusSucceeded, ev.TaskStatusPending, true},
    }
    for _, c := range cases {
        err := TransitTask(c.from, c.to)
        if (err != nil) != c.wantErr {
            t.Errorf("TransitTask(%s→%s) got err=%v want err=%v", c.from, c.to, err, c.wantErr)
        }
    }
}
```

- [ ] **Step 2: 运行测试确认 fail**

```bash
go test ./internal/domains/services/evaluation/ -run TestTransit -v
```

Expected: `undefined: TransitInstance` / `undefined: TransitTask`

- [ ] **Step 3: 实现 state_machine.go**

```go
package evaluation

import (
    "fmt"
    ev "mooc-manus/internal/domains/models/evaluation"
)

var instanceWhitelist = map[ev.InstanceStatus]map[ev.InstanceStatus]bool{
    ev.InstanceStatusPending:      {ev.InstanceStatusQueued: true},
    ev.InstanceStatusQueued:       {ev.InstanceStatusInitializing: true},
    ev.InstanceStatusInitializing: {
        ev.InstanceStatusRunning: true, ev.InstanceStatusFailed: true, ev.InstanceStatusTimeout: true,
    },
    ev.InstanceStatusRunning: {
        ev.InstanceStatusVerifying: true, ev.InstanceStatusFailed: true, ev.InstanceStatusTimeout: true,
    },
    ev.InstanceStatusVerifying: {
        ev.InstanceStatusPassed: true, ev.InstanceStatusFailed: true, ev.InstanceStatusTimeout: true,
    },
    ev.InstanceStatusFailed:  {ev.InstanceStatusPending: true},
    ev.InstanceStatusTimeout: {ev.InstanceStatusPending: true},
}

func TransitInstance(from, to ev.InstanceStatus) error {
    if allowed, ok := instanceWhitelist[from]; ok && allowed[to] {
        return nil
    }
    return fmt.Errorf("非法实例流转: %s → %s", from, to)
}

var taskWhitelist = map[ev.TaskStatus]map[ev.TaskStatus]bool{
    ev.TaskStatusPending: {
        ev.TaskStatusRunning: true, ev.TaskStatusSucceeded: true, ev.TaskStatusPartialFailed: true,
    },
    ev.TaskStatusRunning: {
        ev.TaskStatusSucceeded: true, ev.TaskStatusPartialFailed: true,
    },
    ev.TaskStatusSucceeded:     {ev.TaskStatusRunning: true},
    ev.TaskStatusPartialFailed: {ev.TaskStatusRunning: true},
}

func TransitTask(from, to ev.TaskStatus) error {
    if allowed, ok := taskWhitelist[from]; ok && allowed[to] {
        return nil
    }
    return fmt.Errorf("非法任务流转: %s → %s", from, to)
}
```

- [ ] **Step 4: 跑测试**

```bash
go test ./internal/domains/services/evaluation/ -run TestTransit -v
```

Expected: PASS (两个测试全绿)

- [ ] **Step 5: Commit**

```bash
git add internal/domains/services/evaluation/state_machine*.go
git commit -m "feat(eval): 双状态机白名单函数 + 穷举单测 (M2.1)"
```

---

### Task 2.2: EvaluationDomainService 接口骨架

**Files:**
- Create: `internal/domains/services/evaluation/service.go`
- Create: `internal/domains/services/evaluation/service_impl.go`（先只留构造函数与空方法体，具体实现在 M4-M7 各任务里填）

- [ ] **Step 1: 定义 Service 接口**

```go
type EvaluationDomainService interface {
    // 用例
    CreateCase(ctx context.Context, c *ev.Case) (*ev.Case, error)
    UpdateCase(ctx context.Context, c *ev.Case) (*ev.Case, error)
    DeleteCase(ctx context.Context, id string) error
    ListCases(ctx context.Context, filter repo.CaseListFilter, page repo.Page) ([]*ev.Case, int64, error)
    GetCase(ctx context.Context, id string) (*ev.Case, error)

    // 任务
    CreateTask(ctx context.Context, name string, caseIDs, agentConfigIDs []string) (*ev.Task, error)
    ListTasks(ctx context.Context, filter repo.TaskListFilter, page repo.Page) ([]*ev.Task, int64, error)
    GetTask(ctx context.Context, id string) (*ev.Task, error)
    RetryTaskFailedInstances(ctx context.Context, id string) (int, error)
    DeleteTask(ctx context.Context, id string) error

    // 实例
    ListInstances(ctx context.Context, taskID string, filter repo.InstanceListFilter, page repo.Page) ([]*ev.RunInstance, int64, error)
    GetInstance(ctx context.Context, id string) (*ev.RunInstance, error)
    RetryInstance(ctx context.Context, id string) error
    DeleteInstance(ctx context.Context, id string) error

    // Worker 入口
    ExecuteInstance(ctx context.Context, instanceID string, workerID string) error

    // 巡检
    SweepStaleInstances(ctx context.Context) (int, error)
    ReconcileTaskStatuses(ctx context.Context) (int, error)
    ArchiveDeadTasks(ctx context.Context) (int, error)
}
```

- [ ] **Step 2: 写 `service_impl.go` 构造函数**

```go
type serviceImpl struct {
    caseRepo     repo.EvalCaseRepository
    taskRepo     repo.EvalTaskRepository
    instanceRepo repo.EvalRunInstanceRepository
    resultRepo   repo.EvalResultRepository
    snapshotRepo repo.EvalAgentSnapshotRepository
    // M4 后期注入：executor, aggregator, verifier, chatRunner
    // M5 后期注入：mqClient
}

func NewEvaluationDomainService(...) EvaluationDomainService { return &serviceImpl{...} }
```

方法体先全部 `return nil, errors.New("not implemented in M2")`；编译通过即可。

- [ ] **Step 3: Commit**

```bash
git add internal/domains/services/evaluation/service*.go
git commit -m "feat(eval): EvaluationDomainService 接口骨架 (M2.2)"
```

---

## M3 · Invoker Usage 补强（破坏性变更）

### Task 3.1: 新增 Usage 值对象 + 脱敏正则守护测试

**Files:**
- Create: `internal/domains/models/llm/usage.go`
- Create: `internal/domains/models/tracing/span_usage_masking_test.go`

- [ ] **Step 1: 写 `usage.go`**

```go
package llm

type Usage struct {
    PromptTokens     int64
    CompletionTokens int64
    TotalTokens      int64
}
```

- [ ] **Step 2: 写脱敏正则守护测试（先跑一次确认现状）**

```go
package tracing

import (
    "testing"
    "github.com/stretchr/testify/require"
)

// 确保 llm.usage.* 系列 tag 不被脱敏正则误杀。
// 若此测试 fail，需退化命名为 llm.io.*_units（见 spec §4.2 风险清单）。
func TestUsageTagNotMasked(t *testing.T) {
    s := newTestSpan("t", 1, 0, SpanTypeLLMCall, "")
    s.SetTag("llm.usage.prompt_tokens", int64(123))
    s.SetTag("llm.usage.completion_tokens", int64(456))
    s.SetTag("llm.usage.total_tokens", int64(579))
    require.Equal(t, int64(123), s.Tags["llm.usage.prompt_tokens"])
    require.Equal(t, int64(456), s.Tags["llm.usage.completion_tokens"])
    require.Equal(t, int64(579), s.Tags["llm.usage.total_tokens"])
}
```

- [ ] **Step 3: 跑测试锁定 tag key 命名**

```bash
go test ./internal/domains/models/tracing/... -run TestUsageTagNotMasked -v
```

- **Expected**: PASS → 沿用 `llm.usage.*`；
- **Fallback**: 若脱敏正则命中，将 tag key 改成 `llm.io.prompt_units / completion_units / total_units`，重跑测试直到 PASS，并在 spec §4.2 追加一行"实施阶段确认 tag key 已改为 llm.io.*"。

- [ ] **Step 4: Commit**

```bash
git add internal/domains/models/llm/usage.go internal/domains/models/tracing/span_usage_masking_test.go
git commit -m "feat(llm): 新增 Usage 值对象 + 脱敏正则守护测试 (M3.1)"
```

---

### Task 3.2.a: 底层 SDK 层暴露 Usage（不改 Domain 接口，可编译）

**PlanReview-Blk6 说明**：一次性改接口 + 5 处调用 + 全项目 mock 太大，中断后留半成品。分 3 步走。

**Files:**
- Modify: `internal/infra/external/llm/openai.go`
- Modify: `internal/infra/external/llm/anthropic.go`（若存在，或对应文件名）
- Modify: `internal/infra/external/llm/openai_adapter.go`
- Modify: `internal/infra/external/llm/anthropic_adapter.go`

- [ ] **Step 1: 底层 SDK 包装器返回 SDK 原生 Usage 类型**

（infra 层，直接用 SDK 类型无需 domain 映射）

```go
// openai.go
func (l *OpenAiLLM) StreamingInvoke(msgs, tools) (openai.ChatCompletionMessage, openai.CompletionUsage) {
    acc := openai.ChatCompletionAccumulator{}
    // ...原逻辑不变...
    return acc.Choices[0].Message, acc.Usage
}
func (l *OpenAiLLM) Invoke(msgs, tools) (openai.ChatCompletionMessage, openai.CompletionUsage, error) {
    resp, err := l.client.ChatCompletion(...)
    if err != nil { return openai.ChatCompletionMessage{}, openai.CompletionUsage{}, err }
    return resp.Choices[0].Message, resp.Usage, nil
}
```

Anthropic 同理，SDK 有 `Usage.InputTokens/OutputTokens`。

- [ ] **Step 2: Adapter 内部先把 Usage 记在实例字段（不改 Adapter 对外接口）**

```go
// openai_adapter.go
type OpenAIAdapter struct {
    llm         *OpenAiLLM
    lastUsage   domainllm.Usage  // 新增：最近一次调用的 usage，供上层 getter 读取
    lastUsageMu sync.Mutex
}

func (a *OpenAIAdapter) StreamingInvoke(msgs []domainllm.Message, tools []domainllm.Tool, eventCh chan<- events.AgentEvent) domainllm.Message {
    resp, sdkUsage := a.llm.StreamingInvoke(toSDKMsgs(msgs), toSDKTools(tools))
    a.lastUsageMu.Lock()
    a.lastUsage = domainllm.Usage{
        PromptTokens:     sdkUsage.PromptTokens,
        CompletionTokens: sdkUsage.CompletionTokens,
        TotalTokens:      sdkUsage.TotalTokens,
    }
    a.lastUsageMu.Unlock()
    return fromSDKMessage(resp)
}

// 新增 getter，方便 base.go 在同一 goroutine 内取
func (a *OpenAIAdapter) LastUsage() domainllm.Usage {
    a.lastUsageMu.Lock(); defer a.lastUsageMu.Unlock()
    return a.lastUsage
}
```

Anthropic Adapter 加同样字段与 getter。

- [ ] **Step 3: `go build ./... && go test ./internal/...` 全绿** —— 因为 Adapter 对外接口未变，旧 mock 与调用点无需改动。

- [ ] **Step 4: Commit**

```bash
git commit -am "refactor(llm): Adapter 内部持有最近一次调用 Usage (M3.2.a)"
```

---

### Task 3.2.b: 扩 Invoker 接口签名 + 平推 base.go 与 mock

**Files:**
- Modify: `internal/domains/models/invoker/invoker.go`
- Modify: `internal/infra/external/llm/openai_adapter.go`
- Modify: `internal/infra/external/llm/anthropic_adapter.go`
- Modify: `internal/domains/services/agents/base.go`
- Modify: 所有 mock（`grep -RIn "invoker.Invoker" internal/ --include="*_test.go"`）

- [ ] **Step 1: 改 `invoker.go` 接口签名，加 Usage 返回**

```go
type Invoker interface {
    Invoke(messages []llm.Message, tools []llm.Tool) (llm.Message, llm.Usage, error)
    StreamingInvoke(messages []llm.Message, tools []llm.Tool, eventCh chan<- events.AgentEvent) (llm.Message, llm.Usage)
}
```

- [ ] **Step 2: 把 Adapter 的 getter 直接改成接口返回**

```go
func (a *OpenAIAdapter) StreamingInvoke(...) (domainllm.Message, domainllm.Usage) {
    // ...原有逻辑写 lastUsage 后...
    return fromSDKMessage(resp), a.lastUsage
}
```

删掉 `LastUsage()` getter（YAGNI，已换成正式接口）。

- [ ] **Step 3: 改 `base.go` 两处调用点**

`StreamingInvokeLLM` (`base.go:552` 附近；执行前先 `grep -n "StreamingInvoke" internal/domains/services/agents/base.go` 定位实际行号)：

```go
var invokerUsage llm.Usage
go func() {
    msg, usage := a.invoker.StreamingInvoke(a.GetMessages(), availableTools, llmEventCh)
    message = msg
    invokerUsage = usage
    // ...原逻辑不变...
}()
// 收敛后：
if streamErr != "" {
    a.recordLLMStreamError(llmSpan, streamErr)
} else {
    a.finalizeLLMSpanSuccess(llmSpan, len(message.ToolCalls), invokerUsage)
}
```

`InvokeLLM`：`msg, usage, err := a.invoker.Invoke(...)`；成功时 `finalizeLLMSpanSuccess(llmSpan, len(msg.ToolCalls), usage)`。

- [ ] **Step 4: 一次性平推所有 mock**

```bash
grep -RIn "invoker.Invoker\|StreamingInvoke\|Invoke(" internal/ --include="*_test.go"
```

每处 mock 补返回值：非流 `return llm.Message{...}, llm.Usage{}, nil`；流式 `return llm.Message{...}, llm.Usage{}`。

- [ ] **Step 5: 全量 build + test**

```bash
go build ./...
go test ./internal/... -count=1
```

Expected: 全绿。若有旧测试挂了，就是签名平推漏了，逐个补。此步不允许留半成品。

- [ ] **Step 6: Commit**

```bash
git commit -am "refactor(llm): Invoker 接口扩容 Usage 返回 + 平推 base.go/mock (M3.2.b)"
```

---

### Task 3.2.c: base.go 写入 llm.usage.* span tag + 集成测

**Files:**
- Modify: `internal/domains/services/agents/base.go`（改 `finalizeLLMSpanSuccess` 签名）
- Modify/Create: `internal/applications/services/agent_tracing_integration_test.go`

- [ ] **Step 1: 写失败集成测**

mock Invoker 返回 `Usage{Prompt:100, Completion:50, Total:150}`；跑完从 `AiSpanPO.Tags` 里读三 tag 键值，断言 == 100/50/150；此刻应 FAIL 因为 base.go 未写 tag。

- [ ] **Step 2: 改 `finalizeLLMSpanSuccess`**

```go
func (a *BaseAgent) finalizeLLMSpanSuccess(llmSpan *tracing.Span, toolCallsCount int, usage llm.Usage) {
    llmSpan.SetTag("llm.tool_calls_count", toolCallsCount)
    llmSpan.SetTag("llm.usage.prompt_tokens", usage.PromptTokens)
    llmSpan.SetTag("llm.usage.completion_tokens", usage.CompletionTokens)
    llmSpan.SetTag("llm.usage.total_tokens", usage.TotalTokens)
    llmSpan.AddLog("INFO", "llm.stream.completed", nil)
}
```

- [ ] **Step 3: 跑集成测 → PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(llm): LLM_CALL span 写入 llm.usage.* + 集成测 (M3.2.c)"
```

---

## M4 · 核心执行链路（InstanceExecutor / VerifyRunner / TraceAggregator / Snapshot）

### Task 4.1: Snapshot 冻结逻辑

**Files:**
- Create: `internal/domains/services/evaluation/snapshot.go`
- Test: `internal/domains/services/evaluation/snapshot_test.go`

- [ ] **Step 1: 写测试** —— 给定一个 `appConfig` 输入，`FreezeAppConfig(cfg) *ev.AgentSnapshot` 输出应把 `Model / SystemPrompt / ToolsConfig / MCPConfig / A2AConfig` 逐字段深拷贝到 snapshot；断言修改源 cfg 后 snapshot 不变（深拷贝语义）。

- [ ] **Step 2: 实现 `FreezeAppConfig`** —— 用 `json.Marshal/Unmarshal` 走一遍是最简单的深拷贝方式；`SourceAppConfigID` 回填源 id。

- [ ] **Step 3: 跑测试 → PASS**

```bash
go test ./internal/domains/services/evaluation/ -run TestFreeze -v
```

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): Snapshot 冻结逻辑 + 深拷贝单测 (M4.1)"
```

---

### Task 4.2: VerifyRunner

**Files:**
- Create: `internal/domains/services/evaluation/verify_runner.go`
- Test: `internal/domains/services/evaluation/verify_runner_test.go`

- [ ] **Step 1: 写测试覆盖 4 分支**：exit=0 / exit≠0 / 超时 / stdout 截断至 64KB

```go
func TestVerifyRunner_ExitZero(t *testing.T) {
    r := NewVerifyRunner(60 * time.Second, 64<<10)
    dir := t.TempDir()
    got, err := r.Run(context.Background(), dir, `#!/bin/bash
echo hello
exit 0`)
    require.NoError(t, err)
    assert.Equal(t, 0, got.ExitCode)
    assert.Equal(t, "hello\n", got.Stdout)
}
```

其余分支：`exit 3` 断言 ExitCode=3；`sleep 5` + timeout=1s 断言 err 含 "context deadline exceeded"；stdout 64KB+1 断言截断 + 尾巴含 `[truncated]`。

- [ ] **Step 2: 实现 VerifyRunner**

```go
type VerifyResult struct {
    ExitCode int
    Stdout, Stderr string
    Duration time.Duration
}

type VerifyRunner struct {
    timeout time.Duration
    cap     int
}

func (r *VerifyRunner) Run(ctx context.Context, workdir, script string) (*VerifyResult, error) {
    // 写 .verify.sh (0700)
    path := filepath.Join(workdir, ".verify.sh")
    if err := os.WriteFile(path, []byte(script), 0700); err != nil { return nil, err }

    ctx2, cancel := context.WithTimeout(ctx, r.timeout)
    defer cancel()
    cmd := exec.CommandContext(ctx2, "bash", path)
    cmd.Dir = workdir
    // PlanReview-Blk7 修复 spec §11 风险 6：白名单 env，不继承宿主机（cmd.Env=nil 会继承所有环境变量）
    cmd.Env = []string{
        "PATH=" + os.Getenv("PATH"),
        "HOME=" + os.Getenv("HOME"),
        "LANG=C.UTF-8",
    }

    var outBuf, errBuf capBuffer
    outBuf.limit = r.cap; errBuf.limit = r.cap
    cmd.Stdout = &outBuf; cmd.Stderr = &errBuf

    start := time.Now()
    runErr := cmd.Run()
    res := &VerifyResult{
        Stdout: outBuf.String(), Stderr: errBuf.String(),
        Duration: time.Since(start),
    }
    if ee, ok := runErr.(*exec.ExitError); ok {
        res.ExitCode = ee.ExitCode()
        return res, nil
    }
    if ctx2.Err() != nil { return res, ctx2.Err() }
    return res, runErr
}
```

`capBuffer` 是自定义带上限的 `io.Writer`，超过 cap 时截断 + append `\n[truncated]`。

- [ ] **Step 3: 跑测试 → PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): VerifyRunner + 4 分支单测 (M4.2)"
```

---

### Task 4.3: TraceAggregator

**Files:**
- Create: `internal/domains/services/evaluation/trace_aggregator.go`
- Test: `internal/domains/services/evaluation/trace_aggregator_test.go`

- [ ] **Step 1: 写测试** —— 3 场景：无 AGENT_ROOT / 多 AGENT_ROOT / LLM_CALL 缺 usage tag。

- [ ] **Step 2: 实现**

```go
type Metrics struct {
    AgentLatencyMs int64
    PromptTokens, CompletionTokens, TotalTokens int64
    TraceID string
    Degraded bool // 缺 root 时为 true
}

func (a *TraceAggregator) Aggregate(ctx context.Context, conversationID string) (*Metrics, error) {
    spans, err := a.spanRepo.ListByConversation(ctx, conversationID)
    if err != nil { return nil, err }
    m := &Metrics{}
    rootCount := 0
    for _, s := range spans {
        switch s.SpanType {
        case tracing.SpanTypeAgentRoot:
            rootCount++
            if s.LatencyMs > m.AgentLatencyMs { m.AgentLatencyMs = s.LatencyMs }
            m.TraceID = s.TraceID
        case tracing.SpanTypeLLMCall:
            m.PromptTokens     += tagInt64(s.Tags, "llm.usage.prompt_tokens")
            m.CompletionTokens += tagInt64(s.Tags, "llm.usage.completion_tokens")
            m.TotalTokens      += tagInt64(s.Tags, "llm.usage.total_tokens")
        }
    }
    if rootCount == 0 { m.Degraded = true }
    if rootCount > 1  { a.logger.Warn("多个 AGENT_ROOT", zap.String("conv", conversationID)) }
    return m, nil
}
```

- [ ] **Step 3: 跑测试 → PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): TraceAggregator + 3 分支单测 (M4.3)"
```

---

### Task 4.4: InternalChatRunner（内部 eventCh close 阻塞）

**Files:**
- Create: `internal/domains/services/evaluation/internal_chat_runner.go`
- Test: `internal/domains/services/evaluation/internal_chat_runner_test.go`

- [ ] **Step 1: 定义接口**

```go
type InternalChatRunner interface {
    Run(ctx context.Context, req InternalChatReq) (InternalChatResult, error)
}

type InternalChatReq struct {
    Snapshot       *ev.AgentSnapshot
    ConversationID string
    MessageID      string
    Query          string
    TotalTimeout   time.Duration
}

type InternalChatResult struct {
    Error      error   // 智能体自身报错
    LastAssistantMsg string
    DidTimeout bool
}
```

- [ ] **Step 2: 先落 snapshot 桥接（PlanReview-Blk4 修复）**

评测必须以 snapshot 为准，不能走 appConfig 活对象。做法：`AgentSnapshot` 加方法 `ToAppConfig() *appconfig.AppConfig`，把 snapshot 5 大字段拷贝成一个瞬态 AppConfig 值对象。在 `BaseAgentDomainServiceImpl.createBaseAgent(request)` 内的入口处加一个可选注入路径：

```go
// internal/domains/services/agents/agent.go
type ChatRequest struct {
    // ... 原字段 ...
    ConfigOverride *appconfig.AppConfig // 新增，非空时优先于 AppConfigId 查询
}
```

在 `createBaseAgent` 里检查 `if request.ConfigOverride != nil { cfg = request.ConfigOverride }`，否则走原有 `AppConfigRepository.Get(request.AppConfigId)`。这一改动是**评测能否可回溯**的核心保证。

写单测：`TestBaseAgent_ConfigOverride_Wins`，断言 override 优先于 AppConfigId。

- [ ] **Step 3: 实现 InternalChatRunner —— 复用 BaseAgentDomainService.Chat**

```go
type internalChatRunnerImpl struct {
    baseAgent agents.BaseAgentDomainService
}

func (r *internalChatRunnerImpl) Run(ctx context.Context, req InternalChatReq) (InternalChatResult, error) {
    ctx2, cancel := context.WithTimeout(ctx, req.TotalTimeout)
    defer cancel()

    eventCh := make(chan events.AgentEvent, 64)
    chatReq := agentmdl.ChatRequest{
        Streaming:      true,
        SystemPrompt:   req.Snapshot.SystemPrompt,
        ConversationId: req.ConversationID,
        MessageId:      req.MessageID,
        Query:          req.Query,
        AppConfigId:    req.Snapshot.SourceAppConfigID,       // 仅作追溯记录
        ConfigOverride: req.Snapshot.ToAppConfig(),           // ← 评测语义关键：以 snapshot 为准
    }

    // Chat 无返回值，异步跑；由内部 close(eventCh) 收敛
    go r.baseAgent.Chat(ctx2, chatReq, eventCh)

    var lastMsg string
    var errFromStream error
    for {
        select {
        case <-ctx2.Done():
            return InternalChatResult{DidTimeout: true}, nil
        case ev, ok := <-eventCh:
            if !ok {
                return InternalChatResult{Error: errFromStream, LastAssistantMsg: lastMsg}, nil
            }
            switch v := ev.(type) {
            case *events.MessageEvent:
                lastMsg = v.Content
            case *events.ErrorEvent:
                errFromStream = errors.New(v.Error)
            }
        }
    }
}
```

**签名/字段核对清单**（进 build 前 grep 一次）：
- `agents.BaseAgentDomainService.Chat(ctx, ChatRequest, chan events.AgentEvent)` 无返回值
- `ChatRequest` 字段名以 `internal/domains/models/agents/base.go:14` 为准（`ConversationId` 而非 `ConversationID`，注意大小写）
- `events.MessageEvent` / `events.ErrorEvent` 具体类型名以 `internal/domains/models/events/*.go` 为准

- [ ] **Step 3: 写测试** —— mock `baseAgent` 输出 3 个事件后 close(eventCh)，验证 runner 正常返回；再造一个 mock "永不 close"，验证 15min 后 DidTimeout=true。

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): InternalChatRunner 复用 BaseAgent + eventCh close 阻塞 (M4.4)"
```

---

### Task 4.5.1: InstanceExecutor 骨架 + 状态 CAS 环节（TDD）

**Files:**
- Create: `internal/domains/services/evaluation/executor.go`
- Test: `internal/domains/services/evaluation/executor_cas_test.go`

**PlanReview-Blk5 说明**：原 4.5 一次落 12 步太大。拆成 4 个子任务，每个专注一环。

- [ ] **Step 1: 写失败测试** —— 用 mock instRepo，验证 Execute(ctx, id) 在 QUEUED 状态下先做 CAS QUEUED→INITIALIZING；CAS 失败返回 nil（跳过而非 error）。

- [ ] **Step 2: 实现 struct + 构造函数 + CAS 环节**

```go
type InstanceExecutor struct {
    instRepo      repositories.EvalRunInstanceRepository
    taskRepo      repositories.EvalTaskRepository
    resultRepo    repositories.EvalResultRepository
    verifyRunner  *VerifyRunner
    chatRunner    InternalChatRunner
    aggregator    *TraceAggregator
    tracer        tracing.Tracer
    skillExecutor tools.SkillExecutor    // 拿 CleanupMessage（签名: CleanupMessage(messageID string) error）
    workspaceRoot string                 // 拼 workdir 用
    workerID      string
    heartbeatInterval time.Duration
    logger        *zap.Logger
}

func (e *InstanceExecutor) Execute(ctx context.Context, instanceID string) error {
    ok, err := e.instRepo.CASStatus(ctx, instanceID, ev.InstanceStatusQueued, ev.InstanceStatusInitializing)
    if err != nil { return err }
    if !ok { e.logger.Warn("CAS QUEUED->INITIALIZING 失败", zap.String("id", instanceID)); return nil }
    // 后续在 4.5.2 填
    return nil
}
```

- [ ] **Step 3: 跑测试 → PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): InstanceExecutor 骨架 + CAS 环节 + 单测 (M4.5.1)"
```

---

### Task 4.5.2: 心跳 + init_script 阶段

**Files:**
- Modify: `internal/domains/services/evaluation/executor.go`
- Test: `internal/domains/services/evaluation/executor_init_test.go`

- [ ] **Step 1: 写测试** —— init_script 非空且 exit=0 → status 转 RUNNING；exit≠0 → status 转 FAILED；error_log 含 stderr 前 4KB。

- [ ] **Step 2: 实现 `startHeartbeat` 与 init 阶段**（PlanReview-Blk5 空指针修复：先判 `r != nil`）

```go
func (e *InstanceExecutor) Execute(ctx context.Context, instanceID string) error {
    if ok, _ := e.instRepo.CASStatus(ctx, instanceID, ev.InstanceStatusQueued, ev.InstanceStatusInitializing); !ok {
        return nil
    }
    inst, err := e.instRepo.GetByID(ctx, instanceID)
    if err != nil { return err }

    // 心跳 goroutine 与自监听
    stopHB := e.startHeartbeat(ctx, instanceID)
    defer stopHB()

    workdir := filepath.Join(e.workspaceRoot, inst.ConversationID, inst.MessageID)
    if err := os.MkdirAll(workdir, 0700); err != nil {
        e.finalize(ctx, inst, ev.InstanceStatusFailed, "mkdir workspace: "+err.Error(), nil, nil)
        return nil
    }

    // init_script
    if inst.CaseSnapshot.InitScript != "" {
        r, rerr := e.verifyRunner.Run(ctx, workdir, inst.CaseSnapshot.InitScript)
        stderr := ""
        if r != nil { stderr = r.Stderr }
        if rerr != nil {
            e.finalize(ctx, inst, ev.InstanceStatusFailed, "init_script run: "+rerr.Error()+"; stderr="+stderr, nil, nil)
            return nil
        }
        if r.ExitCode != 0 {
            e.finalize(ctx, inst, ev.InstanceStatusFailed, "init_script exit="+strconv.Itoa(r.ExitCode)+"; stderr="+stderr, nil, nil)
            return nil
        }
    }

    // INITIALIZING -> RUNNING
    if ok, _ := e.instRepo.CASStatus(ctx, instanceID, ev.InstanceStatusInitializing, ev.InstanceStatusRunning); !ok {
        return nil
    }
    // 4.5.3 继续
    return nil
}
```

心跳 goroutine：

```go
func (e *InstanceExecutor) startHeartbeat(ctx context.Context, id string) context.CancelFunc {
    ctx2, cancel := context.WithCancel(ctx)
    go func() {
        t := time.NewTicker(e.heartbeatInterval)
        defer t.Stop()
        for {
            select {
            case <-ctx2.Done(): return
            case <-t.C:
                _ = e.instRepo.UpdateHeartbeat(ctx, id, e.workerID, time.Now())
                if s, _ := e.instRepo.GetStatus(ctx, id); s == ev.InstanceStatusTimeout || s == ev.InstanceStatusCanceled {
                    cancel()
                    return
                }
            }
        }
    }()
    return cancel
}
```

- [ ] **Step 3: 跑测试 → PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): InstanceExecutor 心跳 + init_script (M4.5.2)"
```

---

### Task 4.5.3: chat + verify 阶段

**Files:**
- Modify: `internal/domains/services/evaluation/executor.go`
- Test: `internal/domains/services/evaluation/executor_chat_verify_test.go`

- [ ] **Step 1: 写测试** —— mock chatRunner 返回 3 分支：正常 / DidTimeout=true / Error≠nil；mock verifyRunner 返回 exit=0 / exit=1；组合断言状态机走向。

- [ ] **Step 2: 补齐主流程**

```go
// 承接 4.5.2 尾部
res, err := e.chatRunner.Run(ctx, InternalChatReq{
    Snapshot:       inst.CaseSnapshot.AgentSnapshot,  // instance 里冻结的 snapshot
    ConversationID: inst.ConversationID,
    MessageID:      inst.MessageID,
    Query:          inst.CaseSnapshot.TaskPrompt,
    TotalTimeout:   e.chatTimeout,
})
if err != nil {
    e.finalize(ctx, inst, ev.InstanceStatusFailed, "chat runner: "+err.Error(), nil, nil)
    return nil
}
if res.DidTimeout {
    e.finalize(ctx, inst, ev.InstanceStatusTimeout, "agent_chat_timeout", nil, nil)
    return nil
}
if res.Error != nil {
    e.finalize(ctx, inst, ev.InstanceStatusFailed, "agent_error: "+res.Error.Error(), nil, nil)
    return nil
}

// RUNNING -> VERIFYING
if ok, _ := e.instRepo.CASStatus(ctx, instanceID, ev.InstanceStatusRunning, ev.InstanceStatusVerifying); !ok {
    return nil
}

vres, verr := e.verifyRunner.Run(ctx, workdir, inst.CaseSnapshot.VerifyScript)
passed := verr == nil && vres != nil && vres.ExitCode == 0

// 见 4.5.4 finalize
e.finalizeVerify(ctx, inst, passed, vres, verr)
return nil
```

- [ ] **Step 3: 跑测试 → PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): InstanceExecutor chat + verify 分支 (M4.5.3)"
```

---

### Task 4.5.4: result 落库 + 状态推进 + cleanup

**Files:**
- Modify: `internal/domains/services/evaluation/executor.go`
- Test: `internal/domains/services/evaluation/executor_finalize_test.go`

- [ ] **Step 1: 写测试** —— PASSED 分支：断言 result.passed=true、token 三字段>0、trace_id 回填、task.RecountAndTransit 被调；FAILED 分支：error_log 截断在 64KB。

- [ ] **Step 2: 实现 finalize 系列**

```go
func (e *InstanceExecutor) finalize(ctx context.Context, inst *ev.RunInstance, target ev.InstanceStatus, errMsg string, vres *VerifyResult, metrics *Metrics) {
    // CAS 到目标态
    _, _ = e.instRepo.CASStatus(ctx, inst.ID, inst.Status, target)

    // 写 result（截断 error_log 到 64KB）
    r := &ev.Result{
        InstanceID: inst.ID,
        Passed:     target == ev.InstanceStatusPassed,
        FinishedAt: time.Now(),
    }
    if vres != nil {
        r.VerifyExitCode = vres.ExitCode
        r.VerifyStdout   = truncate(vres.Stdout, 64<<10)
        r.VerifyStderr   = truncate(vres.Stderr, 64<<10)
    }
    if metrics != nil {
        r.PromptTokens     = metrics.PromptTokens
        r.CompletionTokens = metrics.CompletionTokens
        r.TotalTokens      = metrics.TotalTokens
        r.AgentLatencyMs   = metrics.AgentLatencyMs
        _ = e.instRepo.UpdateTraceID(ctx, inst.ID, metrics.TraceID) // 收敛后一次性回填（spec §2.3）
    }
    r.ErrorLog = truncate(errMsg, 64<<10)
    _ = e.resultRepo.Create(ctx, r)

    // 推进 task 计数与状态
    _ = e.taskRepo.RecountAndTransit(ctx, inst.TaskID)

    // 清工作区（signature: CleanupMessage(messageID string) error —— 单参！）
    _ = e.skillExecutor.CleanupMessage(inst.MessageID)
}

func (e *InstanceExecutor) finalizeVerify(ctx context.Context, inst *ev.RunInstance, passed bool, vres *VerifyResult, verr error) {
    // tracer flush + 聚合
    _ = e.tracer.Flush()
    time.Sleep(200 * time.Millisecond)
    metrics, _ := e.aggregator.Aggregate(ctx, inst.ConversationID)

    target := ev.InstanceStatusPassed
    errMsg := ""
    if !passed {
        target = ev.InstanceStatusFailed
        if verr != nil {
            errMsg = "verify: " + verr.Error() // 含 60s 超时也走这里 → FAILED（spec §3.3 R2-N4）
        } else if vres != nil {
            errMsg = firstNonEmpty(vres.Stderr, "verify_exit_"+strconv.Itoa(vres.ExitCode))
        }
    }
    e.finalize(ctx, inst, target, errMsg, vres, metrics)
}

func truncate(s string, n int) string {
    if len(s) <= n { return s }
    return s[:n] + "\n[truncated]"
}
```

- [ ] **Step 3: 集成测**：sqlite in-memory + mock InternalChatRunner + 真实 VerifyRunner，跑完整 PENDING → PASSED 生命周期，断言 4 个 count 一致、result 存在、trace_id 回填。

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): InstanceExecutor finalize + 集成测 (M4.5.4)"
```

---

## M5 · Asynq 基础设施

### Task 5.1: 引入 Asynq 依赖 + 配置项

**Files:**
- Modify: `go.mod` / `go.sum`
- Modify: `config/config.go` / `config/config.toml`

- [ ] **Step 1: 引入依赖**

```bash
go get github.com/hibiken/asynq@latest
```

- [ ] **Step 2: 补 config 结构体（对齐 spec §7.2）**

在 `config.go` 加：

```go
type EvaluationConfig struct {
    Enabled                     bool  `mapstructure:"enabled"`
    WorkerConcurrencyDefault    int   `mapstructure:"worker_concurrency_default"`
    WorkerConcurrencyHigh       int   `mapstructure:"worker_concurrency_high"`
    CaseConcurrencyLimit        int   `mapstructure:"case_concurrency_limit"`
    InstanceTotalTimeoutSec     int   `mapstructure:"instance_total_timeout_sec"`
    VerifyScriptTimeoutSec      int   `mapstructure:"verify_script_timeout_sec"`
    VerifyOutputCapBytes        int   `mapstructure:"verify_output_cap_bytes"`
    HeartbeatIntervalSec        int   `mapstructure:"heartbeat_interval_sec"`
    HeartbeatStaleThresholdSec  int   `mapstructure:"heartbeat_stale_threshold_sec"`
    CronSweeperIntervalSec      int   `mapstructure:"cron_sweeper_interval_sec"`
    CronReconcilerIntervalSec   int   `mapstructure:"cron_reconciler_interval_sec"`
    CronDLQArchiveIntervalSec   int   `mapstructure:"cron_dlq_archive_interval_sec"`
    UploadContentMaxBytes       int64 `mapstructure:"upload_content_max_bytes"`
}

type AsynqConfig struct {
    RedisAddr     string `mapstructure:"redis_addr"`
    RedisPassword string `mapstructure:"redis_password"`
    RedisDB       int    `mapstructure:"redis_db"`
}
```

在 `Config` 顶层聚合两个字段；`config.toml` 加对应段。默认值走 viper `SetDefault`；未 override 时 `asynq.redis_addr` 继承 `[redis]` 段（在 `LoadConfig` 后手动 fill-back）。

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum config/
git commit -m "chore(eval): 引入 Asynq 依赖 + 配置项 (M5.1)"
```

---

### Task 5.2: Asynq Client / Server / Payload / Task 常量

**Files:**
- Create: `internal/infra/mq/task_types.go`
- Create: `internal/infra/mq/payload.go`
- Create: `internal/infra/mq/asynq_client.go`
- Create: `internal/infra/mq/asynq_server.go`

- [ ] **Step 1: `task_types.go`**

```go
package mq

const (
    TaskTypeRunInstance = "eval:run_instance"
    QueueDefault        = "eval:default"
    QueueHigh           = "eval:high"
    QueueDLQ            = "eval:dlq"
)
```

- [ ] **Step 2: `payload.go`**

```go
type RunInstancePayload struct {
    InstanceID string `json:"instance_id"`
    Attempt    int    `json:"attempt"`
    EnqueuedAt int64  `json:"enqueued_at"`
}
```

- [ ] **Step 3: `asynq_client.go` —— 单例 client + `EnqueueRunInstance(ctx, instID, attempt, useHighQueue) error`**

```go
type Client struct{ inner *asynq.Client }

func NewClient(cfg config.AsynqConfig) *Client { ... }

func (c *Client) EnqueueRunInstance(ctx context.Context, instID string, attempt int, useHigh bool) error {
    payload, _ := json.Marshal(RunInstancePayload{
        InstanceID: instID, Attempt: attempt, EnqueuedAt: time.Now().Unix(),
    })
    queue := QueueDefault
    if useHigh { queue = QueueHigh }
    _, err := c.inner.EnqueueContext(ctx,
        asynq.NewTask(TaskTypeRunInstance, payload),
        asynq.Queue(queue),
        asynq.Unique(24*time.Hour),
        asynq.MaxRetry(0),
        asynq.Timeout(20*time.Minute),
        asynq.Retention(72*time.Hour),
    )
    if errors.Is(err, asynq.ErrDuplicateTask) { return nil } // 幂等吞掉
    return err
}
```

- [ ] **Step 4: `asynq_server.go` —— 启动 server + mux + handler 注入**

```go
func StartServer(cfg config.AsynqConfig, evalCfg config.EvaluationConfig, handler asynq.Handler) (*asynq.Server, error) {
    srv := asynq.NewServer(
        asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB},
        asynq.Config{
            Concurrency: evalCfg.WorkerConcurrencyDefault + evalCfg.WorkerConcurrencyHigh,
            Queues: map[string]int{
                QueueDefault: 5,
                QueueHigh:    10,
            },
            RetryDelayFunc: func(n int, e error, t *asynq.Task) time.Duration {
                return 30 * time.Second // case 令牌抢占失败的 short backoff
            },
        },
    )
    mux := asynq.NewServeMux()
    mux.Handle(TaskTypeRunInstance, handler)
    return srv, srv.Start(mux)
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/infra/mq/
git commit -m "feat(eval): Asynq client/server/payload 基础设施 (M5.2)"
```

---

### Task 5.3: CaseTokenGate（Redis Lua 令牌桶）

**Files:**
- Create: `internal/infra/mq/case_token_gate.go`
- Test: `internal/infra/mq/case_token_gate_test.go`（用 miniredis）

- [ ] **Step 1: Lua 脚本**

```lua
-- KEYS[1] = eval:concurrency:case:{case_id}
-- ARGV[1] = limit  ARGV[2] = ttl_seconds
local n = tonumber(redis.call('INCR', KEYS[1]))
if n == 1 then redis.call('EXPIRE', KEYS[1], ARGV[2]) end
if n > tonumber(ARGV[1]) then
    redis.call('DECR', KEYS[1])
    return 0
end
return 1
```

- [ ] **Step 2: 实现 Acquire/Release**

`Acquire` 跑 Lua；`Release` 单 `DECR`；TTL = `instance_total_timeout_sec + 60`。

- [ ] **Step 3: miniredis 单测** 覆盖：并发 8 抢桶大小=4 时恰好 4 成功、TTL 到期自动释放。

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): CaseTokenGate Redis Lua 令牌桶 + miniredis 单测 (M5.3)"
```

---

### Task 5.4: RunInstanceHandler（Asynq Worker 入口）

**Files:**
- Create: `internal/infra/mq/run_instance_handler.go`
- Test: `internal/infra/mq/run_instance_handler_test.go`

- [ ] **Step 1: 实现 handler**

```go
type RunInstanceHandler struct {
    executor *InstanceExecutor                    // 具体类型；构造时 workerID 已注入到 struct 字段
    instRepo repositories.EvalRunInstanceRepository // 用于查 case_id 走令牌桶
    caseGate CaseTokenGate                          // §5.4.1 令牌桶
}

func (h *RunInstanceHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
    var p RunInstancePayload
    if err := json.Unmarshal(task.Payload(), &p); err != nil { return asynq.SkipRetry }

    // PlanReview-R2-B1 修复：直接走 repo 拿 instance（executor 只暴露 Execute 单参，不再对外提供 GetInstance）
    inst, err := h.instRepo.GetByID(ctx, p.InstanceID)
    if err != nil { return asynq.SkipRetry }

    // 令牌桶
    ok, err := h.caseGate.Acquire(ctx, inst.CaseID)
    if err != nil { return err } // Redis 抖动，Asynq 层重试
    if !ok {
        return asynq.RetryTaskError{ Err: errors.New("case slot busy"), Retry: 30 * time.Second }
    }
    defer h.caseGate.Release(ctx, inst.CaseID)

    // 执行：单参签名，workerID 已在 executor 构造时注入（见 Task 4.5.1 结构体字段）
    return h.executor.Execute(ctx, p.InstanceID)
}
```

**接口契约（PlanReview-R2-B1 修复）**：
- `InstanceExecutor.Execute(ctx, instanceID)` **单参**；`workerID / heartbeatInterval` 等运行时上下文挂在 struct 字段，在 M5.5 组装时 `NewInstanceExecutor(...deps..., workerID)` 一次性注入。
- `RunInstanceHandler` 直接依赖 `EvalRunInstanceRepository`（M5.5 组装时注入），不通过 executor 反向查实例，避免 executor 暴露"读实例"接口。
- Task 2.2 `EvaluationDomainService.ExecuteInstance(ctx, id, workerID)` 的语义：这是 domain service 对**外部**调用者（如管理端手动触发单实例执行、集成测）暴露的 API；内部实现是"构造一个临时 InstanceExecutor（用给定 workerID） + 调用 Execute(ctx, id)"。Asynq worker 走的是 M5.5 里预先注入好的常驻 executor，不经 domain service。

`InstanceExecutor.Execute` 内部自己推 QUEUED → INITIALIZING → RUNNING → VERIFYING → 终态；panic 由 defer recover 转 FAILED。

- [ ] **Step 2: 单测**：mock executor 返回 err / OK，断言 handler 行为符合 §5.5 异常分类表。

- [ ] **Step 3: Commit**

```bash
git commit -am "feat(eval): Asynq RunInstanceHandler + 单测 (M5.3)"
```

---

### Task 5.5: 在 route.go / bootstrap 里启动 Asynq server

**Files:**
- Modify: `api/routers/route.go`（在 InitRouter 内）

- [ ] **Step 1: 在 `InitRouter` 起 asynq server（异步 goroutine 内跑）**

```go
if cfg.Evaluation.Enabled {
    asynqCli := mq.NewClient(cfg.Asynq)
    workerID := hostname() + "-" + strconv.Itoa(os.Getpid())
    executor := evaluation.NewInstanceExecutor(
        instRepo, taskRepo, resultRepo, verifyRunner, chatRunner, aggregator,
        tracer, skillExecutor, cfg.Evaluation.WorkspaceRoot,
        workerID,
        time.Duration(cfg.Evaluation.HeartbeatIntervalSec) * time.Second,
        logger,
    )
    evalDomain := evaluation.NewEvaluationDomainService(caseRepo, taskRepo, instRepo, resultRepo, snapRepo, executor, asynqCli)
    handler := mq.NewRunInstanceHandler(executor, instRepo, gate) // PlanReview-R2-B1：handler 直接持 instRepo
    srv, err := mq.StartServer(cfg.Asynq, cfg.Evaluation, handler)
    if err != nil { logger.Fatal(...) }
    // srv.Shutdown 挂到全局 shutdown hook
}
```

- [ ] **Step 2: 观察日志确认 `Asynq connected redis db=1` 输出**

- [ ] **Step 3: Commit**

```bash
git commit -am "feat(eval): 在 InitRouter 内启动 Asynq server (M5.5)"
```

---

## M6 · Application + Handler + Routes

### Task 6.1: Application Service 实现

**Files:**
- Create: `internal/applications/dtos/eval.go`
- Create: `internal/applications/services/eval.go`
- Test: `internal/applications/services/eval_test.go`

- [ ] **Step 1: 定义 DTO**（对齐 spec §6 请求/响应）：`CreateCaseReq / UpdateCaseReq / CaseView / ListCasesQuery / CreateTaskReq / TaskView / InstanceView / ResultView / UploadContentResp` 等。

- [ ] **Step 2: Application Service 编排**

```go
type EvaluationApplicationService interface {
    UploadContent(ctx context.Context, file *multipart.FileHeader) (*UploadContentResp, error)
    CreateCase(ctx context.Context, req *CreateCaseReq) (*CaseView, error)
    // ... 其余 14 个接口 ...
}

type impl struct {
    domain    evaluation.EvaluationDomainService
    mqClient  *mq.Client
    appConfig appconfig.Repository // 用于验证 agent_config_ids 存活
}
```

关键实现要点：
- **`CreateTask`**：事务里插 task + M×N instance + N snapshot；**事务提交后**做 Enqueue（PlanReview-Blk7 修复 spec §11 风险 3）：
  - `batchSize = 50`；每批用 `errgroup` + `semaphore.NewWeighted(8)` 并行 Enqueue，避免 M×N=400 时 HTTP 请求阻塞。
  - 单点 Enqueue 失败记 warn log，实例保持 PENDING；`TaskStatusReconciler`（M7）会扫 `status='PENDING' AND queued_at IS NULL` 重新入队兜底。
  - 引用不存在时 400 全批失败（R2-N5），事务未提交，任何数据不落库。
- **`DeleteCase`**：查 `eval_run_instance WHERE case_id=? AND task.status IN ('PENDING','RUNNING')`；非空 → 409。
- **`DeleteTask`**：spec §2.6 顺序敏感 —— 先 DELETE task 触发 CASCADE，再 DELETE 关联 snapshot；同事务。
- **`RetryTask`**：`SELECT id, attempt FROM eval_run_instance WHERE task_id=? AND status IN ('FAILED','TIMEOUT')`，逐条 CAS 推 PENDING + attempt+1 + 清 conv/msg id，然后走 high queue Enqueue。
- **`UploadContent`**：`c.MaxMultipartMemory + 前置 Content-Length 检查` 双重限 256KB；解码 UTF-8 校验合法；返回文本。

- [ ] **Step 3: 单测**覆盖：CreateTask 引用不存在 → 400；DeleteCase 有活 task → 409；RetryTask 只重投失败实例；UploadContent 超限 → 413。

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): EvaluationApplicationService + DTO + 单测 (M6.1)"
```

---

### Task 6.2: Handler 15 个接口

**Files:**
- Create: `api/handlers/eval.go`
- Test: `api/handlers/eval_test.go`（用 `httptest` + gin）

- [ ] **Step 1: 实现 Handler**（一律薄 —— 只做参数绑定 + 调用 Application + 响应）

```go
type EvalHandler struct{ svc app.EvaluationApplicationService }

func (h *EvalHandler) UploadContent(c *gin.Context) {
    file, err := c.FormFile("file")
    if err != nil { response.BadRequest(c, err); return }
    resp, err := h.svc.UploadContent(c.Request.Context(), file)
    if err != nil { response.Error(c, err); return }
    response.OK(c, resp)
}
// ... 其余 14 个 handler 同样 pattern ...
```

- [ ] **Step 2: 加 Swagger 注解**（R2-N6）：`@Summary / @Tags eval / @Param / @Success / @Router` 齐全。

- [ ] **Step 3: 单测** —— 对每个 handler 走 `httptest.NewRecorder` 打一发；断言 status code + body 结构；mock Application。

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): 15 个 Handler + Swagger 注解 + 单测 (M6.2)"
```

---

### Task 6.3: 路由统一注册

**Files:**
- Modify: `api/routers/route.go`

- [ ] **Step 1: 在 `InitRouter` 内添加 eval 路由组**

```go
evalGroup := r.Group("/api/eval")
{
    evalGroup.POST("/cases/upload-content", evalHandler.UploadContent)
    evalGroup.POST("/cases",             evalHandler.CreateCase)
    evalGroup.GET("/cases",              evalHandler.ListCases)
    evalGroup.GET("/cases/:id",          evalHandler.GetCase)
    evalGroup.PUT("/cases/:id",          evalHandler.UpdateCase)
    evalGroup.DELETE("/cases/:id",       evalHandler.DeleteCase)

    evalGroup.POST("/tasks",             evalHandler.CreateTask)
    evalGroup.GET("/tasks",              evalHandler.ListTasks)
    evalGroup.GET("/tasks/:id",          evalHandler.GetTask)
    evalGroup.POST("/tasks/:id/retry",   evalHandler.RetryTask)
    evalGroup.DELETE("/tasks/:id",       evalHandler.DeleteTask)

    evalGroup.GET("/tasks/:id/instances",     evalHandler.ListInstances)
    evalGroup.GET("/instances/:id",           evalHandler.GetInstance)
    evalGroup.GET("/instances/:id/trace",     evalHandler.GetInstanceTrace)
    evalGroup.POST("/instances/:id/retry",    evalHandler.RetryInstance)
    evalGroup.DELETE("/instances/:id",        evalHandler.DeleteInstance)

    evalGroup.GET("/agent-configs",           evalHandler.ListAgentConfigs)
}
```

- [ ] **Step 2: 同一 `InitRouter` 内启动 Asynq server 与 Cron scheduler（后者见 M7）**

```go
if cfg.Evaluation.Enabled {
    runInstHandler := mq.NewRunInstanceHandler(instanceExecutor, caseTokenGate, hostname)
    asynqSrv, _ := mq.StartServer(cfg.Asynq, cfg.Evaluation, runInstHandler)
    _ = asynqSrv // 挂到全局，Shutdown 时 stop
}
```

- [ ] **Step 3: 启动服务，浏览器打开 Swagger UI 手动确认 17 条路由（含转发）存在**

```bash
go run main.go
# 打开 http://localhost:8080/swagger/index.html —— 应能看到 eval tag 下 15+ 条
```

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(eval): route.go 注册 15 个 API + Asynq server (M6.3)"
```

---

## M7 · Cron 巡检 3 个 Job

### Task 7.1: Cron 单例 + Job 注册

**Files:**
- Create: `internal/infra/scheduler/cron.go`
- Create: `internal/infra/scheduler/jobs.go`
- Modify: `api/routers/route.go`（`InitRouter` 内启动 Scheduler）

- [ ] **Step 1: 引入 `github.com/robfig/cron/v3`**

```bash
go get github.com/robfig/cron/v3@latest
```

- [ ] **Step 2: 单例 Scheduler**

```go
type Scheduler struct{ inner *cron.Cron }

func New(logger *zap.Logger) *Scheduler {
    return &Scheduler{inner: cron.New(cron.WithSeconds(), cron.WithChain(cron.Recover(logger)))}
}
func (s *Scheduler) Start()  { s.inner.Start() }
func (s *Scheduler) Stop()   { s.inner.Stop() }
```

- [ ] **Step 3: 三个 Job 具体逻辑**

```go
type InstanceHeartbeatSweeper struct { repo repositories.EvalRunInstanceRepository; taskRepo ...; cfg EvalCfg }

func (j *InstanceHeartbeatSweeper) Run() {
    // 90s 心跳阈值 AND deadline_at < now() 同时命中（spec §5.6）
    stale, _ := j.repo.ListStaleInstances(ctx, time.Now())
    for _, inst := range stale {
        ok, _ := j.repo.CASStatus(ctx, inst.ID, inst.Status, ev.InstanceStatusTimeout)
        if ok {
            j.resultRepo.Upsert(ctx, &ev.Result{
                InstanceID: inst.ID, Passed: false,
                ErrorLog: "worker_heartbeat_stale_or_deadline_reached",
            })
            j.taskRepo.RecountAndTransit(ctx, inst.TaskID)
        }
    }
}
```

`TaskStatusReconciler`：`SELECT id FROM eval_task WHERE status IN ('PENDING','RUNNING')`；逐个 `RecountAndTransit`；同时补 `SELECT id FROM eval_run_instance WHERE status='PENDING' AND queued_at IS NULL`（Enqueue 失败留守的） → 重新 EnqueueRunInstance。

`AsynqDeadTaskArchiver`：用 `asynq.Inspector` 拉 `eval:dlq` 中 archived tasks；每条 unmarshal payload 拿 `instance_id`；若实例仍非终态，CAS 推 FAILED + upsert result `error_log=asynq_dlq_archived`。

- [ ] **Step 4: 注册**

```go
_, _ = scheduler.AddFunc(fmt.Sprintf("*/%d * * * * *", cfg.CronSweeperIntervalSec),   sweeper.Run)
_, _ = scheduler.AddFunc(fmt.Sprintf("*/%d * * * * *", cfg.CronReconcilerIntervalSec), reconciler.Run)
_, _ = scheduler.AddFunc(fmt.Sprintf("0 */%d * * * *", cfg.CronDLQArchiveIntervalSec/60), archiver.Run)
scheduler.Start()
```

- [ ] **Step 5: 集成测**：构造一条 `status=RUNNING, heartbeat_at=now-2min, deadline_at=now-1min` 的实例；跑一次 `sweeper.Run()`；断言 status=TIMEOUT + result 生成。

- [ ] **Step 6: Commit**

```bash
git add internal/infra/scheduler/ api/routers/route.go
git commit -m "feat(eval): 3 个 Cron 巡检 Job + 集成测 (M7.1)"
```

---

## M8 · 集成测 + 压测

### Task 8.1: 端到端集成测（M×N 生命周期）

**Files:**
- Create: `internal/applications/services/eval_e2e_integration_test.go`

- [ ] **Step 1: 用 miniredis + testcontainers postgres（或 sqlite in-memory）搭起完整栈**

- [ ] **Step 2: 场景**：
  - 创建 2 个 case + 2 个 agent snapshot；
  - 创建 1 个 task → 应生成 4 条 instance；
  - Mock InternalChatRunner 一半返回 pass（verify exit=0）、一半返回 fail（exit=1）；
  - 等待 Asynq worker 消费完（可用 `asynq.Inspector.GetQueueInfo` 轮询到 pending=0）；
  - 断言 task.status=PARTIAL_FAILED，succeeded_count=2 / failed_count=2；
  - 断言 4 条 `eval_result` 存在，且 total_tokens 与 mock Invoker 返回的 usage 一致。

- [ ] **Step 3: Commit**

```bash
git commit -am "test(eval): 端到端 2×2 任务生命周期集成测 (M8.1)"
```

---

### Task 8.2: 并发压测（goroutine 泄漏 + Redis 稳定性）

**Files:**
- Create: `internal/applications/services/eval_stress_test.go`（tag `stress`，默认 skip）

- [ ] **Step 1: 场景**：Enqueue 500 条 instance；mock Invoker 睡 50ms 返回成功；观察 worker pool 稳态。

- [ ] **Step 2: 前后 `runtime.NumGoroutine()` 采样**：差 < 20；

- [ ] **Step 3: Asynq `Inspector.GetQueueInfo("eval:default")`** 峰值 pending 观察；确认 concurrency=12 有效。

- [ ] **Step 4: Commit**

```bash
git commit -am "test(eval): 500 条 instance 压测 + goroutine 泄漏基线 (M8.2)"
```

---

## M9 · E2E 验证 + 文档收尾

### Task 9.1: 手工跑 E2E 验证文档

**Files:**
- 依据：`docs/superpowers/plans/2026-07-16-agent-evaluation-e2e.md`

- [ ] **Step 1: 按 E2E 文档 P1~P10 手工过一遍**（本地起 Redis + PG；用 curl + Swagger）
- [ ] **Step 2: 出现 fail 场景直接开 issue 记录，不阻塞合并**
- [ ] **Step 3: 记录 E2E 结果到 `docs/superpowers/plans/2026-07-16-agent-evaluation-e2e-run.md`**

---

### Task 9.2: README + CHANGELOG + 部署说明

**Files:**
- Modify: `mooc-manus/README.md`（新增"评测能力"段）
- Modify: `mooc-manus/CHANGELOG.md`（若有）
- Create: `mooc-manus/docs/eval-deployment.md`

- [ ] **Step 1: README 加"评测能力"章节**：说明启用配置、端点、Swagger 入口
- [ ] **Step 2: 部署文档**：Redis AOF 建议、asynqmon Web UI 部署方式
- [ ] **Step 3: Commit**

```bash
git commit -am "docs(eval): README + 部署文档 (M9.2)"
```

---

## 附录 A: 关键代码引用速查

| 需要修改的既有文件 | 目的 |
|---|---|
| `internal/domains/models/invoker/invoker.go` | 接口签名扩容 Usage |
| `internal/infra/external/llm/openai.go` | 补 Usage 返回 |
| `internal/infra/external/llm/openai_adapter.go` | 补 Usage 返回 |
| `internal/infra/external/llm/anthropic_adapter.go` | 补 Usage 返回 |
| `internal/domains/services/agents/base.go:475-499` | `startLLMCallSpan / finalizeLLMSpanSuccess` 补 token tag |
| `internal/domains/services/agents/base.go:552, 609` | `StreamingInvokeLLM / InvokeLLM` 接住 Usage |
| `api/routers/route.go: InitRouter` | 注册 eval 路由组 + 启动 Asynq server + Cron |
| `config/config.go` | 加 `EvaluationConfig` + `AsynqConfig` |

## 附录 B: 依赖清单

- `github.com/hibiken/asynq` — 消息队列
- `github.com/robfig/cron/v3` — 定时任务
- `github.com/alicebob/miniredis/v2` (dev) — Redis 单测
- 复用：`gorm.io/gorm`, `gorm.io/datatypes`, `go.uber.org/zap`, `github.com/spf13/viper`, 项目 tracing/agents/tools

## 附录 C: PlanReview 反馈响应清单（Round 1）

Reviewer 提出 8 项 Blocker + 12 项 Nice-to-have。以下为处理情况：

| # | 类型 | 位置 | 处理 |
|---|---|---|---|
| Blk1 | Repository 目录 | Task 1.3/1.4 | 全部改到 `internal/infra/repositories/`（对齐既有 `app_config.go`） |
| Blk2 | 依赖顺序 | Task 5.3/5.4 | 交换 —— 先 CaseTokenGate 后 RunInstanceHandler |
| Blk3 | AutoMigrate 落点 | Task 1.2 | 明确落 `internal/infra/storage/postgres.go: InitStorage()` 尾部 |
| Blk4 | Chat 签名 + snapshot 桥接 | Task 4.4 | 修正 Chat 无返回值；`ChatRequest` 加 `ConfigOverride` 字段前移 snapshot 桥接到 M4 |
| Blk5 | InstanceExecutor 过大 | Task 4.5 | 拆成 4.5.1 骨架/CAS、4.5.2 心跳+init、4.5.3 chat+verify、4.5.4 result+cleanup |
| Blk6 | 破坏性接口一次做完 | Task 3.2 | 拆成 3.2.a Adapter 内部持有 Usage、3.2.b 平推接口+mock、3.2.c 写 span tag |
| Blk7 | §11 风险 3/6 未落地 | Task 6.1 + Task 4.2 | Task 6.1 补 batch=50 + errgroup 并行；Task 4.2 显式白名单 env |
| Blk8 | `/health` 端点不存在 | E-11 | 改用启动日志 + Redis KEYS 校验作为健康信号 |

Nice-to-have 12 项：其中 N-3、N-6、N-9 已合并到对应 Blocker 修复内；余项作为实施阶段的敏捷回填点，不阻断进入 Round 2。

## 附录 D: PlanReview 反馈响应清单（Round 2）

Reviewer 复审确认 Blk1-Blk4/Blk6-Blk8 全部修复到位；找到 1 项新的 Blocker（Blk5 拆分遗留 —— executor 与 handler 签名不一致）。

| # | 位置 | 处理 |
|---|---|---|
| R2-B1 | Task 5.4 handler 调用 executor 签名不匹配 | executor 保持单参 `Execute(ctx, instanceID)`，workerID 挂 struct 字段；handler 直接持 `instRepo` 走 repo 拿 case_id，不经 executor；Task 2.2 `ExecuteInstance(ctx, id, workerID)` 语义定为"外部触发单实例，内部构造临时 executor"，与 Asynq worker 走的常驻 executor 明确区隔 |
| R2-N1 | Task 5.3 commit message 尾巴 `(M5.4)` | 拼写修正为 `(M5.3)` |

Round 2 无其他阻塞。

## 附录 E: PlanReview 最终状态

- Round 1: 8 项 Blocker + 12 项 Nice-to-have → 全部处理
- Round 2: 1 项 Blocker + 1 项 Nice-to-have → 全部处理
- **Round 3: APPROVED** — 无 Blocker，可进入实施阶段

---

**End of Implementation Plan**








