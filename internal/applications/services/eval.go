package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"sync"
	"unicode/utf8"

	"go.uber.org/zap"

	"mooc-manus/config"
	"mooc-manus/internal/applications/dtos"
	ev "mooc-manus/internal/domains/models/evaluation"
	domsvc "mooc-manus/internal/domains/services"
	evalsvc "mooc-manus/internal/domains/services/evaluation"
	"mooc-manus/internal/infra/repositories"
)

// ErrUploadTooLarge 上传内容超过配置上限
var ErrUploadTooLarge = errors.New("上传文件超出上限")

// ErrUploadNotUTF8 上传内容非 UTF-8 文本
var ErrUploadNotUTF8 = errors.New("上传文件必须为 UTF-8 文本")

// MQEnqueuer 抽象 asynq 客户端，便于 application 层测试。
// 生产实现由 mq.Client 提供；单元测试可注入 fake。
type MQEnqueuer interface {
	EnqueueRunInstance(ctx context.Context, instanceID string, attempt int, useHigh bool) error
}

// EvaluationApplicationService 评测应用层入口。
// 负责 DTO ↔ Domain 转换 + Enqueue 编排（Domain 层不依赖 mq）。
type EvaluationApplicationService interface {
	// Case
	UploadContent(ctx context.Context, file *multipart.FileHeader) (*dtos.UploadContentResp, error)
	CreateCase(ctx context.Context, req *dtos.CaseCreateRequest) (*dtos.CaseView, error)
	UpdateCase(ctx context.Context, req *dtos.CaseUpdateRequest) (*dtos.CaseView, error)
	ListCases(ctx context.Context, q *dtos.ListCasesQuery) (*dtos.ListPage[dtos.CaseView], error)
	GetCase(ctx context.Context, id string) (*dtos.CaseView, error)
	DeleteCase(ctx context.Context, id string) error

	// Task
	CreateTask(ctx context.Context, req *dtos.TaskCreateRequest) (*dtos.TaskView, error)
	ListTasks(ctx context.Context, q *dtos.ListTasksQuery) (*dtos.ListPage[dtos.TaskView], error)
	GetTask(ctx context.Context, id string) (*dtos.TaskView, error)
	RetryTask(ctx context.Context, id string) (*dtos.RetryTaskResp, error)
	DeleteTask(ctx context.Context, id string) error

	// Instance
	ListInstances(ctx context.Context, taskID string, q *dtos.ListInstancesQuery) (*dtos.ListPage[dtos.InstanceView], error)
	GetInstance(ctx context.Context, id string) (*dtos.InstanceView, error)
	GetInstanceTraceID(ctx context.Context, id string) (string, error)
	RetryInstance(ctx context.Context, id string) error
	DeleteInstance(ctx context.Context, id string) error

	// AgentConfigs
	ListAgentConfigs(ctx context.Context) ([]dtos.AgentConfigView, error)
}

type evalAppImpl struct {
	domain          evalsvc.EvaluationDomainService
	caseRepo        repositories.EvalCaseRepository
	taskRepo        repositories.EvalTaskRepository
	instRepo        repositories.EvalRunInstanceRepository
	resultRepo      repositories.EvalResultRepository
	appConfigDomain domsvc.AppConfigDomainService
	mq              MQEnqueuer
	evalCfg         config.EvaluationConfig
	logger          *zap.Logger
}

// NewEvaluationApplicationService 装配评测 Application Service。
// mq 允许 nil（例如 Evaluation.Enabled=false 时），此时 CreateTask/RetryTask/RetryInstance 会跳过 enqueue，
// 由 cron 巡检层兜底重新触发。
func NewEvaluationApplicationService(
	domain evalsvc.EvaluationDomainService,
	caseRepo repositories.EvalCaseRepository,
	taskRepo repositories.EvalTaskRepository,
	instRepo repositories.EvalRunInstanceRepository,
	resultRepo repositories.EvalResultRepository,
	appConfigDomain domsvc.AppConfigDomainService,
	mq MQEnqueuer,
	evalCfg config.EvaluationConfig,
	logger *zap.Logger,
) EvaluationApplicationService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &evalAppImpl{
		domain: domain, caseRepo: caseRepo, taskRepo: taskRepo, instRepo: instRepo, resultRepo: resultRepo,
		appConfigDomain: appConfigDomain, mq: mq, evalCfg: evalCfg, logger: logger,
	}
}

// ============ Case ============

