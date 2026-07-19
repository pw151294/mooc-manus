//go:build stress

package evaluation

// Task 8.2 压测：验证 500 实例并发时 goroutine 无泄漏。
// 编译门槛：`go test -tags=stress ...` 才会纳入编译，默认 CI 跳过。

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "mooc-manus/internal/domains/models"
	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/repositories"
)

// TestStress_500Instances 500 条 instance 并发 execute，验证：
//   - executor 成功推进到终态
//   - goroutine 泄漏 < 20（心跳 goroutine 应随 Execute 返回自行回收）
//
// 手工执行：go test -tags=stress -run TestStress_500Instances -count=1 -v -timeout 300s
func TestStress_500Instances(t *testing.T) {
	// 记录初始 goroutine 数（排除测试框架启动的辅助 g）
	// 先 GC 一次以稳定基线
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()
	t.Logf("goroutine baseline before=%d", before)

	ctx := context.Background()

	// ==== 装配 fakes ====
	caseRepo := newFakeCaseRepo()
	instRepo := newFakeInstRepo()
	taskRepo := newE2ETaskRepo(instRepo)
	resultRepo := newFakeResultRepo()
	snapshotRepo := newFakeSnapshotRepo()
	loader := &fakeAppConfigLoader{store: map[string]appconfig.AppConfigDO{}}

	// 100 个 case × 5 个 agent config = 500 条 instance
	const totalCases = 100
	const totalCfgs = 5
	caseIDs := make([]string, 0, totalCases)
	for i := 0; i < totalCases; i++ {
		id := fmt.Sprintf("case-%d", i)
		caseIDs = append(caseIDs, id)
		require.NoError(t, caseRepo.Create(ctx, &ev.Case{
			ID: id, Name: id, TaskPrompt: "q", VerifyScript: "exit 0",
		}))
	}
	cfgIDs := make([]string, 0, totalCfgs)
	for i := 0; i < totalCfgs; i++ {
		id := fmt.Sprintf("cfg-%d", i)
		cfgIDs = append(cfgIDs, id)
		loader.store[id] = makeAppConfig(id)
	}

	// ==== 装配 executor ====
	verifyRunner := NewVerifyRunner(10*time.Second, 4<<10)
	chatRunner := &stubChatRunner{res: InternalChatResult{LastAssistantMsg: "done"}}
	spanRepo := &stubSpanRepo{}
	aggregator := NewTraceAggregator(spanRepo)
	skillExecutor := &stubSkillExecutor{}
	native := newE2ENativeProvider(t.TempDir())

	executor := NewInstanceExecutor(
		instRepo, taskRepo, resultRepo, snapshotRepo,
		verifyRunner, chatRunner, aggregator, nil,
		skillExecutor, native,
		"wk-stress", 200*time.Millisecond, 30*time.Second,
	)

	domain := NewEvaluationDomainService(
		caseRepo, taskRepo, instRepo, resultRepo, snapshotRepo,
		loader, executor, nil,
	)

	// ==== 阶段 1: CreateTask ====
	task, err := domain.CreateTask(ctx, "stress-task", caseIDs, cfgIDs)
	require.NoError(t, err)
	require.Equal(t, totalCases*totalCfgs, task.TotalCount)

	insts, _, err := instRepo.List(ctx, repositories.InstanceListFilter{TaskID: task.ID}, 1, 10000)
	require.NoError(t, err)
	require.Len(t, insts, totalCases*totalCfgs)

	// ==== 阶段 2: CAS PENDING → QUEUED ====
	for _, inst := range insts {
		ok, _ := instRepo.CASStatus(ctx, inst.ID,
			ev.InstanceStatusPending, ev.InstanceStatusQueued)
		require.True(t, ok)
	}

	// ==== 阶段 3: 并发 execute（限制并发度避免宿主机 fork 风暴）====
	const concurrency = 20
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var okCnt atomic.Int32
	start := time.Now()
	for _, inst := range insts {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := executor.Execute(ctx, id); err == nil {
				okCnt.Add(1)
			}
		}(inst.ID)
	}
	wg.Wait()
	elapsed := time.Since(start)
	t.Logf("500 instances execute 完成，用时 %s，成功 %d", elapsed, okCnt.Load())

	// ==== 阶段 4: 等心跳等辅助 goroutine 收敛 ====
	// heartbeat goroutine 里 defer cancel 后需要一次 tick 才能真正退出；
	// finalizeVerify 内部还有 300ms + 500ms sleep（trace flush 重试），此处宽松等待。
	time.Sleep(1500 * time.Millisecond)
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	after := runtime.NumGoroutine()
	delta := after - before
	t.Logf("goroutine before=%d after=%d delta=%d", before, after, delta)

	// ==== 阶段 5: 断言 ====
	assert.Equal(t, int32(totalCases*totalCfgs), okCnt.Load(),
		"所有 instance 都应 execute 成功")
	assert.Less(t, delta, 20, "goroutine 泄漏应 < 20")

	// 抽样确认几个 result 已落库
	sampleCnt := 0
	for i := 0; i < len(insts); i += 50 {
		r, err := resultRepo.Get(ctx, insts[i].ID)
		require.NoError(t, err)
		if r != nil && r.Passed {
			sampleCnt++
		}
	}
	assert.Greater(t, sampleCnt, 0, "抽样 result 应至少有 1 条 passed")

	// task 应到终态 SUCCEEDED（全 case verify 都 exit 0）
	tsk, err := taskRepo.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, ev.TaskStatusSucceeded, tsk.Status)
	assert.Equal(t, totalCases*totalCfgs, tsk.SucceededCount)
}
