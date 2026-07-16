package evaluation

// 本文件汇总 executor_*_test.go 使用的 stub 实现，避免每个测试文件重复。

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/repositories"
)

// ==== stub EvalRunInstanceRepository ====

type casCall struct {
	From ev.InstanceStatus
	To   ev.InstanceStatus
}

type stubInstRepo struct {
	mu sync.Mutex

	inst *ev.RunInstance

	// casReturns：CASStatus 依次返回；下标越界后默认返回 true
	casReturns []bool
	casCalls   []casCall
	casErr     error

	getByIDErr error

	updateTraceCalls []string
	heartbeatCount   atomic.Int32
	getStatusReturn  ev.InstanceStatus
	getStatusErr     error
}

func (r *stubInstRepo) Create(ctx context.Context, inst *ev.RunInstance) error { return nil }
func (r *stubInstRepo) GetByID(ctx context.Context, id string) (*ev.RunInstance, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return r.inst, nil
}
func (r *stubInstRepo) GetStatus(ctx context.Context, id string) (ev.InstanceStatus, error) {
	return r.getStatusReturn, r.getStatusErr
}
func (r *stubInstRepo) List(ctx context.Context, filter repositories.InstanceListFilter, page, size int) ([]*ev.RunInstance, int64, error) {
	return nil, 0, nil
}
func (r *stubInstRepo) Update(ctx context.Context, inst *ev.RunInstance) error { return nil }
func (r *stubInstRepo) Delete(ctx context.Context, id string) error            { return nil }
func (r *stubInstRepo) UpdateTraceID(ctx context.Context, id, traceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateTraceCalls = append(r.updateTraceCalls, traceID)
	return nil
}
func (r *stubInstRepo) ListStaleInstances(ctx context.Context, before time.Time) ([]*ev.RunInstance, error) {
	return nil, nil
}
func (r *stubInstRepo) UpdateHeartbeat(ctx context.Context, id, workerID string, now time.Time) error {
	r.heartbeatCount.Add(1)
	return nil
}
func (r *stubInstRepo) CASStatus(ctx context.Context, id string, from, to ev.InstanceStatus) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := len(r.casCalls)
	r.casCalls = append(r.casCalls, casCall{From: from, To: to})
	if r.casErr != nil {
		return false, r.casErr
	}
	if idx < len(r.casReturns) {
		return r.casReturns[idx], nil
	}
	return true, nil
}

// ==== stub EvalTaskRepository ====

type stubTaskRepo struct {
	recountCalls atomic.Int32
	recountErr   error
}

func (r *stubTaskRepo) Create(ctx context.Context, t *ev.Task) error { return nil }
func (r *stubTaskRepo) Get(ctx context.Context, id string) (*ev.Task, error) {
	return nil, nil
}
func (r *stubTaskRepo) List(ctx context.Context, filter repositories.TaskListFilter, page, size int) ([]*ev.Task, int64, error) {
	return nil, 0, nil
}
func (r *stubTaskRepo) Update(ctx context.Context, t *ev.Task) error { return nil }
func (r *stubTaskRepo) Delete(ctx context.Context, id string) error  { return nil }
func (r *stubTaskRepo) RecountAndTransit(ctx context.Context, taskID string) error {
	r.recountCalls.Add(1)
	return r.recountErr
}

// ==== stub EvalResultRepository ====

type stubResultRepo struct {
	mu       sync.Mutex
	upserted []*ev.Result
	upErr    error
}

func (r *stubResultRepo) Create(ctx context.Context, res *ev.Result) error { return nil }
func (r *stubResultRepo) Get(ctx context.Context, instanceID string) (*ev.Result, error) {
	return nil, nil
}
func (r *stubResultRepo) Upsert(ctx context.Context, res *ev.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserted = append(r.upserted, res)
	return r.upErr
}
func (r *stubResultRepo) lastResult() *ev.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.upserted) == 0 {
		return nil
	}
	return r.upserted[len(r.upserted)-1]
}

// ==== stub EvalAgentSnapshotRepository ====

type stubSnapshotRepo struct {
	snap *ev.AgentSnapshot
	err  error
}

func (r *stubSnapshotRepo) Create(ctx context.Context, s *ev.AgentSnapshot) error { return nil }
func (r *stubSnapshotRepo) Get(ctx context.Context, id string) (*ev.AgentSnapshot, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.snap != nil {
		return r.snap, nil
	}
	// 默认返回一个最小可用 snapshot（chat 桩不做类型断言）
	return &ev.AgentSnapshot{ID: id}, nil
}
func (r *stubSnapshotRepo) Delete(ctx context.Context, id string) error { return nil }
func (r *stubSnapshotRepo) BatchCreate(ctx context.Context, snapshots []*ev.AgentSnapshot) error {
	return nil
}

// ==== stub InternalChatRunner ====

type stubChatRunner struct {
	res  InternalChatResult
	err  error
	last InternalChatReq
	// 若 delay > 0 则 Run 会 sleep 该时长模拟耗时
	delay time.Duration
}

func (c *stubChatRunner) Run(ctx context.Context, req InternalChatReq) (InternalChatResult, error) {
	c.last = req
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
		}
	}
	return c.res, c.err
}

// ==== stub SkillExecutor ====

type stubSkillExecutor struct {
	cleaned []string
	failErr error
	mu      sync.Mutex
}

func (s *stubSkillExecutor) Execute(ctx tools.SkillExecutionContext, bashCommand string) ([]tools.SkillExecutionResult, error) {
	return nil, nil
}
func (s *stubSkillExecutor) CleanupMessage(messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleaned = append(s.cleaned, messageID)
	return s.failErr
}

// ==== stub NativeToolsProvider ====

type stubNativeProvider struct {
	workspaceDir string
	cleaned      []string
	failErr      error
	mu           sync.Mutex
}

func (p *stubNativeProvider) BuildTools(messageId, conversationId string) ([]tools.Tool, error) {
	return nil, nil
}
func (p *stubNativeProvider) Cleanup(messageId string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleaned = append(p.cleaned, messageId)
	return p.failErr
}
func (p *stubNativeProvider) ConversationPlanDir(conversationId string) string {
	return ""
}
func (p *stubNativeProvider) MessageWorkspaceDir(messageId string) string {
	return p.workspaceDir
}