// UploadContent 读取上传文件为 UTF-8 文本，用于前端把脚本 / prompt 内容回显到编辑器。
// 校验：大小 ≤ evalCfg.UploadContentMaxBytes；内容必须 UTF-8。
func (s *evalAppImpl) UploadContent(ctx context.Context, file *multipart.FileHeader) (*dtos.UploadContentResp, error) {
	if file == nil {
		return nil, errors.New("file required")
	}
	if s.evalCfg.UploadContentMaxBytes > 0 && file.Size > s.evalCfg.UploadContentMaxBytes {
		return nil, ErrUploadTooLarge
	}
	f, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open upload file: %w", err)
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read upload file: %w", err)
	}
	if !utf8.Valid(body) {
		return nil, ErrUploadNotUTF8
	}
	return &dtos.UploadContentResp{Content: string(body), Size: len(body)}, nil
}

// CreateCase 转发到 domain；domain 内部会补 ID / 时间戳。
func (s *evalAppImpl) CreateCase(ctx context.Context, req *dtos.CaseCreateRequest) (*dtos.CaseView, error) {
	do := &ev.Case{
		Name:         req.Name,
		Description:  req.Description,
		InitScript:   req.InitScript,
		TaskPrompt:   req.TaskPrompt,
		VerifyScript: req.VerifyScript,
		Tags:         append([]string(nil), req.Tags...),
	}
	out, err := s.domain.CreateCase(ctx, do)
	if err != nil {
		return nil, err
	}
	return caseDOToView(out), nil
}

// UpdateCase 按 req 中非 nil 字段覆盖后再交由 domain 更新。
func (s *evalAppImpl) UpdateCase(ctx context.Context, req *dtos.CaseUpdateRequest) (*dtos.CaseView, error) {
	existing, err := s.domain.GetCase(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("case %s not found", req.ID)
	}
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.InitScript != nil {
		existing.InitScript = *req.InitScript
	}
	if req.TaskPrompt != nil {
		existing.TaskPrompt = *req.TaskPrompt
	}
	if req.VerifyScript != nil {
		existing.VerifyScript = *req.VerifyScript
	}
	if req.Tags != nil {
		existing.Tags = append([]string(nil), (*req.Tags)...)
	}
	out, err := s.domain.UpdateCase(ctx, existing)
	if err != nil {
		return nil, err
	}
	return caseDOToView(out), nil
}

// ListCases 列表 + 分页 → View 列表。
func (s *evalAppImpl) ListCases(ctx context.Context, q *dtos.ListCasesQuery) (*dtos.ListPage[dtos.CaseView], error) {
	page, size := normalizePage(q.Page, q.Size, 20)
	filter := repositories.CaseListFilter{
		NameLike: q.NameLike,
		Tags:     append([]string(nil), q.Tags...),
	}
	cases, total, err := s.domain.ListCases(ctx, filter, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]dtos.CaseView, 0, len(cases))
	for _, c := range cases {
		items = append(items, *caseDOToView(c))
	}
	return &dtos.ListPage[dtos.CaseView]{Items: items, Total: total, Page: page, Size: size}, nil
}

// GetCase 单条查询。
func (s *evalAppImpl) GetCase(ctx context.Context, id string) (*dtos.CaseView, error) {
	c, err := s.domain.GetCase(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("case %s not found", id)
	}
	return caseDOToView(c), nil
}

// DeleteCase 转发；ErrCaseHasRunningReferences sentinel 由 handler 转 409。
func (s *evalAppImpl) DeleteCase(ctx context.Context, id string) error {
	return s.domain.DeleteCase(ctx, id)
}

// ============ Task ============

// CreateTask 编排：domain 落 task + instances → application 拉出所有 instance → 并行 Enqueue。
// Enqueue 单点失败不阻塞返回；巡检 cron 会兜底把长时间处于 PENDING 且 queued_at IS NULL 的实例重新推进。
func (s *evalAppImpl) CreateTask(ctx context.Context, req *dtos.TaskCreateRequest) (*dtos.TaskView, error) {
	task, err := s.domain.CreateTask(ctx, req.Name, req.CaseIDs, req.AgentConfigIDs)
	if err != nil {
		return nil, err
	}
	// 拉出该 task 下所有实例（M×N，通常 <= 数百，走大 size 一次拉完）
	insts, _, err := s.instRepo.List(ctx, repositories.InstanceListFilter{TaskID: task.ID}, 1, 10000)
	if err != nil {
		s.logger.Warn("list task instances after create failed",
			zap.String("task_id", task.ID), zap.Error(err))
		return taskDOToView(task), nil
	}
	s.enqueueInstances(ctx, insts, false)
	return taskDOToView(task), nil
}

