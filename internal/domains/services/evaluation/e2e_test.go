package evaluation

// 本文件承载 Task 8.1：evaluation 域端到端集成测。
// 通过纯内存 fakes + 真实 executor 组件（VerifyRunner 跑真实 bash）验证：
//   CreateTask → 顺序 Execute → Result 生成 → Task recount 全链路
// 覆盖 spec §3.5 + §4.5：QUEUED → INITIALIZING → RUNNING → VERIFYING → PASSED/FAILED 全状态推进。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "mooc-manus/internal/domains/models"
	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/repositories"
)

// e2eTaskRepo 是 E2E 场景专用的 task repo：
//   - 内存实现 + 真实 RecountAndTransit（对齐 infra/repositories/eval_task.go 的 SQL 语义）
//   - 依赖注入 instRepo 用于 recount 时汇总实例状态
type e2eTaskRepo struct {
	mu           sync.Mutex
	store        map[string]*ev.Task
	instRepo     *fakeInstRepo
	recountCalls atomic.Int32
}

func newE2ETaskRepo(instRepo *fakeInstRepo) *e2eTaskRepo {
	return &e2eTaskRepo{store: map[string]*ev.Task{}, instRepo: instRepo}
}

func (r *e2eTaskRepo) Create(ctx context.Context, t *ev.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyT := *t
	r.store[t.ID] = &copyT
	return nil
}

func (r *e2eTaskRepo) Get(ctx context.Context, id string) (*ev.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.store[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found", id)
	}
	copyT := *t
	return &copyT, nil
}

func (r *e2eTaskRepo) List(ctx context.Context, filter repositories.TaskListFilter, page, size int) ([]*ev.Task, int64, error) {
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

func (r *e2eTaskRepo) Update(ctx context.Context, t *ev.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyT := *t
	r.store[t.ID] = &copyT
	return nil
}

func (r *e2eTaskRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.store, id)
	return nil
}

// RecountAndTransit 对齐 infra/repositories/eval_task.go 的 SQL 语义：
//   terminal < total          → RUNNING
//   terminal == total, passed == total → SUCCEEDED
//   其它                        → PARTIAL_FAILED
// 使用 instRepo 内存视图统计 —— 与生产 SQL 视角一致（终态四种 + PASSED 计数）。
func (r *e2eTaskRepo) RecountAndTransit(ctx context.Context, taskID string) error {
	r.recountCalls.Add(1)

	// 拉全部实例（page 大到覆盖测试规模）
	insts, _, err := r.instRepo.List(ctx, repositories.InstanceListFilter{TaskID: taskID}, 1, 100000)
	if err != nil {
		return err
	}
	var terminal, passed, total int64
	for _, inst := range insts {
		total++
		switch inst.Status {
		case ev.InstanceStatusPassed:
			terminal++
			passed++
		case ev.InstanceStatusFailed, ev.InstanceStatusTimeout, ev.InstanceStatusCanceled:
			terminal++
		}
	}

	var newStatus ev.TaskStatus
	switch {
	case terminal < total:
		newStatus = ev.TaskStatusRunning
	case terminal == total && passed == total:
		newStatus = ev.TaskStatusSucceeded
	default:
		newStatus = ev.TaskStatusPartialFailed
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.store[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	t.Status = newStatus
	t.SucceededCount = int(passed)
	t.FailedCount = int(terminal - passed)
	t.RunningCount = int(total - terminal)
	if newStatus == ev.TaskStatusSucceeded || newStatus == ev.TaskStatusPartialFailed {
		now := time.Now()
		t.FinishedAt = &now
	}
	return nil
}

// e2eNativeProvider 每 messageID 独立 workspace，避免并发 verify_script 互相踩踏
// （VerifyRunner 会写 workdir/.verify.sh，共享目录并发会 race）。
// 实现 tools.NativeToolsProvider 全接口。
type e2eNativeProvider struct {
	mu      sync.Mutex
	baseDir string
	dirs    map[string]string
	cleaned []string
}

func newE2ENativeProvider(baseDir string) *e2eNativeProvider {
	return &e2eNativeProvider{baseDir: baseDir, dirs: map[string]string{}}
}

func (p *e2eNativeProvider) BuildTools(messageId, conversationId string) ([]tools.Tool, error) {
	return nil, nil
}

func (p *e2eNativeProvider) Cleanup(messageId string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleaned = append(p.cleaned, messageId)
	if dir, ok := p.dirs[messageId]; ok {
		_ = os.RemoveAll(dir)
		delete(p.dirs, messageId)
	}
	return nil
}

func (p *e2eNativeProvider) ConversationPlanDir(conversationId string) string {
	return ""
}

func (p *e2eNativeProvider) MessageWorkspaceDir(messageId string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if dir, ok := p.dirs[messageId]; ok {
		return dir
	}
	dir := filepath.Join(p.baseDir, messageId)
	p.dirs[messageId] = dir
	return dir
}

func (p *e2eNativeProvider) cleanedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.cleaned)
}

