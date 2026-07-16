package evaluation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/repositories"
)

// ErrCaseHasRunningReferences DeleteCase 前置校验：case 仍被进行中的 task/instance 引用。
// Application 层可通过 errors.Is 判定该 sentinel。
var ErrCaseHasRunningReferences = errors.New("case has running references")

// ErrInstanceNotRetryable RetryInstance 前置校验：仅 FAILED / TIMEOUT 可重试。
var ErrInstanceNotRetryable = errors.New("instance status not retryable")

// ErrInstanceNotDeletable DeleteInstance 前置校验：仅终态或 PENDING 可删除。
var ErrInstanceNotDeletable = errors.New("instance status not deletable")

// ErrExecutorNotWired ExecuteInstance 时 executor 未注入。
var ErrExecutorNotWired = errors.New("instance executor not wired")

type serviceImpl struct {
	caseRepo     repositories.EvalCaseRepository
	taskRepo     repositories.EvalTaskRepository
	instanceRepo repositories.EvalRunInstanceRepository
	resultRepo   repositories.EvalResultRepository
	snapshotRepo repositories.EvalAgentSnapshotRepository

	// 通过抽象反转依赖：evaluation 包不直接依赖父 services 包 / asynq。
	appConfigLoader AppConfigLoader
	executor        *InstanceExecutor
	dlqInspector    DLQInspector

	logger *zap.Logger
}

// NewEvaluationDomainService 构造 EvaluationDomainService。
// 说明：
//   - appConfigLoader / executor / dlqInspector 可为 nil，对应能力将降级：
//     未注入 appConfigLoader → CreateTask 返回错误；
//     未注入 executor → ExecuteInstance 返回 ErrExecutorNotWired；
//     未注入 dlqInspector → ArchiveDeadTasks 返回 (0, nil)。
//   - logger 为 nil 时使用 zap.NewNop()。
func NewEvaluationDomainService(
	caseRepo repositories.EvalCaseRepository,
	taskRepo repositories.EvalTaskRepository,
	instanceRepo repositories.EvalRunInstanceRepository,
	resultRepo repositories.EvalResultRepository,
	snapshotRepo repositories.EvalAgentSnapshotRepository,
	appConfigLoader AppConfigLoader,
	executor *InstanceExecutor,
	dlqInspector DLQInspector,
	logger *zap.Logger,
) EvaluationDomainService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &serviceImpl{
		caseRepo:        caseRepo,
		taskRepo:        taskRepo,
		instanceRepo:    instanceRepo,
		resultRepo:      resultRepo,
		snapshotRepo:    snapshotRepo,
		appConfigLoader: appConfigLoader,
		executor:        executor,
		dlqInspector:    dlqInspector,
		logger:          logger,
	}
}

// ================ 用例 (Case) ================

// CreateCase 分配 UUID 并写库。CreatedAt/UpdatedAt 由 domain 层统一填充。
func (s *serviceImpl) CreateCase(ctx context.Context, c *ev.Case) (*ev.Case, error) {
	if c == nil {
		return nil, errors.New("nil case")
	}
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if err := s.caseRepo.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create case: %w", err)
	}
	return c, nil
}

// UpdateCase 前置校验存在后再更新，UpdatedAt 刷新为当前时间。
func (s *serviceImpl) UpdateCase(ctx context.Context, c *ev.Case) (*ev.Case, error) {
	if c == nil || c.ID == "" {
		return nil, errors.New("case id required")
	}
	existing, err := s.caseRepo.Get(ctx, c.ID)
	if err != nil {
		return nil, fmt.Errorf("get case: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("case %s not found", c.ID)
	}
	// 保留原 CreatedAt，避免被零值覆盖
	if c.CreatedAt.IsZero() {
		c.CreatedAt = existing.CreatedAt
	}
	c.UpdatedAt = time.Now()
	if err := s.caseRepo.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("update case: %w", err)
	}
	return c, nil
}