// ListTasks 列表。
func (s *evalAppImpl) ListTasks(ctx context.Context, q *dtos.ListTasksQuery) (*dtos.ListPage[dtos.TaskView], error) {
	page, size := normalizePage(q.Page, q.Size, 20)
	filter := repositories.TaskListFilter{}
	if q.Status != "" {
		filter.Status = ev.TaskStatus(q.Status)
	}
	tasks, total, err := s.domain.ListTasks(ctx, filter, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]dtos.TaskView, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, *taskDOToView(t))
	}
	return &dtos.ListPage[dtos.TaskView]{Items: items, Total: total, Page: page, Size: size}, nil
}

// GetTask 单条查询。
func (s *evalAppImpl) GetTask(ctx context.Context, id string) (*dtos.TaskView, error) {
	t, err := s.domain.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("task %s not found", id)
	}
	return taskDOToView(t), nil
}

// RetryTask 编排：
//  1. domain.RetryTaskFailedInstances 把 FAILED/TIMEOUT 回退到 PENDING（attempt+1）
//  2. application 拉 attempt>=1 && status=PENDING 的实例，走高优队列 Enqueue
//  3. 返回 domain 返回的 retried_count
func (s *evalAppImpl) RetryTask(ctx context.Context, id string) (*dtos.RetryTaskResp, error) {
	count, err := s.domain.RetryTaskFailedInstances(ctx, id)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		// 拉出 PENDING 状态的实例，attempt>=1 的才是刚 CAS 过的重试项
		insts, _, lerr := s.instRepo.List(ctx, repositories.InstanceListFilter{
			TaskID:   id,
			StatusIn: []ev.InstanceStatus{ev.InstanceStatusPending},
		}, 1, 10000)
		if lerr != nil {
			s.logger.Warn("list pending instances after retry failed",
				zap.String("task_id", id), zap.Error(lerr))
		} else {
			retried := make([]*ev.RunInstance, 0, len(insts))
			for _, inst := range insts {
				if inst.Attempt >= 1 {
					retried = append(retried, inst)
				}
			}
			s.enqueueInstances(ctx, retried, true)
		}
	}
	return &dtos.RetryTaskResp{RetriedCount: count}, nil
}

// DeleteTask 转发。
func (s *evalAppImpl) DeleteTask(ctx context.Context, id string) error {
	return s.domain.DeleteTask(ctx, id)
}

// ============ Instance ============

// ListInstances 分页列表；taskID 从 URL path 注入。
func (s *evalAppImpl) ListInstances(ctx context.Context, taskID string, q *dtos.ListInstancesQuery) (*dtos.ListPage[dtos.InstanceView], error) {
	page, size := normalizePage(q.Page, q.Size, 50)
	filter := repositories.InstanceListFilter{TaskID: taskID}
	if q.Status != "" {
		filter.Status = ev.InstanceStatus(q.Status)
	}
	insts, total, err := s.domain.ListInstances(ctx, taskID, filter, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]dtos.InstanceView, 0, len(insts))
	for _, inst := range insts {
		// 列表页 Result 不填充，节省一次查询
		items = append(items, *instanceDOToView(inst, nil))
	}
	return &dtos.ListPage[dtos.InstanceView]{Items: items, Total: total, Page: page, Size: size}, nil
}

// GetInstance 单条实例；若 Result 存在则一并填充。
func (s *evalAppImpl) GetInstance(ctx context.Context, id string) (*dtos.InstanceView, error) {
	inst, err := s.domain.GetInstance(ctx, id)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, fmt.Errorf("instance %s not found", id)
	}
	// 尝试取 Result；实例未跑完时 Result 不存在，静默处理
	res, rerr := s.resultRepo.Get(ctx, id)
	if rerr != nil {
		// 假设 not found 走 GORM ErrRecordNotFound；此处不区分，直接置 nil
		s.logger.Debug("get instance result missing", zap.String("id", id), zap.Error(rerr))
		res = nil
	}
	return instanceDOToView(inst, res), nil
}

// GetInstanceTraceID 仅返回 trace_id，实例详情页会用它跳转 Trace 详情页。
func (s *evalAppImpl) GetInstanceTraceID(ctx context.Context, id string) (string, error) {
	inst, err := s.domain.GetInstance(ctx, id)
	if err != nil {
		return "", err
	}
	if inst == nil {
		return "", fmt.Errorf("instance %s not found", id)
	}
	return inst.TraceID, nil
}

// RetryInstance 单实例重试：domain 做 CAS，application 拿到 nil error 后 enqueue 高优队列。
func (s *evalAppImpl) RetryInstance(ctx context.Context, id string) error {
	if err := s.domain.RetryInstance(ctx, id); err != nil {
		return err
	}
	if s.mq != nil {
		if err := s.mq.EnqueueRunInstance(ctx, id, 0, true); err != nil {
			s.logger.Warn("enqueue instance after retry failed",
				zap.String("id", id), zap.Error(err))
		}
	}
	return nil
}

