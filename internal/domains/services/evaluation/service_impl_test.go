package evaluation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appconfig "mooc-manus/internal/domains/models"
	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/repositories"
)

// ============================================================
// 面向 serviceImpl 测试的内存桩仓储（与 executor_stubs_test.go 中的桩独立）
// 命名前缀 fake* 以避免与 stub* 冲突。
// ============================================================

// fakeCaseRepo 记录 Get/Create/Update/Delete 并支持 ExistsRunningReferences
type fakeCaseRepo struct {
	mu       sync.Mutex
	store    map[string]*ev.Case
	running  map[string]bool // caseID → 是否有活引用
	createErr error
	getErr    error
}

func newFakeCaseRepo() *fakeCaseRepo {
	return &fakeCaseRepo{store: map[string]*ev.Case{}, running: map[string]bool{}}
}

func (r *fakeCaseRepo) Create(ctx context.Context, c *ev.Case) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	// 深拷贝以避免外部修改污染
	copyC := *c
	r.store[c.ID] = &copyC
	return nil
}
func (r *fakeCaseRepo) Get(ctx context.Context, id string) (*ev.Case, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return nil, r.getErr
	}
	c, ok := r.store[id]
	if !ok {
		return nil, fmt.Errorf("case %s not found", id)
	}
	return c, nil
}
func (r *fakeCaseRepo) List(ctx context.Context, filter repositories.CaseListFilter, page, size int) ([]*ev.Case, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := make([]*ev.Case, 0, len(r.store))
	for _, c := range r.store {
		all = append(all, c)
	}
	return all, int64(len(all)), nil
}
func (r *fakeCaseRepo) Update(ctx context.Context, c *ev.Case) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.store[c.ID]; !ok {
		return fmt.Errorf("case %s not found", c.ID)
	}
	copyC := *c
	r.store[c.ID] = &copyC
	return nil
}
func (r *fakeCaseRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.store, id)
	return nil
}
func (r *fakeCaseRepo) ExistsRunningReferences(ctx context.Context, caseID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[caseID], nil
}