// TestE2E_2x2_TaskLifecycle 2×2 M×N 全链路生命周期：
//   - 2 case（pass / fail） × 2 agent config → 4 instance
//   - 顺序 execute 每条 instance（真实 VerifyRunner，跑真实 bash）
//   - 2 条 verify exit=0 → PASSED；2 条 verify exit=1 → FAILED
//   - 断言 task.status=PARTIAL_FAILED，SucceededCount=2，FailedCount=2，4 条 result 落库
func TestE2E_2x2_TaskLifecycle(t *testing.T) {
	ctx := context.Background()

	// ==== 装配 fakes ====
	caseRepo := newFakeCaseRepo()
	instRepo := newFakeInstRepo()
	taskRepo := newE2ETaskRepo(instRepo)
	resultRepo := newFakeResultRepo()
	snapshotRepo := newFakeSnapshotRepo()
	loader := &fakeAppConfigLoader{store: map[string]appconfig.AppConfigDO{}}

	// 2 个 case：pass / fail（VerifyScript 直接决定 verify 阶段结果）
	require.NoError(t, caseRepo.Create(ctx, &ev.Case{
		ID: "c-pass", Name: "pass", TaskPrompt: "do it", VerifyScript: "exit 0",
	}))
	require.NoError(t, caseRepo.Create(ctx, &ev.Case{
		ID: "c-fail", Name: "fail", TaskPrompt: "do it", VerifyScript: "echo bad 1>&2; exit 1",
	}))

	// 2 个 agent config：内容对评测流转无意义，只需 loader 能取到
	loader.store["cfg-a"] = makeAppConfig("cfg-a")
	loader.store["cfg-b"] = makeAppConfig("cfg-b")

	// ==== 装配 executor ====
	verifyRunner := NewVerifyRunner(10*time.Second, 4<<10)
	// stubChatRunner 直接返回 mock 结果模拟 chat 完成
	chatRunner := &stubChatRunner{res: InternalChatResult{LastAssistantMsg: "done"}}
	// aggregator 用 stubSpanRepo（返回空 span → Degraded=true，但不阻塞 Passed/Failed 判定）
	spanRepo := &stubSpanRepo{}
	aggregator := NewTraceAggregator(spanRepo)
	skillExecutor := &stubSkillExecutor{}
	native := newE2ENativeProvider(t.TempDir())

	executor := NewInstanceExecutor(
		instRepo, taskRepo, resultRepo, snapshotRepo,
		verifyRunner, chatRunner, aggregator, nil,
		skillExecutor, native,
		"wk-e2e", 200*time.Millisecond, 30*time.Second,
	)

	// ==== 装配 domain service（executor 已就绪，可完整链路）====
	domain := NewEvaluationDomainService(
		caseRepo, taskRepo, instRepo, resultRepo, snapshotRepo,
		loader, executor, nil,
	)

	// ==== 阶段 1: CreateTask ====
	task, err := domain.CreateTask(ctx, "e2e-task",
		[]string{"c-pass", "c-fail"}, []string{"cfg-a", "cfg-b"})
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, 4, task.TotalCount, "M=2 × N=2 → 4 instance")
	assert.Equal(t, ev.TaskStatusPending, task.Status)

	// ==== 阶段 2: 手工把 PENDING → QUEUED（模拟 Application 层 asynq enqueue 前的 CAS）====
	insts, _, err := instRepo.List(ctx, repositories.InstanceListFilter{TaskID: task.ID}, 1, 100)
	require.NoError(t, err)
	require.Len(t, insts, 4)
	for _, inst := range insts {
		ok, cerr := instRepo.CASStatus(ctx, inst.ID,
			ev.InstanceStatusPending, ev.InstanceStatusQueued)
		require.NoError(t, cerr)
		require.True(t, ok, "PENDING→QUEUED CAS 应成功")
	}

	// ==== 阶段 3: 顺序 execute 每条 instance ====
	for _, inst := range insts {
		require.NoError(t, executor.Execute(ctx, inst.ID))
	}

	// ==== 阶段 4: 断言 task 最终状态 ====
	tsk, err := taskRepo.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, ev.TaskStatusPartialFailed, tsk.Status,
		"2 PASSED + 2 FAILED → PARTIAL_FAILED")
	assert.Equal(t, 2, tsk.SucceededCount)
	assert.Equal(t, 2, tsk.FailedCount)
	assert.Equal(t, 0, tsk.RunningCount)
	assert.NotNil(t, tsk.FinishedAt, "终态 task 应有 FinishedAt")

	// recount 至少被调用 4 次（每条 instance finalize 一次）
	assert.GreaterOrEqual(t, int(taskRepo.recountCalls.Load()), 4)

	// ==== 阶段 5: 断言 4 条 Result 落库 ====
	passCnt, failCnt := 0, 0
	for _, inst := range insts {
		r, err := resultRepo.Get(ctx, inst.ID)
		require.NoError(t, err)
		require.NotNil(t, r, "instance %s 应有 Result", inst.ID)
		if r.Passed {
			passCnt++
			assert.Equal(t, 0, r.VerifyExitCode)
		} else {
			failCnt++
		}
	}
	assert.Equal(t, 2, passCnt)
	assert.Equal(t, 2, failCnt)

	// ==== 阶段 6: 断言实例状态推进到位 ====
	insts2, _, err := instRepo.List(ctx, repositories.InstanceListFilter{TaskID: task.ID}, 1, 100)
	require.NoError(t, err)
	termPassed, termFailed := 0, 0
	for _, inst := range insts2 {
		switch inst.Status {
		case ev.InstanceStatusPassed:
			termPassed++
		case ev.InstanceStatusFailed:
			termFailed++
		default:
			t.Errorf("instance %s 处于非终态: %s", inst.ID, inst.Status)
		}
	}
	assert.Equal(t, 2, termPassed)
	assert.Equal(t, 2, termFailed)

	// ==== 阶段 7: 断言 cleanup 被调用（4 条 instance × 1 次）====
	skillExecutor.mu.Lock()
	skillCleaned := len(skillExecutor.cleaned)
	skillExecutor.mu.Unlock()
	assert.Equal(t, 4, skillCleaned)
	assert.Equal(t, 4, native.cleanedCount())
}