// DeleteCase 前置校验：有活引用 → 返回 ErrCaseHasRunningReferences。
func (s *serviceImpl) DeleteCase(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("case id required")
	}
	exists, err := s.caseRepo.ExistsRunningReferences(ctx, id)
	if err != nil {
		return fmt.Errorf("check case running references: %w", err)
	}
	if exists {
		return ErrCaseHasRunningReferences
	}
	return s.caseRepo.Delete(ctx, id)
}

func (s *serviceImpl) ListCases(ctx context.Context, filter repositories.CaseListFilter, page, size int) ([]*ev.Case, int64, error) {
	return s.caseRepo.List(ctx, filter, page, size)
}

func (s *serviceImpl) GetCase(ctx context.Context, id string) (*ev.Case, error) {
	return s.caseRepo.Get(ctx, id)
}

// ================ 任务 (Task) ================

// CreateTask 装配一个新的评测任务：
//  1. 校验入参非空；
//  2. 逐个加载 AppConfig 并冻结成 AgentSnapshot（M×N 中 N 个）；
//  3. 批量写 snapshot；
//  4. 逐个加载 Case（M×N 中 M 个）；
//  5. 组装 Task 与 M×N 个 RunInstance；
//  6. 写 Task → 写 Instances（非事务，失败仅日志；生产可通过巡检收敛）。
//
// 不 Enqueue：Enqueue 由 Application 层控制，避免 domain 依赖 mqClient。
func (s *serviceImpl) CreateTask(ctx context.Context, name string, caseIDs, agentConfigIDs []string) (*ev.Task, error) {
	if name == "" {
		return nil, errors.New("task name required")
	}
	if len(caseIDs) == 0 {
		return nil, errors.New("at least one case required")
	}
	if len(agentConfigIDs) == 0 {
		return nil, errors.New("at least one agent config required")
	}
	if s.appConfigLoader == nil {
		return nil, errors.New("app config loader not wired")
	}

	// Step 1: 加载所有 AppConfig 并冻结成 snapshot
	// snapshot 索引与 agentConfigIDs 一一对应，便于 instance 组装时按位取用。
	snapshots := make([]*ev.AgentSnapshot, 0, len(agentConfigIDs))
	for _, cfgID := range agentConfigIDs {
		cfg, err := s.appConfigLoader.GetById(cfgID)
		if err != nil {
			return nil, fmt.Errorf("load app config %s: %w", cfgID, err)
		}
		snap, err := FreezeAppConfig(&cfg)
		if err != nil {
			return nil, fmt.Errorf("freeze app config %s: %w", cfgID, err)
		}
		snapshots = append(snapshots, snap)
	}

	// Step 2: 批量写 snapshot（先落库，避免 instance 引用悬空）
	if err := s.snapshotRepo.BatchCreate(ctx, snapshots); err != nil {
		return nil, fmt.Errorf("batch create snapshots: %w", err)
	}

	// Step 3: 加载所有 Case
	cases := make([]*ev.Case, 0, len(caseIDs))
	for _, cid := range caseIDs {
		c, err := s.caseRepo.Get(ctx, cid)
		if err != nil {
			return nil, fmt.Errorf("load case %s: %w", cid, err)
		}
		if c == nil {
			return nil, fmt.Errorf("case %s not found", cid)
		}
		cases = append(cases, c)
	}

	// Step 4: 组装 Task
	now := time.Now()
	task := &ev.Task{
		ID:             uuid.NewString(),
		Name:           name,
		CaseIDs:        append([]string(nil), caseIDs...),
		AgentConfigIDs: append([]string(nil), agentConfigIDs...),
		Status:         ev.TaskStatusPending,
		TotalCount:     len(caseIDs) * len(agentConfigIDs),
		CreatedAt:      now,
	}

	// Step 5: 组装 M×N 个 RunInstance
	instances := make([]*ev.RunInstance, 0, task.TotalCount)
	for _, c := range cases {
		for i, snap := range snapshots {
			_ = i
			inst := &ev.RunInstance{
				ID:                    uuid.NewString(),
				TaskID:                task.ID,
				CaseID:                c.ID,
				CaseSnapshot:          *c, // 值拷贝：Case 全部为值/字符串/切片，切片长度已定
				AgentConfigSnapshotID: snap.ID,
				Status:                ev.InstanceStatusPending,
				Attempt:               0,
				ConversationID:        uuid.NewString(),
				MessageID:             uuid.NewString(),
			}
			instances = append(instances, inst)
		}
	}

	// Step 6: 落库 —— 先 Task，再 Instances（非事务；失败仅记录日志）
	if err := s.taskRepo.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	for _, inst := range instances {
		if err := s.instanceRepo.Create(ctx, inst); err != nil {
			// 兜底：巡检层可基于 task 总数与实际 instance 数比对
			s.logger.Warn("create run instance failed",
				zap.String("task_id", task.ID),
				zap.String("instance_id", inst.ID),
				zap.Error(err))
			return nil, fmt.Errorf("create run instance %s: %w", inst.ID, err)
		}
	}

	return task, nil
}