// DeleteInstance 转发。
func (s *evalAppImpl) DeleteInstance(ctx context.Context, id string) error {
	return s.domain.DeleteInstance(ctx, id)
}

// ============ AgentConfigs ============

// ListAgentConfigs 仅返回 id / provider / modelName，用于评测任务创建页 selector。
func (s *evalAppImpl) ListAgentConfigs(ctx context.Context) ([]dtos.AgentConfigView, error) {
	list, err := s.appConfigDomain.List()
	if err != nil {
		return nil, err
	}
	views := make([]dtos.AgentConfigView, 0, len(list))
	for _, cfg := range list {
		views = append(views, dtos.AgentConfigView{
			ID:        cfg.AppConfigID,
			ModelName: cfg.ModelConfig.ModelName,
			Provider:  cfg.ModelConfig.Provider,
		})
	}
	return views, nil
}

// ============ helpers ============

// enqueueInstances 并行投递实例任务。
// 单点失败仅日志；用 semaphore 控制并发（默认 8）。
func (s *evalAppImpl) enqueueInstances(ctx context.Context, insts []*ev.RunInstance, useHigh bool) {
	if s.mq == nil || len(insts) == 0 {
		return
	}
	const parallelism = 8
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for _, inst := range insts {
		wg.Add(1)
		sem <- struct{}{}
		go func(instID string, attempt int) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.mq.EnqueueRunInstance(ctx, instID, attempt, useHigh); err != nil {
				s.logger.Warn("enqueue run instance failed",
					zap.String("id", instID), zap.Error(err))
			}
		}(inst.ID, inst.Attempt)
	}
	wg.Wait()
}

// normalizePage 兜底默认值，避免 form 未传时 Offset 变负数。
func normalizePage(page, size, defSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = defSize
	}
	return page, size
}

// caseDOToView Case DO → View
func caseDOToView(c *ev.Case) *dtos.CaseView {
	if c == nil {
		return nil
	}
	return &dtos.CaseView{
		ID:           c.ID,
		Name:         c.Name,
		Description:  c.Description,
		InitScript:   c.InitScript,
		TaskPrompt:   c.TaskPrompt,
		VerifyScript: c.VerifyScript,
		Tags:         append([]string(nil), c.Tags...),
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}

// taskDOToView Task DO → View
func taskDOToView(t *ev.Task) *dtos.TaskView {
	if t == nil {
		return nil
	}
	return &dtos.TaskView{
		ID:             t.ID,
		Name:           t.Name,
		CaseIDs:        append([]string(nil), t.CaseIDs...),
		AgentConfigIDs: append([]string(nil), t.AgentConfigIDs...),
		Status:         string(t.Status),
		TotalCount:     t.TotalCount,
		SucceededCount: t.SucceededCount,
		FailedCount:    t.FailedCount,
		RunningCount:   t.RunningCount,
		CreatedAt:      t.CreatedAt,
		StartedAt:      t.StartedAt,
		FinishedAt:     t.FinishedAt,
	}
}

// instanceDOToView RunInstance DO + 可选 Result → View
func instanceDOToView(inst *ev.RunInstance, res *ev.Result) *dtos.InstanceView {
	if inst == nil {
		return nil
	}
	view := &dtos.InstanceView{
		ID:             inst.ID,
		TaskID:         inst.TaskID,
		CaseID:         inst.CaseID,
		Status:         string(inst.Status),
		Attempt:        inst.Attempt,
		ConversationID: inst.ConversationID,
		MessageID:      inst.MessageID,
		TraceID:        inst.TraceID,
		QueuedAt:       inst.QueuedAt,
		StartedAt:      inst.StartedAt,
		FinishedAt:     inst.FinishedAt,
		HeartbeatAt:    inst.HeartbeatAt,
		DeadlineAt:     inst.DeadlineAt,
		WorkerID:       inst.WorkerID,
		ErrorMessage:   inst.ErrorMessage,
	}
	if res != nil {
		view.Result = &dtos.ResultView{
			InstanceID:       res.InstanceID,
			Passed:           res.Passed,
			VerifyExitCode:   res.VerifyExitCode,
			VerifyStdout:     res.VerifyStdout,
			VerifyStderr:     res.VerifyStderr,
			PromptTokens:     res.PromptTokens,
			CompletionTokens: res.CompletionTokens,
			TotalTokens:      res.TotalTokens,
			AgentLatencyMs:   res.AgentLatencyMs,
			ErrorLog:         res.ErrorLog,
			FinishedAt:       res.FinishedAt,
		}
	}
	return view
}