// fakeTaskRepo 记录 Create/Update/Delete/Recount
type fakeTaskRepo struct {
	mu           sync.Mutex
	store        map[string]*ev.Task
	recountCalls atomic.Int32
	createErr    error
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{store: map[string]*ev.Task{}}
}
func (r *fakeTaskRepo) Create(ctx context.Context, t *ev.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	copyT := *t
	r.store[t.ID] = &copyT
	return nil
}
func (r *fakeTaskRepo) Get(ctx context.Context, id string) (*ev.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.store[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found", id)
	}
	return t, nil
}
func (r *fakeTaskRepo) List(ctx context.Context, filter repositories.TaskListFilter, page, size int) ([]*ev.Task, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := make([]*ev.Task, 0, len(r.store))
	for _, t := range r.store {
		if len(filter.StatusIn) > 0 {
			match := false
			for _, s := range filter.StatusIn {
				if t.Status == s {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		} else if filter.Status != "" && t.Status != filter.Status {
			continue
		}
		all = append(all, t)
	}
	return all, int64(len(all)), nil
}
func (r *fakeTaskRepo) Update(ctx context.Context, t *ev.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyT := *t
	r.store[t.ID] = &copyT
	return nil
}
func (r *fakeTaskRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.store, id)
	return nil
}
func (r *fakeTaskRepo) RecountAndTransit(ctx context.Context, taskID string) error {
	r.recountCalls.Add(1)
	return nil
}

// fakeInstRepo 内存实现，支持 CAS / List / Get
type fakeInstRepo struct {
	mu      sync.Mutex
	store   map[string]*ev.RunInstance
	stale   []*ev.RunInstance // ListStaleInstances 返回
	casErr  error
}

func newFakeInstRepo() *fakeInstRepo {
	return &fakeInstRepo{store: map[string]*ev.RunInstance{}}
}
func (r *fakeInstRepo) Create(ctx context.Context, inst *ev.RunInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyI := *inst
	r.store[inst.ID] = &copyI
	return nil
}
func (r *fakeInstRepo) GetByID(ctx context.Context, id string) (*ev.RunInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inst, ok := r.store[id]
	if !ok {
		return nil, nil
	}
	return inst, nil
}
func (r *fakeInstRepo) GetStatus(ctx context.Context, id string) (ev.InstanceStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inst, ok := r.store[id]
	if !ok {
		return "", nil
	}
	return inst.Status, nil
}
func (r *fakeInstRepo) List(ctx context.Context, filter repositories.InstanceListFilter, page, size int) ([]*ev.RunInstance, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := make([]*ev.RunInstance, 0, len(r.store))
	for _, inst := range r.store {
		if filter.TaskID != "" && inst.TaskID != filter.TaskID {
			continue
		}
		if len(filter.StatusIn) > 0 {
			match := false
			for _, s := range filter.StatusIn {
				if inst.Status == s {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		} else if filter.Status != "" && inst.Status != filter.Status {
			continue
		}
		all = append(all, inst)
	}
	return all, int64(len(all)), nil
}
func (r *fakeInstRepo) Update(ctx context.Context, inst *ev.RunInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyI := *inst
	r.store[inst.ID] = &copyI
	return nil
}
func (r *fakeInstRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.store, id)
	return nil
}
func (r *fakeInstRepo) UpdateTraceID(ctx context.Context, id, traceID string) error { return nil }
func (r *fakeInstRepo) UpdateQueuedAt(ctx context.Context, id string, queuedAt *time.Time) error {
	return nil
}
func (r *fakeInstRepo) ListStaleInstances(ctx context.Context, before time.Time) ([]*ev.RunInstance, error) {
	return r.stale, nil
}
func (r *fakeInstRepo) UpdateHeartbeat(ctx context.Context, id, workerID string, now time.Time) error {
	return nil
}
func (r *fakeInstRepo) CASStatus(ctx context.Context, id string, from, to ev.InstanceStatus) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.casErr != nil {
		return false, r.casErr
	}
	inst, ok := r.store[id]
	if !ok {
		return false, nil
	}
	if inst.Status != from {
		return false, nil
	}
	inst.Status = to
	return true, nil
}

// fakeResultRepo 内存实现，用于观察 Upsert
type fakeResultRepo struct {
	mu       sync.Mutex
	upserted []*ev.Result
}

func newFakeResultRepo() *fakeResultRepo { return &fakeResultRepo{} }
func (r *fakeResultRepo) Create(ctx context.Context, res *ev.Result) error { return nil }
func (r *fakeResultRepo) Get(ctx context.Context, instanceID string) (*ev.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.upserted {
		if x.InstanceID == instanceID {
			return x, nil
		}
	}
	return nil, nil
}
func (r *fakeResultRepo) Upsert(ctx context.Context, res *ev.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyR := *res
	r.upserted = append(r.upserted, &copyR)
	return nil
}

// fakeSnapshotRepo 内存实现，观察 BatchCreate / Delete
type fakeSnapshotRepo struct {
	mu       sync.Mutex
	store    map[string]*ev.AgentSnapshot
	deleted  []string
	batchErr error
}

func newFakeSnapshotRepo() *fakeSnapshotRepo {
	return &fakeSnapshotRepo{store: map[string]*ev.AgentSnapshot{}}
}
func (r *fakeSnapshotRepo) Create(ctx context.Context, s *ev.AgentSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyS := *s
	r.store[s.ID] = &copyS
	return nil
}
func (r *fakeSnapshotRepo) Get(ctx context.Context, id string) (*ev.AgentSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.store[id]
	if !ok {
		return nil, nil
	}
	return s, nil
}
func (r *fakeSnapshotRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.store, id)
	r.deleted = append(r.deleted, id)
	return nil
}
func (r *fakeSnapshotRepo) BatchCreate(ctx context.Context, snapshots []*ev.AgentSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.batchErr != nil {
		return r.batchErr
	}
	for _, s := range snapshots {
		copyS := *s
		r.store[s.ID] = &copyS
	}
	return nil
}