func (s *serviceImpl) ListTasks(ctx context.Context, filter repositories.TaskListFilter, page, size int) ([]*ev.Task, int64, error) {
	return s.taskRepo.List(ctx, filter, page, size)
}

func (s *serviceImpl) GetTask(ctx context.Context, id string) (*ev.Task, error) {
	return s.taskRepo.Get(ctx, id)
}

// RetryTaskFailedInstances 把 task 下 FAILED / TIMEOUT 状态的 instance 全部推回 PENDING。
// 返回成功推进的实例数。Enqueue 由 Application 层负责（domain 不依赖 mq）。
func (s *serviceImpl) RetryTaskFailedInstances(ctx context.Context, id string) (int, error) {
	if id == "" {
		return 0, errors.New("task id required")
	}
	// 使用 StatusIn 一次性拉出可重试的实例（size 走一个大页码，实际生产 M×N 有限）
	insts, _, err := s.instanceRepo.List(ctx, repositories.InstanceListFilter{
		TaskID: id,
		StatusIn: []ev.InstanceStatus{
			ev.InstanceStatusFailed,
			ev.InstanceStatusTimeout,
		},
	}, 1, 10000)
	if err != nil {
		return 0, fmt.Errorf("list failed instances: %w", err)
	}

	count := 0
	for _, inst := range insts {
		// CAS 从原状态 → PENDING
		ok, cerr := s.instanceRepo.CASStatus(ctx, inst.ID, inst.Status, ev.InstanceStatusPending)
		if cerr != nil {
			s.logger.Warn("CAS status failed on retry",
				zap.String("instance_id", inst.ID), zap.Error(cerr))
			continue
		}
		if !ok {
			// 状态已被其他流程推进，跳过
			continue
		}
		// attempt+1，并清理时间戳字段
		inst.Attempt++
		inst.Status = ev.InstanceStatusPending
		inst.QueuedAt = nil
		inst.StartedAt = nil
		inst.FinishedAt = nil
		inst.HeartbeatAt = nil
		if uerr := s.instanceRepo.Update(ctx, inst); uerr != nil {
			s.logger.Warn("update instance after retry CAS failed",
				zap.String("instance_id", inst.ID), zap.Error(uerr))
			continue
		}
		count++
	}
	return count, nil
}

