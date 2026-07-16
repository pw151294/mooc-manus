# 智能体评测系统 DDD 架构详解

> 本文档描述 mooc-manus 智能体评测系统的完整 DDD 分层架构，包含 API 层、应用层、领域层、持久化层的接口定义、领域模型和数据模型。

## 目录

- [1. 系统概览](#1-系统概览)
- [2. Domain Layer（领域层）](#2-domain-layer领域层)
- [3. Infrastructure Layer（基础设施层）](#3-infrastructure-layer基础设施层)
- [4. Application Layer（应用层）](#4-application-layer应用层规划中)
- [5. API Layer（API 层）](#5-api-layerapi-层规划中)
- [6. 关键流程图](#6-关键流程图)
- [7. 已实现文件清单](#7-已实现文件清单)

---

## 1. 系统概览

评测系统是一个用于自动化测试智能体性能的子系统，采用经典的 DDD（领域驱动设计）分层架构。

### 1.1 核心概念

| 概念 | 说明 |
|------|------|
| **Case（测试用例）** | 定义一个测试场景，包含初始化脚本、任务提示词和验证脚本 |
| **Task（评测任务）** | 将 M 个 Case 和 N 个 AgentConfig 组合，产生 M×N 个 RunInstance |
| **RunInstance（运行实例）** | 单次评测的执行单元，记录运行状态和结果 |
| **Result（评测结果）** | 存储验证结果、Token 消耗和性能指标 |
| **AgentSnapshot（智能体快照）** | 评测时的智能体配置快照，确保可复现 |

### 1.2 架构分层

```
┌─────────────────────────────────────────────────────────┐
│                     API Layer                            │  (规划中)
│  - RESTful API (15 个端点)                               │
│  - Handler 层薄封装                                      │
└─────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────┐
│                 Application Layer                        │  (规划中)
│  - EvaluationApplicationService                          │
│  - DTO 转换 (PO ↔ DO ↔ DTO)                             │
│  - 事务编排 / 事件发布                                   │
└─────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────┐
│                   Domain Layer                           │  ✅ 已实现
│  ┌─────────────────────────────────────────────────┐   │
│  │ Domain Models (值对象 + 实体)                    │   │
│  │  - Case, Task, RunInstance, Result, AgentSnapshot│   │
│  │  - TaskStatus, InstanceStatus 枚举               │   │
│  └─────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Domain Services                                  │   │
│  │  - EvaluationDomainService (接口)                │   │
│  │  - state_machine.go (状态流转白名单)             │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────┐
│              Infrastructure Layer                        │  ✅ 已实现
│  ┌─────────────────────────────────────────────────┐   │
│  │ Repositories (仓储接口 + GORM 实现)              │   │
│  │  - EvalCaseRepository                            │   │
│  │  - EvalTaskRepository                            │   │
│  │  - EvalRunInstanceRepository                     │   │
│  │  - EvalResultRepository                          │   │
│  │  - EvalAgentSnapshotRepository                   │   │
│  └─────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Persistence Objects (PO) + Converter             │   │
│  │  - EvalCasePO, EvalTaskPO, EvalRunInstancePO...  │   │
│  │  - eval_converter.go (PO ↔ DO 双向转换)          │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────┐
│                    Database (PostgreSQL)                 │
│  - JSONB 字段存储复杂对象                                │
│  - GIN 索引优化 JSONB 查询                               │
│  - 联合索引优化状态查询                                  │
└─────────────────────────────────────────────────────────┘
```

### 1.3 DDD 分层原则

严格遵循 mooc-manus 项目的分层规范：

1. **Handler 层**: 只做参数校验和响应，无业务逻辑
2. **Application 层**: DTO 转换、事务、事件编排
3. **Domain 层**: 业务规则、状态机、聚合根行为
4. **Repository 层**: DB IO，永远只操作 PO

**三态转换路径**: `PO ↔ DO ↔ DTO`
- Repository 层只见 PO
- Domain 层只见 DO
- Handler/Application 层做 DTO 转换

---

## 2. Domain Layer（领域层）

### 2.1 领域模型（Domain Objects）

领域模型位于 `internal/domains/models/evaluation/`，包含 5 个核心 DO 和 2 个状态枚举。

#### 2.1.1 Case（测试用例）

```go
// 文件位置: internal/domains/models/evaluation/case.go
package evaluation

import "time"

type Case struct {
    ID           string    // UUID
    Name         string    // 用例名称
    Description  string    // 用例描述
    InitScript   string    // 初始化脚本（Bash），可空
    TaskPrompt   string    // 任务提示词
    VerifyScript string    // 验证脚本（Bash），退出码 0=通过
    Tags         []string  // 标签（如 "bugfix", "feature"）
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

**职责**: 描述一个测试场景，包含智能体的输入（TaskPrompt）和输出验证逻辑（VerifyScript）。

**关键字段**:
- `InitScript`: 在执行前运行，准备测试环境
- `TaskPrompt`: 发送给智能体的任务描述
- `VerifyScript`: 执行后运行，通过退出码判断是否通过

---

#### 2.1.2 Task（评测任务）

```go
// 文件位置: internal/domains/models/evaluation/task.go
package evaluation

import "time"

type Task struct {
    ID             string
    Name           string
    CaseIDs        []string     // M 个 Case
    AgentConfigIDs []string     // N 个 AgentConfig
    Status         TaskStatus   // PENDING/RUNNING/SUCCEEDED/PARTIAL_FAILED
    TotalCount     int          // M × N
    SucceededCount int
    FailedCount    int
    RunningCount   int
    CreatedAt      time.Time
    StartedAt      *time.Time
    FinishedAt     *time.Time
}
```

**状态枚举** (`internal/domains/models/evaluation/status.go`):

```go
type TaskStatus string

const (
    TaskStatusPending       TaskStatus = "PENDING"
    TaskStatusRunning       TaskStatus = "RUNNING"
    TaskStatusSucceeded     TaskStatus = "SUCCEEDED"
    TaskStatusPartialFailed TaskStatus = "PARTIAL_FAILED"
)
```

**状态流转图**:

```
       ┌─────────────┐
       │   PENDING   │
       └──────┬──────┘
              │
              ├─→ RUNNING ──┬─→ SUCCEEDED ──┐
              │             │               │ (重试)
              │             └─→ PARTIAL_FAILED ─┤
              │                                 │
              ├─→ SUCCEEDED  (瞬时终态)         │
              └─→ PARTIAL_FAILED (瞬时终态)     │
                                               │
              ┌────────────────────────────────┘
              ↓
           RUNNING
```

**职责**: 编排多个 Case 和多个智能体配置的笛卡尔积测试。

**聚合根关系**: Task 是聚合根，通过 `CaseIDs` 和 `AgentConfigIDs` 引用其他实体。

---

#### 2.1.3 RunInstance（运行实例）

```go
// 文件位置: internal/domains/models/evaluation/run_instance.go
package evaluation

import "time"

type RunInstance struct {
    ID                    string
    TaskID                string
    CaseID                string
    CaseSnapshot          Case          // 快照，防止 Case 被修改
    AgentConfigSnapshotID string
    Status                InstanceStatus // 9 种状态
    Attempt               int            // 重试次数
    ConversationID        string         // 关联的会话 ID
    MessageID             string
    TraceID               string         // 链路追踪 ID
    QueuedAt              *time.Time
    StartedAt             *time.Time
    FinishedAt            *time.Time
    HeartbeatAt           *time.Time     // Worker 心跳
    DeadlineAt            *time.Time     // 超时截止时间
    WorkerID              string         // 执行 Worker 的标识
    ErrorMessage          string
}
```

**状态枚举**:

```go
type InstanceStatus string

const (
    InstanceStatusPending      InstanceStatus = "PENDING"
    InstanceStatusQueued       InstanceStatus = "QUEUED"
    InstanceStatusInitializing InstanceStatus = "INITIALIZING"
    InstanceStatusRunning      InstanceStatus = "RUNNING"
    InstanceStatusVerifying    InstanceStatus = "VERIFYING"
    InstanceStatusPassed       InstanceStatus = "PASSED"
    InstanceStatusFailed       InstanceStatus = "FAILED"
    InstanceStatusTimeout      InstanceStatus = "TIMEOUT"
    InstanceStatusCanceled     InstanceStatus = "CANCELED"
)

func (s InstanceStatus) IsTerminal() bool {
    switch s {
    case InstanceStatusPassed, InstanceStatusFailed,
         InstanceStatusTimeout, InstanceStatusCanceled:
        return true
    }
    return false
}
```

**状态流转图**:

```
                ┌─────────────┐
                │   PENDING   │◄─────────────┐
                └──────┬──────┘              │
                       ↓                     │ (重试)
                ┌──────────────┐             │
                │   QUEUED     │             │
                └──────┬───────┘             │
                       ↓                     │
              ┌────────────────┐             │
              │ INITIALIZING   │             │
              └───┬────────┬───┘             │
                  ↓        ↓ (失败/超时)     │
          ┌───────────┐   FAILED/TIMEOUT ────┤
          │  RUNNING  │                      │
          └─┬─────┬──┘                       │
            ↓     ↓ (失败/超时)              │
     ┌──────────┐ FAILED/TIMEOUT ────────────┤
     │VERIFYING │                            │
     └──┬────┬──┘                            │
        ↓    ↓                               │
    PASSED  FAILED ─────────────────────────┘

注：PASSED 是最终终态，不允许重试
```

**关键机制**:

- **CAS（Compare-And-Swap）**: 通过 `CASStatus` 方法实现状态的原子更新，防止并发竞态
- **心跳机制**: `HeartbeatAt` 字段记录 Worker 的心跳，巡检器检测超时实例
- **快照机制**: `CaseSnapshot` 字段冻结测试时的 Case 内容，确保可复现

---

#### 2.1.4 Result（评测结果）

```go
// 文件位置: internal/domains/models/evaluation/result.go
package evaluation

import "time"

type Result struct {
    InstanceID       string    // 1:1 关联 RunInstance
    Passed           bool      // 是否通过
    VerifyExitCode   int       // 验证脚本退出码
    VerifyStdout     string
    VerifyStderr     string
    PromptTokens     int64     // Token 消耗
    CompletionTokens int64
    TotalTokens      int64
    AgentLatencyMs   int64     // 智能体响应延迟（毫秒）
    ErrorLog         string    // 错误日志
    FinishedAt       time.Time
}
```

**关键字段**:
- `Passed`: 通过 `VerifyExitCode == 0` 判断
- `PromptTokens/CompletionTokens`: 从链路追踪的 `llm.io.*_units` tag 中提取
- `AgentLatencyMs`: 从 span 的 `LatencyMs` 中提取

---

#### 2.1.5 AgentSnapshot（智能体快照）

```go
// 文件位置: internal/domains/models/evaluation/agent_snapshot.go
package evaluation

import "time"

type AgentSnapshot struct {
    ID                string
    SourceAppConfigID string         // 源 AppConfig ID
    Model             string          // 模型名称
    SystemPrompt      string
    ToolsConfig       map[string]any  // JSON 存储
    MCPConfig         map[string]any
    A2AConfig         map[string]any
    CreatedAt         time.Time
}
```

**职责**: 冻结评测时的智能体配置，确保评测可复现。

**设计原因**:
- AppConfig 可能会被用户修改，导致历史评测结果无法对比
- Snapshot 机制保证了评测的可复现性

---

### 2.2 领域服务（Domain Services）

#### 2.2.1 EvaluationDomainService

评测领域服务是所有业务逻辑的中枢，定义在 `internal/domains/services/evaluation/service.go`。

```go
package evaluation

import (
    "context"
    ev "mooc-manus/internal/domains/models/evaluation"
    "mooc-manus/internal/infra/repositories"
)

type EvaluationDomainService interface {
    // ==================== 用例管理 ====================
    CreateCase(ctx context.Context, c *ev.Case) (*ev.Case, error)
    UpdateCase(ctx context.Context, c *ev.Case) (*ev.Case, error)
    DeleteCase(ctx context.Context, id string) error
    ListCases(ctx context.Context, filter repositories.CaseListFilter, page, size int) ([]*ev.Case, int64, error)
    GetCase(ctx context.Context, id string) (*ev.Case, error)

    // ==================== 任务管理 ====================
    CreateTask(ctx context.Context, name string, caseIDs, agentConfigIDs []string) (*ev.Task, error)
    ListTasks(ctx context.Context, filter repositories.TaskListFilter, page, size int) ([]*ev.Task, int64, error)
    GetTask(ctx context.Context, id string) (*ev.Task, error)
    RetryTaskFailedInstances(ctx context.Context, id string) (int, error)
    DeleteTask(ctx context.Context, id string) error

    // ==================== 实例管理 ====================
    ListInstances(ctx context.Context, taskID string, filter repositories.InstanceListFilter, page, size int) ([]*ev.RunInstance, int64, error)
    GetInstance(ctx context.Context, id string) (*ev.RunInstance, error)
    RetryInstance(ctx context.Context, id string) error
    DeleteInstance(ctx context.Context, id string) error

    // ==================== Worker 入口 ====================
    ExecuteInstance(ctx context.Context, instanceID string, workerID string) error

    // ==================== 巡检 ====================
    SweepStaleInstances(ctx context.Context) (int, error)
    ReconcileTaskStatuses(ctx context.Context) (int, error)
    ArchiveDeadTasks(ctx context.Context) (int, error)
}
```

**核心方法说明**:

##### 2.2.1.1 CreateTask - 创建评测任务

**输入**: 任务名称、Case ID 列表、AgentConfig ID 列表

**行为流程**:
```
1. 校验 CaseIDs 和 AgentConfigIDs 存在
2. 开启事务
3. 为每个 AgentConfig 创建 Snapshot 记录
4. 创建 Task 实体（TotalCount = M × N）
5. 为每个 (Case × AgentConfig) 组合创建 RunInstance（Status=PENDING）
6. 提交事务
7. 事务提交后异步 Enqueue 到 Asynq 队列
```

##### 2.2.1.2 ExecuteInstance - Worker 执行入口

**核心行为流程**:
```
1. CAS 更新状态: QUEUED → INITIALIZING
2. 创建工作区并执行 InitScript
3. CAS 更新状态: INITIALIZING → RUNNING
4. 调用智能体执行任务（复用 BaseAgentDomainService）
5. CAS 更新状态: RUNNING → VERIFYING
6. 执行 VerifyScript
7. CAS 更新状态: VERIFYING → PASSED/FAILED
8. 通过 TraceAggregator 聚合 span 指标
9. 记录 Result（含 token/latency/exit_code）
10. 触发 Task.RecountAndTransit
```

##### 2.2.1.3 SweepStaleInstances - 巡检超时实例

**行为**:
- 查询 `heartbeat_at < now() - 90s` OR `deadline_at < now()` 的非终态实例
- CAS 更新状态 → TIMEOUT
- 记录错误日志

##### 2.2.1.4 ReconcileTaskStatuses - 同步任务状态

**行为**:
- 对每个 RUNNING 的 Task，调用 `TaskRepo.RecountAndTransit`
- 补漏 Enqueue 失败留守的 PENDING 实例

---

#### 2.2.2 状态机白名单

状态流转的合法性由 `state_machine.go` 提供的白名单函数保证。

```go
// 文件位置: internal/domains/services/evaluation/state_machine.go
package evaluation

import (
    "fmt"
    ev "mooc-manus/internal/domains/models/evaluation"
)

// ==================== RunInstance 状态流转白名单 ====================
var instanceWhitelist = map[ev.InstanceStatus]map[ev.InstanceStatus]bool{
    ev.InstanceStatusPending: {
        ev.InstanceStatusQueued: true,
    },
    ev.InstanceStatusQueued: {
        ev.InstanceStatusInitializing: true,
    },
    ev.InstanceStatusInitializing: {
        ev.InstanceStatusRunning: true,
        ev.InstanceStatusFailed:  true,
        ev.InstanceStatusTimeout: true,
    },
    ev.InstanceStatusRunning: {
        ev.InstanceStatusVerifying: true,
        ev.InstanceStatusFailed:    true,
        ev.InstanceStatusTimeout:   true,
    },
    ev.InstanceStatusVerifying: {
        ev.InstanceStatusPassed:  true,
        ev.InstanceStatusFailed:  true,
        ev.InstanceStatusTimeout: true,
    },
    ev.InstanceStatusFailed: {
        ev.InstanceStatusPending: true, // 重试
    },
    ev.InstanceStatusTimeout: {
        ev.InstanceStatusPending: true, // 重试
    },
}

func TransitInstance(from, to ev.InstanceStatus) error {
    if allowed, ok := instanceWhitelist[from]; ok && allowed[to] {
        return nil
    }
    return fmt.Errorf("非法实例流转: %s → %s", from, to)
}

// ==================== Task 状态流转白名单 ====================
var taskWhitelist = map[ev.TaskStatus]map[ev.TaskStatus]bool{
    ev.TaskStatusPending: {
        ev.TaskStatusRunning:       true,
        ev.TaskStatusSucceeded:     true, // edge case: 全部瞬间完成
        ev.TaskStatusPartialFailed: true,
    },
    ev.TaskStatusRunning: {
        ev.TaskStatusSucceeded:     true,
        ev.TaskStatusPartialFailed: true,
    },
    ev.TaskStatusSucceeded: {
        ev.TaskStatusRunning: true, // 重试
    },
    ev.TaskStatusPartialFailed: {
        ev.TaskStatusRunning: true, // 重试
    },
}

func TransitTask(from, to ev.TaskStatus) error {
    if allowed, ok := taskWhitelist[from]; ok && allowed[to] {
        return nil
    }
    return fmt.Errorf("非法任务流转: %s → %s", from, to)
}
```

**设计意图**:
- 使用白名单模式，显式定义允许的状态流转
- 在 CAS 操作前调用 `TransitInstance/TransitTask` 验证合法性
- 防止非法状态流转导致数据不一致

**穷举测试**（`state_machine_test.go`）:
- 16 个 InstanceStatus 转换场景（13 合法 + 3 非法）
- 9 个 TaskStatus 转换场景（7 合法 + 2 非法）

---

## 3. Infrastructure Layer（基础设施层）

### 3.1 Repository 接口

Repository 接口和实现全部放在 `internal/infra/repositories/`，遵循项目现有约定。

#### 3.1.1 EvalCaseRepository

```go
// 文件位置: internal/infra/repositories/eval_case.go
package repositories

import (
    "context"
    "mooc-manus/internal/domains/models/evaluation"
)

type CaseListFilter struct {
    NameLike string
    Tags     []string
}

type EvalCaseRepository interface {
    Create(ctx context.Context, c *evaluation.Case) error
    Get(ctx context.Context, id string) (*evaluation.Case, error)
    List(ctx context.Context, filter CaseListFilter, page, size int) ([]*evaluation.Case, int64, error)
    Update(ctx context.Context, c *evaluation.Case) error
    Delete(ctx context.Context, id string) error
    ExistsRunningReferences(ctx context.Context, caseID string) (bool, error)
}
```

**关键方法**:

- `ExistsRunningReferences`: 检查是否有正在运行的任务引用该 Case，防止误删

```go
// GORM 实现示例
func (r *evalCaseRepositoryImpl) ExistsRunningReferences(
    ctx context.Context, caseID string,
) (bool, error) {
    var count int64
    err := r.db.WithContext(ctx).Model(&models.EvalRunInstancePO{}).
        Joins("JOIN eval_task ON eval_run_instance.task_id = eval_task.id").
        Where("eval_run_instance.case_id = ?", caseID).
        Where("eval_task.status IN ?", []string{"PENDING", "RUNNING"}).
        Count(&count).Error
    return count > 0, err
}
```

---

#### 3.1.2 EvalTaskRepository

```go
// 文件位置: internal/infra/repositories/eval_task.go
package repositories

import (
    "context"
    "mooc-manus/internal/domains/models/evaluation"
)

type TaskListFilter struct {
    Status evaluation.TaskStatus
}

type EvalTaskRepository interface {
    Create(ctx context.Context, t *evaluation.Task) error
    Get(ctx context.Context, id string) (*evaluation.Task, error)
    List(ctx context.Context, filter TaskListFilter, page, size int) ([]*evaluation.Task, int64, error)
    Update(ctx context.Context, t *evaluation.Task) error
    Delete(ctx context.Context, id string) error
    RecountAndTransit(ctx context.Context, taskID string) error
}
```

**关键方法：`RecountAndTransit`**

原子地统计 RunInstance 终态数量并更新 Task 状态。

```go
func (r *evalTaskRepositoryImpl) RecountAndTransit(
    ctx context.Context, taskID string,
) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        var terminal, passed, total int64
        err := tx.Raw(`
            SELECT
              COUNT(*) FILTER (WHERE status IN ('PASSED','FAILED','TIMEOUT','CANCELED')) AS terminal,
              COUNT(*) FILTER (WHERE status='PASSED') AS passed,
              COUNT(*) AS total
            FROM eval_run_instance WHERE task_id = ?
        `, taskID).Row().Scan(&terminal, &passed, &total)
        if err != nil {
            return err
        }

        var newStatus string
        switch {
        case terminal < total:
            newStatus = "RUNNING"
        case terminal == total && passed == total:
            newStatus = "SUCCEEDED"
        default:
            newStatus = "PARTIAL_FAILED"
        }

        now := time.Now()
        upd := map[string]any{
            "status":          newStatus,
            "succeeded_count": passed,
            "failed_count":    terminal - passed,
            "running_count":   total - terminal,
        }
        if newStatus == "SUCCEEDED" || newStatus == "PARTIAL_FAILED" {
            upd["finished_at"] = &now
        }
        return tx.Model(&models.EvalTaskPO{}).
            Where("id = ?", taskID).Updates(upd).Error
    })
}
```

---

#### 3.1.3 EvalRunInstanceRepository（核心）

这是最重要的仓储接口，包含关键的 CAS 状态更新方法。

```go
// 文件位置: internal/infra/repositories/eval_run_instance.go
package repositories

import (
    "context"
    "mooc-manus/internal/domains/models/evaluation"
    "time"
)

type InstanceListFilter struct {
    TaskID string
    Status evaluation.InstanceStatus
}

type EvalRunInstanceRepository interface {
    Create(ctx context.Context, inst *evaluation.RunInstance) error
    GetByID(ctx context.Context, id string) (*evaluation.RunInstance, error)
    GetStatus(ctx context.Context, id string) (evaluation.InstanceStatus, error)
    List(ctx context.Context, filter InstanceListFilter, page, size int) ([]*evaluation.RunInstance, int64, error)
    Update(ctx context.Context, inst *evaluation.RunInstance) error
    Delete(ctx context.Context, id string) error
    UpdateTraceID(ctx context.Context, id, traceID string) error
    ListStaleInstances(ctx context.Context, before time.Time) ([]*evaluation.RunInstance, error)
    UpdateHeartbeat(ctx context.Context, id, workerID string, now time.Time) error

    // ⭐ 核心方法：CAS 原子更新状态
    CASStatus(ctx context.Context, id string, from, to evaluation.InstanceStatus) (bool, error)
}
```

**核心方法 1：CASStatus（Compare-And-Swap）**

```go
// GORM 实现
func (r *evalRunInstanceRepositoryImpl) CASStatus(
    ctx context.Context,
    id string,
    from, to evaluation.InstanceStatus,
) (bool, error) {
    result := r.db.WithContext(ctx).
        Model(&models.EvalRunInstancePO{}).
        Where("id = ? AND status = ?", id, string(from)).
        Update("status", string(to))
    return result.RowsAffected > 0, result.Error
}
```

**对应的 SQL**:
```sql
UPDATE eval_run_instance
SET status = 'VERIFYING'
WHERE id = 'xxx' AND status = 'RUNNING';
-- 如果 status 已经不是 RUNNING，则 RowsAffected = 0
```

**使用场景**:
```go
// Worker 尝试将状态从 RUNNING → VERIFYING
ok, err := repo.CASStatus(ctx, instanceID,
    evaluation.InstanceStatusRunning,
    evaluation.InstanceStatusVerifying)
if !ok {
    // 状态已被其他 Worker 或巡检器修改，放弃操作
    return nil
}
// 成功抢到状态锁，继续执行
```

**并发竞态测试** (`eval_run_instance_test.go`):

```go
func TestCASStatusRacesOnce(t *testing.T) {
    // 创建一个 RUNNING 状态的实例
    // ...

    // 两个 goroutine 同时尝试 CAS RUNNING → VERIFYING
    var wg sync.WaitGroup
    results := make([]bool, 2)
    for i := 0; i < 2; i++ {
        i := i
        wg.Add(1)
        go func() {
            defer wg.Done()
            ok, _ := repo.CASStatus(ctx, instID,
                InstanceStatusRunning, InstanceStatusVerifying)
            results[i] = ok
        }()
    }
    wg.Wait()

    // 断言：只有一方成功
    won := 0
    for _, r := range results {
        if r { won++ }
    }
    assert.Equal(t, 1, won, "只应有一方 CAS 成功")
}
```

**核心方法 2：ListStaleInstances**

查询超时实例（心跳落后 OR deadline 已过）。

```go
func (r *evalRunInstanceRepositoryImpl) ListStaleInstances(
    ctx context.Context,
    before time.Time,
) ([]*evaluation.RunInstance, error) {
    var pos []models.EvalRunInstancePO
    err := r.db.WithContext(ctx).
        Where("status NOT IN ?", []string{"PASSED", "FAILED", "TIMEOUT", "CANCELED"}).
        Where("(heartbeat_at IS NULL OR heartbeat_at < ?) OR (deadline_at IS NOT NULL AND deadline_at < ?)",
            before, time.Now()).
        Find(&pos).Error
    if err != nil {
        return nil, err
    }
    dos := make([]*evaluation.RunInstance, len(pos))
    for i := range pos {
        dos[i] = instanceToDO(&pos[i])
    }
    return dos, nil
}
```

---

#### 3.1.4 EvalResultRepository

```go
// 文件位置: internal/infra/repositories/eval_result.go
package repositories

import (
    "context"
    "mooc-manus/internal/domains/models/evaluation"
)

type EvalResultRepository interface {
    Create(ctx context.Context, r *evaluation.Result) error
    Get(ctx context.Context, instanceID string) (*evaluation.Result, error)
    Upsert(ctx context.Context, r *evaluation.Result) error
}
```

**关键方法：`Upsert`** - 基于 `instance_id` 唯一约束的插入或更新

```go
func (r *evalResultRepositoryImpl) Upsert(
    ctx context.Context, res *evaluation.Result,
) error {
    po := resultToPO(res)
    return r.db.WithContext(ctx).Clauses(clause.OnConflict{
        Columns: []clause.Column{{Name: "instance_id"}},
        DoUpdates: clause.AssignmentColumns([]string{
            "passed", "verify_exit_code", "verify_stdout", "verify_stderr",
            "prompt_tokens", "completion_tokens", "total_tokens",
            "agent_latency_ms", "error_log", "finished_at",
        }),
    }).Create(po).Error
}
```

**对应的 SQL**:
```sql
INSERT INTO eval_result (instance_id, passed, ...) VALUES (...)
ON CONFLICT (instance_id) DO UPDATE SET
    passed = EXCLUDED.passed,
    verify_exit_code = EXCLUDED.verify_exit_code,
    ...;
```

---

#### 3.1.5 EvalAgentSnapshotRepository

```go
// 文件位置: internal/infra/repositories/eval_agent_snapshot.go
package repositories

import (
    "context"
    "mooc-manus/internal/domains/models/evaluation"
)

type EvalAgentSnapshotRepository interface {
    Create(ctx context.Context, s *evaluation.AgentSnapshot) error
    Get(ctx context.Context, id string) (*evaluation.AgentSnapshot, error)
    Delete(ctx context.Context, id string) error
    BatchCreate(ctx context.Context, snapshots []*evaluation.AgentSnapshot) error
}
```

**关键方法：`BatchCreate`** - CreateTask 时批量创建 N 个 Snapshot

```go
func (r *evalAgentSnapshotRepositoryImpl) BatchCreate(
    ctx context.Context, snapshots []*evaluation.AgentSnapshot,
) error {
    pos := make([]*models.EvalAgentSnapshotPO, len(snapshots))
    for i, s := range snapshots {
        pos[i] = snapshotToPO(s)
    }
    return r.db.WithContext(ctx).Create(&pos).Error
}
```

---

### 3.2 Persistence Objects（持久化对象）

PO 位于 `internal/infra/models/`，与 GORM 强绑定，使用 PostgreSQL 特有的类型。

#### 3.2.1 EvalCasePO

```go
// 文件位置: internal/infra/models/eval_case.go
package models

import (
    "time"
    "gorm.io/datatypes"
)

type EvalCasePO struct {
    ID           string         `gorm:"type:uuid;primaryKey"`
    Name         string         `gorm:"type:varchar(255);uniqueIndex"`
    Description  string         `gorm:"type:text"`
    InitScript   string         `gorm:"type:text"`
    TaskPrompt   string         `gorm:"type:text"`
    VerifyScript string         `gorm:"type:text"`
    Tags         datatypes.JSON `gorm:"type:jsonb"`
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

func (EvalCasePO) TableName() string {
    return "eval_case"
}
```

**PostgreSQL 表结构**:
```sql
CREATE TABLE eval_case (
    id            UUID PRIMARY KEY,
    name          VARCHAR(255) UNIQUE,
    description   TEXT,
    init_script   TEXT,
    task_prompt   TEXT,
    verify_script TEXT,
    tags          JSONB,
    created_at    TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ
);

-- GIN 索引优化 tags JSONB 查询
CREATE INDEX idx_eval_case_tags_gin ON eval_case USING GIN (tags);
```

---

#### 3.2.2 EvalTaskPO

```go
// 文件位置: internal/infra/models/eval_task.go
package models

import (
    "time"
    "gorm.io/datatypes"
)

type EvalTaskPO struct {
    ID             string         `gorm:"type:uuid;primaryKey"`
    Name           string         `gorm:"type:varchar(255)"`
    CaseIDs        datatypes.JSON `gorm:"type:jsonb"`
    AgentConfigIDs datatypes.JSON `gorm:"type:jsonb"`
    Status         string         `gorm:"type:varchar(24);index"`
    TotalCount     int
    SucceededCount int
    FailedCount    int
    RunningCount   int
    CreatedAt      time.Time
    StartedAt      *time.Time
    FinishedAt     *time.Time
}

func (EvalTaskPO) TableName() string {
    return "eval_task"
}
```

---

#### 3.2.3 EvalRunInstancePO

```go
// 文件位置: internal/infra/models/eval_run_instance.go
package models

import (
    "time"
    "gorm.io/datatypes"
)

type EvalRunInstancePO struct {
    ID                    string         `gorm:"type:uuid;primaryKey"`
    TaskID                string         `gorm:"type:uuid;index:idx_task_status,priority:1;uniqueIndex:uk_task_case_snap,priority:1;constraint:OnDelete:CASCADE"`
    CaseID                string         `gorm:"type:uuid;uniqueIndex:uk_task_case_snap,priority:2"`
    CaseSnapshot          datatypes.JSON `gorm:"type:jsonb"`
    AgentConfigSnapshotID string         `gorm:"type:uuid;uniqueIndex:uk_task_case_snap,priority:3;constraint:OnDelete:RESTRICT"`
    Status                string         `gorm:"type:varchar(24);index:idx_task_status,priority:2;index:idx_status_heartbeat,priority:1"`
    Attempt               int
    ConversationID        string         `gorm:"type:varchar(64)"`
    MessageID             string         `gorm:"type:varchar(64)"`
    TraceID               string         `gorm:"type:varchar(64)"`
    QueuedAt              *time.Time
    StartedAt             *time.Time
    FinishedAt            *time.Time
    HeartbeatAt           *time.Time     `gorm:"index:idx_status_heartbeat,priority:2"`
    DeadlineAt            *time.Time
    WorkerID              string         `gorm:"type:varchar(64)"`
    ErrorMessage          string         `gorm:"type:text"`
}

func (EvalRunInstancePO) TableName() string {
    return "eval_run_instance"
}
```

**索引设计**:

| 索引名 | 列 | 用途 |
|--------|-----|------|
| `idx_task_status` | `(task_id, status)` | 按任务查询实例状态分布 |
| `idx_status_heartbeat` | `(status, heartbeat_at)` | 巡检器扫描超时实例 |
| `uk_task_case_snap` | `(task_id, case_id, agent_config_snapshot_id)` UNIQUE | 防止重复创建 |

**外键约束**:
- `task_id` → `eval_task.id` ON DELETE CASCADE
- `agent_config_snapshot_id` → `eval_agent_snapshot.id` ON DELETE RESTRICT（防止误删 snapshot）

---

#### 3.2.4 EvalResultPO

```go
// 文件位置: internal/infra/models/eval_result.go
package models

import "time"

type EvalResultPO struct {
    InstanceID       string `gorm:"type:uuid;uniqueIndex;constraint:OnDelete:CASCADE"`
    Passed           bool
    VerifyExitCode   int
    VerifyStdout     string `gorm:"type:text"`
    VerifyStderr     string `gorm:"type:text"`
    PromptTokens     int64
    CompletionTokens int64
    TotalTokens      int64
    AgentLatencyMs   int64
    ErrorLog         string `gorm:"type:text"`
    FinishedAt       time.Time
}

func (EvalResultPO) TableName() string {
    return "eval_result"
}
```

---

#### 3.2.5 EvalAgentSnapshotPO

```go
// 文件位置: internal/infra/models/eval_agent_snapshot.go
package models

import (
    "time"
    "gorm.io/datatypes"
)

type EvalAgentSnapshotPO struct {
    ID                string         `gorm:"type:uuid;primaryKey"`
    SourceAppConfigID string         `gorm:"type:uuid;index"`
    Model             string         `gorm:"type:varchar(64)"`
    SystemPrompt      string         `gorm:"type:text"`
    ToolsConfig       datatypes.JSON `gorm:"type:jsonb"`
    MCPConfig         datatypes.JSON `gorm:"type:jsonb"`
    A2AConfig         datatypes.JSON `gorm:"type:jsonb"`
    CreatedAt         time.Time
}

func (EvalAgentSnapshotPO) TableName() string {
    return "eval_agent_snapshot"
}
```

---

### 3.3 PO ↔ DO 转换函数

集中在 `internal/infra/repositories/eval_converter.go` 中，处理 JSONB 字段的序列化/反序列化。

**示例：Case 的转换**

```go
// 文件位置: internal/infra/repositories/eval_converter.go
package repositories

import (
    "encoding/json"
    "mooc-manus/internal/domains/models/evaluation"
    "mooc-manus/internal/infra/models"
    "gorm.io/datatypes"
)

// PO → DO
func caseToDO(po *models.EvalCasePO) *evaluation.Case {
    if po == nil {
        return nil
    }
    var tags []string
    if po.Tags != nil {
        _ = json.Unmarshal(po.Tags, &tags)
    }
    return &evaluation.Case{
        ID:           po.ID,
        Name:         po.Name,
        Description:  po.Description,
        InitScript:   po.InitScript,
        TaskPrompt:   po.TaskPrompt,
        VerifyScript: po.VerifyScript,
        Tags:         tags,
        CreatedAt:    po.CreatedAt,
        UpdatedAt:    po.UpdatedAt,
    }
}

// DO → PO
func caseToPO(do *evaluation.Case) *models.EvalCasePO {
    if do == nil {
        return nil
    }
    tagsJSON, _ := json.Marshal(do.Tags)
    return &models.EvalCasePO{
        ID:           do.ID,
        Name:         do.Name,
        Description:  do.Description,
        InitScript:   do.InitScript,
        TaskPrompt:   do.TaskPrompt,
        VerifyScript: do.VerifyScript,
        Tags:         datatypes.JSON(tagsJSON),
        CreatedAt:    do.CreatedAt,
        UpdatedAt:    do.UpdatedAt,
    }
}
```

**转换函数清单**:
- `caseToDO` / `caseToPO`
- `taskToDO` / `taskToPO`
- `instanceToDO` / `instanceToPO`
- `resultToDO` / `resultToPO`
- `snapshotToDO` / `snapshotToPO`

---

### 3.4 AutoMigrate 与索引管理

数据库表的创建统一在 `internal/infra/storage/postgres.go` 的 `InitStorage()` 函数中。

```go
// 文件位置: internal/infra/storage/postgres.go
func InitStorage() error {
    // ... 建立 DB 连接 ...

    // 评测模块 AutoMigrate
    // 依赖顺序：snapshot/task 无外键 → instance 依赖它们 → result 依赖 instance
    if err := db.AutoMigrate(
        &models.EvalCasePO{},
        &models.EvalTaskPO{},
        &models.EvalAgentSnapshotPO{},
        &models.EvalRunInstancePO{},
        &models.EvalResultPO{},
    ); err != nil {
        return fmt.Errorf("eval AutoMigrate: %w", err)
    }

    // GIN 索引 post-hook（GORM 不支持声明 GIN，用原生 SQL）
    db.Exec(`CREATE INDEX IF NOT EXISTS idx_eval_case_tags_gin ON eval_case USING GIN (tags)`)

    return nil
}
```

---

### 3.5 LLM Usage 补强（M3 里程碑）

为了收集评测的 Token 消耗，对现有 LLM 调用链路进行了改造。

#### 3.5.1 Usage 值对象

```go
// 文件位置: internal/domains/models/llm/usage.go
package llm

type Usage struct {
    PromptTokens     int64
    CompletionTokens int64
    TotalTokens      int64
}
```

#### 3.5.2 Invoker 接口扩容

```go
// 文件位置: internal/domains/models/invoker/invoker.go
type Invoker interface {
    Invoke(messages []llm.Message, tools []llm.Tool) (llm.Message, error)
    StreamingInvoke(messages []llm.Message, tools []llm.Tool, eventCh chan<- events.AgentEvent) llm.Message
    LastUsage() llm.Usage // 新增：获取最近一次调用的 token 消耗
}
```

#### 3.5.3 Adapter 层记录 Usage

```go
// 文件位置: internal/infra/external/llm/openai_adapter.go
type OpenAIAdapter struct {
    llm         *OpenAiLLM
    lastUsage   domainllm.Usage // 最近一次调用的 usage
    lastUsageMu sync.Mutex
}

func (a *OpenAIAdapter) Invoke(...) (domainllm.Message, error) {
    resp, sdkUsage, err := a.llm.Invoke(...)
    // ...
    a.lastUsageMu.Lock()
    a.lastUsage = domainllm.Usage{
        PromptTokens:     int64(sdkUsage.PromptTokens),
        CompletionTokens: int64(sdkUsage.CompletionTokens),
        TotalTokens:      int64(sdkUsage.TotalTokens),
    }
    a.lastUsageMu.Unlock()
    return fromOpenAIMessage(resp), nil
}

func (a *OpenAIAdapter) LastUsage() domainllm.Usage {
    a.lastUsageMu.Lock()
    defer a.lastUsageMu.Unlock()
    return a.lastUsage
}
```

#### 3.5.4 BaseAgent 写入 Span Tag

```go
// 文件位置: internal/domains/services/agents/base.go
func (a *BaseAgent) finalizeLLMSpanSuccess(llmSpan *tracing.Span, toolCallsCount int) {
    llmSpan.SetTag("llm.tool_calls_count", toolCallsCount)

    // 写入 usage tag（使用 llm.io.*_units 避免脱敏正则误杀）
    usage := a.invoker.LastUsage()
    llmSpan.SetTag("llm.io.prompt_units", usage.PromptTokens)
    llmSpan.SetTag("llm.io.completion_units", usage.CompletionTokens)
    llmSpan.SetTag("llm.io.total_units", usage.TotalTokens)

    llmSpan.AddLog("INFO", "llm.stream.completed", nil)
}
```

**⚠️ 关键设计决策**:

原计划使用 `llm.usage.prompt_tokens` 命名，但 tracing 层的脱敏正则包含 `token` 关键词，会导致误杀（值被替换为 `***`）。经守护测试验证后，退化为 `llm.io.*_units` 命名。

**脱敏正则**（`internal/domains/models/tracing/span.go`）:
```go
sensitiveRegex = regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret|authorization)`)
```

**守护测试**（`span_usage_masking_test.go`）:
```go
func TestUsageTagNotMasked(t *testing.T) {
    s := newTestSpan("t", 1, 0, SpanTypeLLMCall, "")
    s.SetTag("llm.io.prompt_units", int64(123))
    s.SetTag("llm.io.completion_units", int64(456))
    s.SetTag("llm.io.total_units", int64(579))
    require.Equal(t, int64(123), s.tags["llm.io.prompt_units"])
    require.Equal(t, int64(456), s.tags["llm.io.completion_units"])
    require.Equal(t, int64(579), s.tags["llm.io.total_units"])
}
```

---

## 4. Application Layer（应用层，规划中）

> ⚠️ 本层尚未实现，以下为设计方案。

应用层承担 DTO 转换、事务编排、事件发布的职责。

### 4.1 EvaluationApplicationService 接口

```go
// 计划文件位置: internal/applications/services/eval.go
type EvaluationApplicationService interface {
    UploadContent(ctx context.Context, file *multipart.FileHeader) (*UploadContentResp, error)

    // 用例 CRUD
    CreateCase(ctx context.Context, req *CreateCaseReq) (*CaseView, error)
    UpdateCase(ctx context.Context, id string, req *UpdateCaseReq) (*CaseView, error)
    DeleteCase(ctx context.Context, id string) error
    ListCases(ctx context.Context, query *ListCasesQuery) ([]*CaseView, int64, error)
    GetCase(ctx context.Context, id string) (*CaseView, error)

    // 任务 CRUD
    CreateTask(ctx context.Context, req *CreateTaskReq) (*TaskView, error)
    ListTasks(ctx context.Context, query *ListTasksQuery) ([]*TaskView, int64, error)
    GetTask(ctx context.Context, id string) (*TaskView, error)
    RetryTask(ctx context.Context, id string) (*RetryTaskResp, error)
    DeleteTask(ctx context.Context, id string) error

    // 实例查询
    ListInstances(ctx context.Context, taskID string, query *ListInstancesQuery) ([]*InstanceView, int64, error)
    GetInstance(ctx context.Context, id string) (*InstanceView, error)
    GetInstanceTrace(ctx context.Context, id string) (*TraceView, error)
    RetryInstance(ctx context.Context, id string) error
    DeleteInstance(ctx context.Context, id string) error

    // 辅助
    ListAgentConfigs(ctx context.Context) ([]*AgentConfigView, error)
}
```

### 4.2 关键实现要点

#### 4.2.1 CreateTask 实现要点

```
1. 校验 CaseIDs 和 AgentConfigIDs 都存在（400 全批失败）
2. 开启事务：
   a. 创建 N 个 AgentSnapshot（BatchCreate）
   b. 创建 1 个 Task（TotalCount = M × N）
   c. 创建 M × N 个 RunInstance（Status=PENDING）
3. 事务提交
4. 事务提交后异步 Enqueue：
   - batchSize = 50
   - errgroup + semaphore.NewWeighted(8) 并行 Enqueue
   - Enqueue 失败记 warn log，实例保持 PENDING
   - TaskStatusReconciler 兜底重投
```

#### 4.2.2 DeleteCase 实现要点

```go
// 前置检查
exists, _ := domain.ExistsRunningReferences(caseID)
if exists {
    return 409 // Conflict
}
return domain.DeleteCase(caseID)
```

#### 4.2.3 RetryTask 实现要点

```go
// 查询 FAILED/TIMEOUT 的实例
instances, _ := repo.List(InstanceListFilter{
    TaskID: taskID,
    Status: []InstanceStatus{FAILED, TIMEOUT},
})
// 逐条 CAS 推 PENDING + attempt+1
for _, inst := range instances {
    ok, _ := repo.CASStatus(inst.ID, inst.Status, PENDING)
    if ok {
        inst.Attempt++
        inst.ConversationID = ""
        inst.MessageID = ""
        repo.Update(inst)
        // 走 high queue Enqueue
        asynqClient.EnqueueRunInstance(inst.ID, inst.Attempt, true)
    }
}
```

---

## 5. API Layer（API 层，规划中）

> ⚠️ 本层尚未实现，以下为设计方案。

API 层由 Handler 实现，位于 `api/handlers/eval.go`，统一在 `api/routers/route.go: InitRouter` 注册。

### 5.1 API 端点清单（15 个）

| 分组 | 方法 | 路径 | 说明 |
|------|------|------|------|
| **用例** | POST | `/api/eval/cases/upload-content` | 上传脚本文件 |
| | POST | `/api/eval/cases` | 创建用例 |
| | GET | `/api/eval/cases` | 列表 |
| | GET | `/api/eval/cases/:id` | 详情 |
| | PUT | `/api/eval/cases/:id` | 更新 |
| | DELETE | `/api/eval/cases/:id` | 删除 |
| **任务** | POST | `/api/eval/tasks` | 创建任务 |
| | GET | `/api/eval/tasks` | 列表 |
| | GET | `/api/eval/tasks/:id` | 详情 |
| | POST | `/api/eval/tasks/:id/retry` | 重试失败实例 |
| | DELETE | `/api/eval/tasks/:id` | 删除 |
| **实例** | GET | `/api/eval/tasks/:id/instances` | 任务下实例列表 |
| | GET | `/api/eval/instances/:id` | 实例详情 |
| | GET | `/api/eval/instances/:id/trace` | 实例链路 |
| | POST | `/api/eval/instances/:id/retry` | 重试单个实例 |
| | DELETE | `/api/eval/instances/:id` | 删除实例 |
| **辅助** | GET | `/api/eval/agent-configs` | 列出可用 AgentConfig |

### 5.2 Handler 示例

```go
// 计划文件位置: api/handlers/eval.go
type EvalHandler struct {
    svc app.EvaluationApplicationService
}

// @Summary 创建评测任务
// @Tags eval
// @Accept json
// @Produce json
// @Param req body CreateTaskReq true "任务参数"
// @Success 200 {object} TaskView
// @Router /api/eval/tasks [post]
func (h *EvalHandler) CreateTask(c *gin.Context) {
    var req CreateTaskReq
    if err := c.ShouldBindJSON(&req); err != nil {
        response.BadRequest(c, err)
        return
    }
    resp, err := h.svc.CreateTask(c.Request.Context(), &req)
    if err != nil {
        response.Error(c, err)
        return
    }
    response.OK(c, resp)
}
```

### 5.3 路由注册

```go
// api/routers/route.go: InitRouter
evalGroup := r.Group("/api/eval")
{
    evalGroup.POST("/cases/upload-content", evalHandler.UploadContent)
    evalGroup.POST("/cases", evalHandler.CreateCase)
    evalGroup.GET("/cases", evalHandler.ListCases)
    evalGroup.GET("/cases/:id", evalHandler.GetCase)
    evalGroup.PUT("/cases/:id", evalHandler.UpdateCase)
    evalGroup.DELETE("/cases/:id", evalHandler.DeleteCase)

    evalGroup.POST("/tasks", evalHandler.CreateTask)
    evalGroup.GET("/tasks", evalHandler.ListTasks)
    evalGroup.GET("/tasks/:id", evalHandler.GetTask)
    evalGroup.POST("/tasks/:id/retry", evalHandler.RetryTask)
    evalGroup.DELETE("/tasks/:id", evalHandler.DeleteTask)

    evalGroup.GET("/tasks/:id/instances", evalHandler.ListInstances)
    evalGroup.GET("/instances/:id", evalHandler.GetInstance)
    evalGroup.GET("/instances/:id/trace", evalHandler.GetInstanceTrace)
    evalGroup.POST("/instances/:id/retry", evalHandler.RetryInstance)
    evalGroup.DELETE("/instances/:id", evalHandler.DeleteInstance)

    evalGroup.GET("/agent-configs", evalHandler.ListAgentConfigs)
}
```

---

## 6. 关键流程图

### 6.1 完整评测生命周期

```
┌───────────┐
│  Client   │
└─────┬─────┘
      │ POST /api/eval/tasks
      │ {name, case_ids, agent_config_ids}
      ↓
┌───────────────────┐
│   EvalHandler     │
│  (API Layer)      │
└─────────┬─────────┘
          │ CreateTask(req)
          ↓
┌───────────────────────────┐
│  EvaluationApplication    │
│  Service (App Layer)      │
│                           │
│  1. 校验 IDs 存在          │
│  2. 事务：                 │
│     - BatchCreate         │
│       Snapshot × N        │
│     - Create Task         │
│     - Create Instance     │
│       × M × N             │
│  3. 提交事务               │
│  4. 异步 Enqueue          │
└─────────┬─────────────────┘
          │ (M×N 实例入队)
          ↓
┌───────────────────────────┐
│    Asynq Redis Queue      │
└─────────┬─────────────────┘
          │
          ↓
┌───────────────────────────┐
│  RunInstanceHandler       │
│  (Worker, Infra Layer)    │
│                           │
│  1. 令牌桶抢占             │
│  2. 调用 Executor          │
└─────────┬─────────────────┘
          │ Execute(ctx, instanceID)
          ↓
┌───────────────────────────┐
│   InstanceExecutor        │
│   (Domain Service)        │
│                           │
│  CAS: QUEUED→INIT         │
│    ↓                      │
│  Run InitScript            │
│    ↓                      │
│  CAS: INIT→RUNNING        │
│    ↓                      │
│  ChatRunner.Run           │──→ BaseAgent (复用)
│    (agent chat)            │
│    ↓                      │
│  CAS: RUNNING→VERIFYING   │
│    ↓                      │
│  Run VerifyScript          │
│    ↓                      │
│  TraceAggregator          │──→ 从 ai_span 聚合
│  (聚合指标)                │    token/latency
│    ↓                      │
│  CAS: VERIFYING→PASSED    │
│    /FAILED                │
│    ↓                      │
│  Result.Upsert             │
│    ↓                      │
│  Task.RecountAndTransit    │
│    ↓                      │
│  Skill.CleanupMessage     │──→ 清理工作区
└───────────────────────────┘
```

### 6.2 CAS 并发保护流程

```
Worker A                    Worker B (or Cron Sweeper)
   │                            │
   │  CAS(id, RUNNING, VERIFYING)│
   │───────────────┐             │
   │               │             │
   │               │  同时       │  CAS(id, RUNNING, TIMEOUT)
   │               │             │───────────────┐
   ↓               ↓             ↓               ↓
   ┌─────────────────────────────────────────────┐
   │              PostgreSQL                      │
   │  UPDATE ... WHERE id=? AND status='RUNNING' │
   │                                              │
   │  行锁自动串行化，两条 UPDATE 依次执行         │
   │                                              │
   │  Worker A 先到:                              │
   │    RowsAffected = 1, status → VERIFYING     │
   │                                              │
   │  Worker B 后到:                              │
   │    RowsAffected = 0 (status 已变)           │
   └─────────────────────────────────────────────┘
                  ↓                        ↓
       Worker A: 继续 verify        Worker B: 放弃
```

### 6.3 巡检 Cron 三大 Job

```
                    ┌────────────────────┐
                    │  Cron Scheduler    │
                    │  (robfig/cron/v3)  │
                    └──┬─────┬─────┬─────┘
                       │     │     │
        ┌──────────────┘     │     └──────────────┐
        │                    │                    │
        ↓                    ↓                    ↓
┌──────────────┐    ┌──────────────┐    ┌──────────────────┐
│ Sweeper      │    │ Reconciler   │    │  DLQ Archiver    │
│ (每 30s)     │    │ (每 60s)     │    │  (每 5min)       │
│              │    │              │    │                  │
│ ListStale +  │    │ 遍历         │    │ Inspector 拉     │
│ CAS→TIMEOUT  │    │ RUNNING task │    │ eval:dlq         │
│              │    │ RecountAndTr │    │ CAS→FAILED       │
│              │    │ + 补漏       │    │                  │
│              │    │ Enqueue      │    │                  │
└──────────────┘    └──────────────┘    └──────────────────┘
```

### 6.4 Token 消耗回填流程

```
┌───────────────┐
│  LLM SDK      │
│  (openai-go)  │
└──────┬────────┘
       │ Response 含 Usage
       ↓
┌───────────────┐
│  OpenAiLLM    │  (infra 层包装)
│  返回 SDK     │
│  Usage 类型   │
└──────┬────────┘
       ↓
┌───────────────┐
│ OpenAIAdapter │  (Domain 转换)
│ lastUsage 字段│
│ + 互斥锁      │
└──────┬────────┘
       │ LastUsage() → llm.Usage
       ↓
┌───────────────┐
│  BaseAgent    │
│finalizeLLMSpan│
│  Success()    │
│               │
│ SetTag("llm.  │
│  io.prompt_   │
│  units", ...) │
└──────┬────────┘
       │ (tag 写入 Span)
       ↓
┌───────────────┐
│  ai_span 表   │
│  (Postgres)   │
└──────┬────────┘
       │ 评测结束后
       │ TraceAggregator 查询
       ↓
┌───────────────┐
│  Result 表    │
│  prompt_tokens│
│  = SUM(...)   │
└───────────────┘
```

---

## 7. 已实现文件清单

### 7.1 领域层（Domain Layer）

```
internal/domains/models/evaluation/
├── status.go             # TaskStatus / InstanceStatus 枚举 + IsTerminal()
├── case.go               # Case DO
├── task.go               # Task DO
├── run_instance.go       # RunInstance DO
├── result.go             # Result DO
└── agent_snapshot.go     # AgentSnapshot DO

internal/domains/models/llm/
└── usage.go              # Usage 值对象（新增）

internal/domains/services/evaluation/
├── service.go            # EvaluationDomainService 接口
├── service_impl.go       # 骨架实现（M4-M7 待填）
├── state_machine.go      # 状态流转白名单函数
└── state_machine_test.go # 25 个穷举测试用例
```

### 7.2 基础设施层（Infrastructure Layer）

```
internal/infra/models/
├── eval_case.go              # EvalCasePO
├── eval_task.go              # EvalTaskPO
├── eval_run_instance.go      # EvalRunInstancePO
├── eval_result.go            # EvalResultPO
└── eval_agent_snapshot.go    # EvalAgentSnapshotPO

internal/infra/repositories/
├── eval_case.go              # EvalCaseRepository + Impl
├── eval_task.go              # EvalTaskRepository + Impl
├── eval_run_instance.go      # EvalRunInstanceRepository + Impl
├── eval_run_instance_test.go # CAS 并发竞态测试
├── eval_result.go            # EvalResultRepository + Impl
├── eval_agent_snapshot.go    # EvalAgentSnapshotRepository + Impl
└── eval_converter.go         # PO ↔ DO 转换函数

internal/infra/storage/
└── postgres.go               # AutoMigrate 注册 5 张表 + GIN 索引

internal/infra/external/llm/
├── openai.go                 # 底层 SDK 返回 Usage
├── openai_adapter.go         # Adapter 缓存 lastUsage + LastUsage getter
└── anthropic_adapter.go      # 同上（占位实现）

internal/domains/models/invoker/
└── invoker.go                # Invoker 接口新增 LastUsage()

internal/domains/models/tracing/
└── span_usage_masking_test.go # 脱敏正则守护测试

internal/domains/services/agents/
├── base.go                    # finalizeLLMSpanSuccess 写入 usage tag
└── finalize_llm_span_test.go  # 单元测试
```

### 7.3 文件依赖关系图

```
   [Domain Models]         [Domain Services]
        │                       │
        ├──────────────────────►│  (依赖)
        │                       │
        ↓                       ↓
   [Repository Interface]  [State Machine]
        │
        │ (实现)
        ↓
   [Repository Impl] ───► [Persistence Objects] ──► [PostgreSQL]
        │                       │
        │                       │
        └────► [Converter] ◄────┘
                (PO ↔ DO)
```

---

## 8. 关键设计决策

### 8.1 为什么用 CAS 而不是分布式锁？

- **CAS 简单可靠**: PostgreSQL 的行锁天然支持，无需额外基础设施
- **无死锁**: CAS 失败直接返回 false，调用方自行决定重试或放弃
- **性能好**: 单次 UPDATE 原子操作，无需 Redis 或 ZooKeeper

### 8.2 为什么用 Snapshot 而不是引用？

- **可复现性**: 评测历史结果必须可复现，AppConfig 修改不能影响历史
- **数据完整性**: 即使源 AppConfig 被删除，评测记录仍可查看

### 8.3 为什么用 Asynq 而不是直接 `go func()`？

- **可靠投递**: Redis 持久化，进程重启不丢任务
- **限流控制**: 通过 Queue 优先级和 Concurrency 精确控制并发
- **可观测性**: asynqmon 提供 Web UI 观察队列状态
- **DLQ**: 失败任务归档，方便故障排查

### 8.4 为什么 Tag 命名用 `llm.io.*_units` 而不是 `llm.usage.*_tokens`？

- 现有脱敏正则 `(?i)(api[_-]?key|token|password|secret|authorization)` 包含 `token`
- 会导致 `llm.usage.prompt_tokens` 的值被替换为 `***`
- 退化命名 `llm.io.prompt_units` 避免正则命中，同时保持语义清晰

### 8.5 为什么 Repository 接口和实现放同一目录？

- 对齐既有约定（如 `internal/infra/repositories/app_config.go`）
- 避免风格分裂增加认知负担
- 接口和实现在同 package，通过文件分离即可

---

## 9. 测试覆盖

### 9.1 已实现的测试

| 测试文件 | 测试内容 | 用例数 |
|---------|---------|--------|
| `state_machine_test.go` | 状态流转白名单穷举 | 25 |
| `eval_run_instance_test.go` | CAS 并发竞态 | 1 |
| `span_usage_masking_test.go` | 脱敏正则守护 | 1 |
| `finalize_llm_span_test.go` | Span usage tag 写入 | 1 |

### 9.2 测试策略

- **单元测试**: 状态机、转换函数、纯函数逻辑
- **集成测试**: 使用 SQLite in-memory 测试 Repository（跳过 JSONB 兼容性问题时手动建表）
- **并发测试**: CAS 竞态测试，验证只有一方成功

---

## 10. 后续里程碑

| 里程碑 | 说明 |
|--------|------|
| M4 | 核心执行链路（InstanceExecutor / VerifyRunner / TraceAggregator） |
| M5 | Asynq 消息队列基础设施（Client / Server / Handler） |
| M6 | Application 层 + Handler 层 + 路由注册 |
| M7 | Cron 巡检 3 个 Job |
| M8 | 集成测 + 压测 |
| M9 | E2E 验证 + 文档 |

---

**文档版本**: 2026-07-16
**对应里程碑**: M1（数据层）+ M2（状态机）+ M3（Invoker Usage 补强）已完成
**下一里程碑**: M4（核心执行链路）