// fakeAppConfigLoader 内存 AppConfigLoader 实现
type fakeAppConfigLoader struct {
	store map[string]appconfig.AppConfigDO
	err   error
}

func (l *fakeAppConfigLoader) GetById(id string) (appconfig.AppConfigDO, error) {
	if l.err != nil {
		return appconfig.AppConfigDO{}, l.err
	}
	c, ok := l.store[id]
	if !ok {
		return appconfig.AppConfigDO{}, fmt.Errorf("app config %s not found", id)
	}
	return c, nil
}

// helper: 构造一份最小 AppConfigDO
func makeAppConfig(id string) appconfig.AppConfigDO {
	return appconfig.AppConfigDO{
		AppConfigID: id,
		ModelConfig: appconfig.ModelConfig{
			Provider:  "openai",
			BaseUrl:   "https://api",
			ApiKey:    "sk-xxx",
			ModelName: "gpt-4o-mini",
		},
	}
}

// helper: 构造 serviceImpl 及其全部内存桩
func newTestService(t *testing.T) (*serviceImpl, *fakeCaseRepo, *fakeTaskRepo, *fakeInstRepo, *fakeResultRepo, *fakeSnapshotRepo, *fakeAppConfigLoader) {
	t.Helper()
	caseRepo := newFakeCaseRepo()
	taskRepo := newFakeTaskRepo()
	instRepo := newFakeInstRepo()
	resultRepo := newFakeResultRepo()
	snapRepo := newFakeSnapshotRepo()
	loader := &fakeAppConfigLoader{store: map[string]appconfig.AppConfigDO{}}
	svc := NewEvaluationDomainService(
		caseRepo, taskRepo, instRepo, resultRepo, snapRepo,
		loader, nil, nil,
	).(*serviceImpl)
	return svc, caseRepo, taskRepo, instRepo, resultRepo, snapRepo, loader
}

// ============================================================
// 用例测试
// ============================================================

// TestCreateTask_Success 验证：M=2 case × N=2 config → 4 instance + 2 snapshot 全部落库
func TestCreateTask_Success(t *testing.T) {
	svc, caseRepo, taskRepo, instRepo, _, snapRepo, loader := newTestService(t)

	// 准备 2 个 case + 2 个 AppConfig
	c1 := &ev.Case{ID: "case-1", Name: "c1", TaskPrompt: "p1", VerifyScript: "exit 0"}
	c2 := &ev.Case{ID: "case-2", Name: "c2", TaskPrompt: "p2", VerifyScript: "exit 0"}
	_ = caseRepo.Create(context.Background(), c1)
	_ = caseRepo.Create(context.Background(), c2)
	loader.store["cfg-1"] = makeAppConfig("cfg-1")
	loader.store["cfg-2"] = makeAppConfig("cfg-2")

	task, err := svc.CreateTask(context.Background(), "task-A",
		[]string{"case-1", "case-2"}, []string{"cfg-1", "cfg-2"})
	if err != nil {
		t.Fatalf("CreateTask err: %v", err)
	}
	if task == nil || task.ID == "" {
		t.Fatalf("expected task with ID")
	}
	if task.TotalCount != 4 {
		t.Fatalf("expected TotalCount=4, got %d", task.TotalCount)
	}
	if task.Status != ev.TaskStatusPending {
		t.Fatalf("expected status=PENDING, got %s", task.Status)
	}
	// snapshot 落库 2 个
	if len(snapRepo.store) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapRepo.store))
	}
	// instance 落库 4 个
	insts, _, _ := instRepo.List(context.Background(), repositories.InstanceListFilter{TaskID: task.ID}, 1, 100)
	if len(insts) != 4 {
		t.Fatalf("expected 4 instances, got %d", len(insts))
	}
	// 每个 instance 的 CaseSnapshot 需要与 case 一致
	caseHit := map[string]int{"case-1": 0, "case-2": 0}
	snapHit := map[string]int{}
	for _, ins := range insts {
		if ins.Status != ev.InstanceStatusPending {
			t.Fatalf("expected PENDING instance, got %s", ins.Status)
		}
		if ins.ConversationID == "" || ins.MessageID == "" {
			t.Fatalf("conversation/message id should be assigned")
		}
		caseHit[ins.CaseID]++
		snapHit[ins.AgentConfigSnapshotID]++
	}
	if caseHit["case-1"] != 2 || caseHit["case-2"] != 2 {
		t.Fatalf("case hit distribution wrong: %+v", caseHit)
	}
	if len(snapHit) != 2 {
		t.Fatalf("expected 2 unique snapshot ids across instances, got %d (%+v)", len(snapHit), snapHit)
	}
	// task 已落库
	got, _ := taskRepo.Get(context.Background(), task.ID)
	if got == nil {
		t.Fatalf("expected task persisted")
	}
}