// DeleteTask 删除任务及关联数据：
//  1. 先 List instances 收集 snapshotIDs（去重）；
//  2. Delete task —— 依赖 CASCADE 删 instance / result；
//  3. 逐个 Delete snapshot（restrictive 关系，无级联）。
func (s *serviceImpl) DeleteTask(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("task id required")
	}

	// 收集去重的 snapshotIDs
	insts, _, err := s.instanceRepo.List(ctx, repositories.InstanceListFilter{TaskID: id}, 1, 10000)
	if err != nil {
		return fmt.Errorf("list task instances: %w", err)
	}
	snapIDSet := make(map[string]struct{}, len(insts))
	for _, inst := range insts {
		if inst.AgentConfigSnapshotID != "" {
			snapIDSet[inst.AgentConfigSnapshotID] = struct{}{}
		}
	}

	// 删 task（CASCADE 干掉 instance / result）
	if err := s.taskRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete task: %w", err)
	}

	// 逐一删 snapshot（restrictive 无 CASCADE）
	for sid := range snapIDSet {
		if derr := s.snapshotRepo.Delete(ctx, sid); derr != nil {
			// 已经删完 task，这里只记日志，不阻塞返回（残留 snapshot 靠离线巡检清理）
			s.logger.Warn("delete snapshot failed",
				zap.String("snapshot_id", sid), zap.Error(derr))
		}
	}
	return nil
}

// ================ 实例 (RunInstance) ================

func (s *serviceImpl) ListInstances(ctx context.Context, taskID string, filter repositories.InstanceListFilter, page, size int) ([]*ev.RunInstance, int64, error) {
	// taskID 传入优先于 filter 内部值，避免调用方遗忘同步
	if taskID != "" {
		filter.TaskID = taskID
	}
	return s.instanceRepo.List(ctx, filter, page, size)
}

func (s *serviceImpl) GetInstance(ctx context.Context, id string) (*ev.RunInstance, error) {
	return s.instanceRepo.GetByID(ctx, id)
}

// RetryInstance 单实例重试：仅允许 FAILED / TIMEOUT。
// CAS 成功后 attempt+1、清时间戳。不 Enqueue（Application 层负责）。
func (s *serviceImpl) RetryInstance(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("instance id required")
	}
	inst, err := s.instanceRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if inst == nil {
		return fmt.Errorf("instance %s not found", id)
	}
	if inst.Status != ev.InstanceStatusFailed && inst.Status != ev.InstanceStatusTimeout {
		return ErrInstanceNotRetryable
	}
	ok, cerr := s.instanceRepo.CASStatus(ctx, id, inst.Status, ev.InstanceStatusPending)
	if cerr != nil {
		return fmt.Errorf("cas status: %w", cerr)
	}
	if !ok {
		return ErrInstanceNotRetryable
	}
	inst.Attempt++
	inst.Status = ev.InstanceStatusPending
	inst.QueuedAt = nil
	inst.StartedAt = nil
	inst.FinishedAt = nil
	inst.HeartbeatAt = nil
	if uerr := s.instanceRepo.Update(ctx, inst); uerr != nil {
		return fmt.Errorf("update instance after cas: %w", uerr)
	}
	return nil
}

// DeleteInstance 只允许终态或 PENDING 删除；进行中的实例禁止删除。
func (s *serviceImpl) DeleteInstance(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("instance id required")
	}
	inst, err := s.instanceRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if inst == nil {
		return fmt.Errorf("instance %s not found", id)
	}
	if !inst.Status.IsTerminal() && inst.Status != ev.InstanceStatusPending {
		return ErrInstanceNotDeletable
	}
	return s.instanceRepo.Delete(ctx, id)
}

// ================ Worker 入口 ================

// ExecuteInstance 转发给注入的 InstanceExecutor。
// workerID 参数保留仅为语义完整；实际使用的 workerID 已在 executor 构造时冻结。
func (s *serviceImpl) ExecuteInstance(ctx context.Context, instanceID string, workerID string) error {
	if s.executor == nil {
		return ErrExecutorNotWired
	}
	_ = workerID // 语义占位：executor 内部已持有 workerID
	return s.executor.Execute(ctx, instanceID)
}

// ================ 巡检 (M7) ================