// TestCreateTask_CaseNotFound 验证：不存在的 case → 返回错误
// 副作用：snapshot 已提前批量落库（domain 设计选择）。此用例断言不会创建 task / instance。
func TestCreateTask_CaseNotFound(t *testing.T) {
	svc, _, taskRepo, instRepo, _, _, loader := newTestService(t)
	loader.store["cfg-1"] = makeAppConfig("cfg-1")

	_, err := svc.CreateTask(context.Background(), "task-x",
		[]string{"case-missing"}, []string{"cfg-1"})
	if err == nil {
		t.Fatalf("expected error for missing case")
	}
	// task 不应被创建
	if len(taskRepo.store) != 0 {
		t.Fatalf("expected no task created, got %d", len(taskRepo.store))
	}
	// instance 不应被创建
	if len(instRepo.store) != 0 {
		t.Fatalf("expected no instance created, got %d", len(instRepo.store))
	}
}

// TestCreateTask_ConfigNotFound 验证：不存在的 config → 返回错误，且 snapshot 未写库
func TestCreateTask_ConfigNotFound(t *testing.T) {
	svc, caseRepo, taskRepo, instRepo, _, snapRepo, _ := newTestService(t)
	_ = caseRepo.Create(context.Background(), &ev.Case{ID: "case-1"})

	_, err := svc.CreateTask(context.Background(), "task-x",
		[]string{"case-1"}, []string{"cfg-missing"})
	if err == nil {
		t.Fatalf("expected error for missing config")
	}
	if len(snapRepo.store) != 0 || len(taskRepo.store) != 0 || len(instRepo.store) != 0 {
		t.Fatalf("expected no side effects, snap=%d task=%d inst=%d",
			len(snapRepo.store), len(taskRepo.store), len(instRepo.store))
	}
}

// TestDeleteCase_HasRunningReferences 验证：活引用返回 sentinel error
func TestDeleteCase_HasRunningReferences(t *testing.T) {
	svc, caseRepo, _, _, _, _, _ := newTestService(t)
	_ = caseRepo.Create(context.Background(), &ev.Case{ID: "case-1"})
	caseRepo.running["case-1"] = true

	err := svc.DeleteCase(context.Background(), "case-1")
	if !errors.Is(err, ErrCaseHasRunningReferences) {
		t.Fatalf("expected ErrCaseHasRunningReferences, got %v", err)
	}
	// case 未被删除
	if _, gerr := caseRepo.Get(context.Background(), "case-1"); gerr != nil {
		t.Fatalf("case should not be deleted: %v", gerr)
	}
}

// TestDeleteCase_NoRunningReferences 无活引用 → 正常删除
func TestDeleteCase_NoRunningReferences(t *testing.T) {
	svc, caseRepo, _, _, _, _, _ := newTestService(t)
	_ = caseRepo.Create(context.Background(), &ev.Case{ID: "case-1"})

	if err := svc.DeleteCase(context.Background(), "case-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, gerr := caseRepo.Get(context.Background(), "case-1"); gerr == nil {
		t.Fatalf("case should have been deleted")
	}
}

// TestRetryTaskFailedInstances 验证：只针对 FAILED / TIMEOUT 推回 PENDING 且 attempt+1
func TestRetryTaskFailedInstances(t *testing.T) {
	svc, _, _, instRepo, _, _, _ := newTestService(t)
	// 4 实例：FAILED / TIMEOUT / PASSED / RUNNING
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i1", TaskID: "t1", Status: ev.InstanceStatusFailed, Attempt: 1})
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i2", TaskID: "t1", Status: ev.InstanceStatusTimeout, Attempt: 2})
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i3", TaskID: "t1", Status: ev.InstanceStatusPassed})
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i4", TaskID: "t1", Status: ev.InstanceStatusRunning})

	n, err := svc.RetryTaskFailedInstances(context.Background(), "t1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
	// FAILED/TIMEOUT → PENDING，attempt+1
	i1, _ := instRepo.GetByID(context.Background(), "i1")
	if i1.Status != ev.InstanceStatusPending || i1.Attempt != 2 {
		t.Fatalf("i1 wrong: %+v", i1)
	}
	i2, _ := instRepo.GetByID(context.Background(), "i2")
	if i2.Status != ev.InstanceStatusPending || i2.Attempt != 3 {
		t.Fatalf("i2 wrong: %+v", i2)
	}
	// PASSED/RUNNING 不动
	i3, _ := instRepo.GetByID(context.Background(), "i3")
	if i3.Status != ev.InstanceStatusPassed {
		t.Fatalf("i3 should stay PASSED, got %s", i3.Status)
	}
	i4, _ := instRepo.GetByID(context.Background(), "i4")
	if i4.Status != ev.InstanceStatusRunning {
		t.Fatalf("i4 should stay RUNNING, got %s", i4.Status)
	}
}

// TestRetryInstance_Success 单实例重试：FAILED → PENDING
func TestRetryInstance_Success(t *testing.T) {
	svc, _, _, instRepo, _, _, _ := newTestService(t)
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i1", Status: ev.InstanceStatusFailed, Attempt: 0})

	if err := svc.RetryInstance(context.Background(), "i1"); err != nil {
		t.Fatalf("err: %v", err)
	}
	got, _ := instRepo.GetByID(context.Background(), "i1")
	if got.Status != ev.InstanceStatusPending || got.Attempt != 1 {
		t.Fatalf("wrong after retry: %+v", got)
	}
}

// TestRetryInstance_NotRetryable 非 FAILED/TIMEOUT → ErrInstanceNotRetryable
func TestRetryInstance_NotRetryable(t *testing.T) {
	svc, _, _, instRepo, _, _, _ := newTestService(t)
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i1", Status: ev.InstanceStatusPassed})

	err := svc.RetryInstance(context.Background(), "i1")
	if !errors.Is(err, ErrInstanceNotRetryable) {
		t.Fatalf("expected ErrInstanceNotRetryable, got %v", err)
	}
}

// TestDeleteInstance_TerminalAllowed 终态可以删除
func TestDeleteInstance_TerminalAllowed(t *testing.T) {
	svc, _, _, instRepo, _, _, _ := newTestService(t)
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i1", Status: ev.InstanceStatusPassed})

	if err := svc.DeleteInstance(context.Background(), "i1"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if got, _ := instRepo.GetByID(context.Background(), "i1"); got != nil {
		t.Fatalf("should be deleted")
	}
}

// TestDeleteInstance_RunningRejected RUNNING 拒绝删除
func TestDeleteInstance_RunningRejected(t *testing.T) {
	svc, _, _, instRepo, _, _, _ := newTestService(t)
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i1", Status: ev.InstanceStatusRunning})

	err := svc.DeleteInstance(context.Background(), "i1")
	if !errors.Is(err, ErrInstanceNotDeletable) {
		t.Fatalf("expected ErrInstanceNotDeletable, got %v", err)
	}
}

// TestSweepStaleInstances CAS 成功后写 result + recount
func TestSweepStaleInstances(t *testing.T) {
	svc, _, taskRepo, instRepo, resultRepo, _, _ := newTestService(t)
	// 2 条候选：都 RUNNING 状态
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i1", TaskID: "t1", Status: ev.InstanceStatusRunning})
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i2", TaskID: "t2", Status: ev.InstanceStatusRunning})
	instRepo.stale = []*ev.RunInstance{
		{ID: "i1", TaskID: "t1", Status: ev.InstanceStatusRunning},
		{ID: "i2", TaskID: "t2", Status: ev.InstanceStatusRunning},
	}

	n, err := svc.SweepStaleInstances(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 swept, got %d", n)
	}
	// 状态应变为 TIMEOUT
	i1, _ := instRepo.GetByID(context.Background(), "i1")
	if i1.Status != ev.InstanceStatusTimeout {
		t.Fatalf("i1 status wrong: %s", i1.Status)
	}
	// result 已 upsert
	if len(resultRepo.upserted) != 2 {
		t.Fatalf("expected 2 results upserted, got %d", len(resultRepo.upserted))
	}
	for _, r := range resultRepo.upserted {
		if r.Passed {
			t.Fatalf("expected passed=false, got %+v", r)
		}
		if r.ErrorLog == "" {
			t.Fatalf("expected non-empty ErrorLog")
		}
	}
	// recount 触发 2 次
	if taskRepo.recountCalls.Load() != 2 {
		t.Fatalf("expected 2 recounts, got %d", taskRepo.recountCalls.Load())
	}
}

// TestSweepStaleInstances_CASSkip 已经被别人抢先推进的实例应跳过
func TestSweepStaleInstances_CASSkip(t *testing.T) {
	svc, _, taskRepo, instRepo, resultRepo, _, _ := newTestService(t)
	// 快照记录状态是 RUNNING，但库里已经是 PASSED
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i1", TaskID: "t1", Status: ev.InstanceStatusPassed})
	instRepo.stale = []*ev.RunInstance{
		{ID: "i1", TaskID: "t1", Status: ev.InstanceStatusRunning},
	}

	n, err := svc.SweepStaleInstances(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 swept, got %d", n)
	}
	if len(resultRepo.upserted) != 0 {
		t.Fatalf("no result should be upserted")
	}
	if taskRepo.recountCalls.Load() != 0 {
		t.Fatalf("no recount should happen")
	}
}

// TestReconcileTaskStatuses 只针对 PENDING / RUNNING 的 task recount
func TestReconcileTaskStatuses(t *testing.T) {
	svc, _, taskRepo, _, _, _, _ := newTestService(t)
	_ = taskRepo.Create(context.Background(), &ev.Task{ID: "t1", Status: ev.TaskStatusPending})
	_ = taskRepo.Create(context.Background(), &ev.Task{ID: "t2", Status: ev.TaskStatusRunning})
	_ = taskRepo.Create(context.Background(), &ev.Task{ID: "t3", Status: ev.TaskStatusSucceeded})
	_ = taskRepo.Create(context.Background(), &ev.Task{ID: "t4", Status: ev.TaskStatusPartialFailed})

	n, err := svc.ReconcileTaskStatuses(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 reconciled, got %d", n)
	}
	if taskRepo.recountCalls.Load() != 2 {
		t.Fatalf("expected 2 recounts, got %d", taskRepo.recountCalls.Load())
	}
}

// TestArchiveDeadTasks_NoInspector 未注入 → 返回 (0, nil)
func TestArchiveDeadTasks_NoInspector(t *testing.T) {
	svc, _, _, _, _, _, _ := newTestService(t)
	n, err := svc.ArchiveDeadTasks(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

// TestArchiveDeadTasks_WithInspector 注入后应把非终态实例落 FAILED + result + recount
func TestArchiveDeadTasks_WithInspector(t *testing.T) {
	caseRepo := newFakeCaseRepo()
	taskRepo := newFakeTaskRepo()
	instRepo := newFakeInstRepo()
	resultRepo := newFakeResultRepo()
	snapRepo := newFakeSnapshotRepo()
	inspector := &fakeDLQInspector{ids: []string{"i1", "i2"}}
	loader := &fakeAppConfigLoader{store: map[string]appconfig.AppConfigDO{}}
	svc := NewEvaluationDomainService(
		caseRepo, taskRepo, instRepo, resultRepo, snapRepo,
		loader, nil, inspector,
	).(*serviceImpl)

	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i1", TaskID: "t1", Status: ev.InstanceStatusRunning})
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i2", TaskID: "t2", Status: ev.InstanceStatusPassed}) // 终态跳过

	n, err := svc.ArchiveDeadTasks(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 archived, got %d", n)
	}
	i1, _ := instRepo.GetByID(context.Background(), "i1")
	if i1.Status != ev.InstanceStatusFailed {
		t.Fatalf("i1 should be FAILED, got %s", i1.Status)
	}
	if len(resultRepo.upserted) != 1 {
		t.Fatalf("expected 1 result upserted, got %d", len(resultRepo.upserted))
	}
	if taskRepo.recountCalls.Load() != 1 {
		t.Fatalf("expected 1 recount, got %d", taskRepo.recountCalls.Load())
	}
}

// TestExecuteInstance_NoExecutor executor 未注入 → ErrExecutorNotWired
func TestExecuteInstance_NoExecutor(t *testing.T) {
	svc, _, _, _, _, _, _ := newTestService(t)
	err := svc.ExecuteInstance(context.Background(), "any", "worker-1")
	if !errors.Is(err, ErrExecutorNotWired) {
		t.Fatalf("expected ErrExecutorNotWired, got %v", err)
	}
}

// TestDeleteTask_Success 删除 task 并去重删除 snapshot
func TestDeleteTask_Success(t *testing.T) {
	svc, _, taskRepo, instRepo, _, snapRepo, _ := newTestService(t)
	// 手工塞入 task + 2 个 instance 共享同一个 snapshot
	_ = taskRepo.Create(context.Background(), &ev.Task{ID: "t1", Status: ev.TaskStatusPending})
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i1", TaskID: "t1", AgentConfigSnapshotID: "s1"})
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i2", TaskID: "t1", AgentConfigSnapshotID: "s1"})
	_ = instRepo.Create(context.Background(), &ev.RunInstance{ID: "i3", TaskID: "t1", AgentConfigSnapshotID: "s2"})
	_ = snapRepo.BatchCreate(context.Background(), []*ev.AgentSnapshot{
		{ID: "s1"}, {ID: "s2"},
	})

	if err := svc.DeleteTask(context.Background(), "t1"); err != nil {
		t.Fatalf("err: %v", err)
	}
	// task 已删
	if _, gerr := taskRepo.Get(context.Background(), "t1"); gerr == nil {
		t.Fatalf("task should be deleted")
	}
	// snapshot 应被去重删除（s1 只删 1 次）
	if got, _ := snapRepo.Get(context.Background(), "s1"); got != nil {
		t.Fatalf("s1 should be deleted")
	}
	if got, _ := snapRepo.Get(context.Background(), "s2"); got != nil {
		t.Fatalf("s2 should be deleted")
	}
	// 计入 deleted 的调用次数：确认 s1 未被重复删（只删 1 次）
	countS1 := 0
	for _, id := range snapRepo.deleted {
		if id == "s1" {
			countS1++
		}
	}
	if countS1 != 1 {
		t.Fatalf("expected s1 deleted exactly once (dedup), got %d", countS1)
	}
}

// fakeDLQInspector 内存实现
type fakeDLQInspector struct {
	ids []string
	err error
}

func (f *fakeDLQInspector) ListArchivedRunInstanceIDs(ctx context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ids, nil
}