// SweepStaleInstances 心跳/deadline 过期收敛：
//   - ListStaleInstances(before=now)；
//   - 对每条：CAS 当前状态 → TIMEOUT；成功则 Upsert result + task recount。
func (s *serviceImpl) SweepStaleInstances(ctx context.Context) (int, error) {
	now := time.Now()
	insts, err := s.instanceRepo.ListStaleInstances(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("list stale instances: %w", err)
	}
	count := 0
	for _, inst := range insts {
		ok, cerr := s.instanceRepo.CASStatus(ctx, inst.ID, inst.Status, ev.InstanceStatusTimeout)
		if cerr != nil {
			s.logger.Warn("cas timeout failed",
				zap.String("instance_id", inst.ID), zap.Error(cerr))
			continue
		}
		if !ok {
			// 状态已被 executor / 其他巡检推进
			continue
		}
		// Upsert result（幂等）+ recount
		res := &ev.Result{
			InstanceID: inst.ID,
			Passed:     false,
			ErrorLog:   "worker_heartbeat_stale_or_deadline_reached",
			FinishedAt: time.Now(),
		}
		if uerr := s.resultRepo.Upsert(ctx, res); uerr != nil {
			s.logger.Warn("upsert result failed on sweep",
				zap.String("instance_id", inst.ID), zap.Error(uerr))
			// 继续 recount，让上层可感知
		}
		if rerr := s.taskRepo.RecountAndTransit(ctx, inst.TaskID); rerr != nil {
			s.logger.Warn("recount task failed on sweep",
				zap.String("task_id", inst.TaskID), zap.Error(rerr))
		}
		count++
	}
	return count, nil
}

// ReconcileTaskStatuses 对 PENDING / RUNNING 的 task 逐个 RecountAndTransit。
// 使用 StatusIn 一次性拉出，减少往返。返回处理数。
func (s *serviceImpl) ReconcileTaskStatuses(ctx context.Context) (int, error) {
	tasks, _, err := s.taskRepo.List(ctx, repositories.TaskListFilter{
		StatusIn: []ev.TaskStatus{
			ev.TaskStatusPending,
			ev.TaskStatusRunning,
		},
	}, 1, 10000)
	if err != nil {
		return 0, fmt.Errorf("list tasks for reconcile: %w", err)
	}
	count := 0
	for _, t := range tasks {
		if rerr := s.taskRepo.RecountAndTransit(ctx, t.ID); rerr != nil {
			s.logger.Warn("recount task failed",
				zap.String("task_id", t.ID), zap.Error(rerr))
			continue
		}
		count++
	}
	return count, nil
}

// ArchiveDeadTasks 从 asynq DLQ 拿出已归档的实例 → 落 FAILED + result + recount。
// 若 dlqInspector 未注入则降级为 stub 返回 (0, nil)。
func (s *serviceImpl) ArchiveDeadTasks(ctx context.Context) (int, error) {
	if s.dlqInspector == nil {
		s.logger.Warn("dlq inspector not wired, archive dead tasks skipped")
		return 0, nil
	}
	ids, err := s.dlqInspector.ListArchivedRunInstanceIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list archived run instance ids: %w", err)
	}
	count := 0
	for _, id := range ids {
		inst, gerr := s.instanceRepo.GetByID(ctx, id)
		if gerr != nil {
			s.logger.Warn("get archived instance failed",
				zap.String("instance_id", id), zap.Error(gerr))
			continue
		}
		if inst == nil || inst.Status.IsTerminal() {
			continue
		}
		ok, cerr := s.instanceRepo.CASStatus(ctx, id, inst.Status, ev.InstanceStatusFailed)
		if cerr != nil || !ok {
			continue
		}
		_ = s.resultRepo.Upsert(ctx, &ev.Result{
			InstanceID: id,
			Passed:     false,
			ErrorLog:   "asynq_archived_dead_letter",
			FinishedAt: time.Now(),
		})
		_ = s.taskRepo.RecountAndTransit(ctx, inst.TaskID)
		count++
	}
	return count, nil
}
