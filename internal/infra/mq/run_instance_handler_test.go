package mq

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/repositories"
)

// ==== fake executor ====

type fakeExecutor struct {
	called   bool
	gotID    string
	returnFn func(ctx context.Context, id string) error
}

func (f *fakeExecutor) Execute(ctx context.Context, id string) error {
	f.called = true
	f.gotID = id
	if f.returnFn != nil {
		return f.returnFn(ctx, id)
	}
	return nil
}

// ==== fake instRepo ====

// fakeInstRepo 只实现 handler 用到的 GetByID；其余方法 panic 保护未使用假设。
type fakeInstRepo struct {
	repositories.EvalRunInstanceRepository // 内嵌接口以获取默认方法签名
	inst                                   *ev.RunInstance
	err                                    error
}

func (f *fakeInstRepo) GetByID(ctx context.Context, id string) (*ev.RunInstance, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.inst == nil {
		return nil, errors.New("not found")
	}
	// 拷贝一份避免测试互相影响
	c := *f.inst
	return &c, nil
}

// ==== fake caseGate ====

type fakeCaseGate struct {
	acquireOK   bool
	acquireErr  error
	acquireHit  int
	releaseHit  int
	lastCaseID  string
}

func (g *fakeCaseGate) Acquire(ctx context.Context, caseID string) (bool, error) {
	g.acquireHit++
	g.lastCaseID = caseID
	return g.acquireOK, g.acquireErr
}
func (g *fakeCaseGate) Release(ctx context.Context, caseID string) error {
	g.releaseHit++
	return nil
}

// ---- 测试 ----

// TestHandler_BadPayload_SkipRetry payload 无法解析时返回 SkipRetry
func TestHandler_BadPayload_SkipRetry(t *testing.T) {
	h := NewRunInstanceHandler(&fakeExecutor{}, &fakeInstRepo{}, &fakeCaseGate{})
	task := asynq.NewTask(TaskTypeRunInstance, []byte("{not json"))

	err := h.ProcessTask(context.Background(), task)
	require.Error(t, err)
	assert.ErrorIs(t, err, asynq.SkipRetry)
}

// TestHandler_InstanceNotFound_SkipRetry GetByID 失败视为 SkipRetry
func TestHandler_InstanceNotFound_SkipRetry(t *testing.T) {
	repo := &fakeInstRepo{err: errors.New("not found")}
	h := NewRunInstanceHandler(&fakeExecutor{}, repo, &fakeCaseGate{})
	payload, _ := (&RunInstancePayload{InstanceID: "x", EnqueuedAt: time.Now().Unix()}).Marshal()

	err := h.ProcessTask(context.Background(), asynq.NewTask(TaskTypeRunInstance, payload))
	require.Error(t, err)
	assert.ErrorIs(t, err, asynq.SkipRetry)
}

// TestHandler_CaseGateBusy_Retry token 已满时返回普通 err（非 SkipRetry），由 asynq 重投
func TestHandler_CaseGateBusy_Retry(t *testing.T) {
	repo := &fakeInstRepo{inst: &ev.RunInstance{ID: "i-1", CaseID: "c-1"}}
	gate := &fakeCaseGate{acquireOK: false}
	fx := &fakeExecutor{}
	h := NewRunInstanceHandler(fx, repo, gate)

	payload, _ := (&RunInstancePayload{InstanceID: "i-1"}).Marshal()
	err := h.ProcessTask(context.Background(), asynq.NewTask(TaskTypeRunInstance, payload))

	require.Error(t, err)
	assert.NotErrorIs(t, err, asynq.SkipRetry, "case busy 必须允许 asynq 重投")
	assert.False(t, fx.called, "token 未拿到时 executor 不应被调用")
	assert.Equal(t, 0, gate.releaseHit, "Acquire 失败时不能 Release")
}

// TestHandler_RedisFlaky_ReturnError Acquire 报错时直接透传 err，让 asynq 重试
func TestHandler_RedisFlaky_ReturnError(t *testing.T) {
	repo := &fakeInstRepo{inst: &ev.RunInstance{ID: "i-2", CaseID: "c-2"}}
	gate := &fakeCaseGate{acquireErr: errors.New("redis: connection refused")}
	fx := &fakeExecutor{}
	h := NewRunInstanceHandler(fx, repo, gate)

	payload, _ := (&RunInstancePayload{InstanceID: "i-2"}).Marshal()
	err := h.ProcessTask(context.Background(), asynq.NewTask(TaskTypeRunInstance, payload))

	require.Error(t, err)
	assert.NotErrorIs(t, err, asynq.SkipRetry)
	assert.False(t, fx.called)
}

// TestHandler_Success token 拿到后调 executor，最后 Release
func TestHandler_Success(t *testing.T) {
	repo := &fakeInstRepo{inst: &ev.RunInstance{ID: "i-3", CaseID: "c-3"}}
	gate := &fakeCaseGate{acquireOK: true}
	fx := &fakeExecutor{}
	h := NewRunInstanceHandler(fx, repo, gate)

	payload, _ := (&RunInstancePayload{InstanceID: "i-3"}).Marshal()
	err := h.ProcessTask(context.Background(), asynq.NewTask(TaskTypeRunInstance, payload))

	require.NoError(t, err)
	assert.True(t, fx.called)
	assert.Equal(t, "i-3", fx.gotID)
	assert.Equal(t, "c-3", gate.lastCaseID)
	assert.Equal(t, 1, gate.acquireHit)
	assert.Equal(t, 1, gate.releaseHit, "成功流程末尾必须 Release")
}

// TestHandler_ExecutorError_ReleaseStillHappens executor 报错，Release 仍需触发
func TestHandler_ExecutorError_ReleaseStillHappens(t *testing.T) {
	repo := &fakeInstRepo{inst: &ev.RunInstance{ID: "i-4", CaseID: "c-4"}}
	gate := &fakeCaseGate{acquireOK: true}
	fx := &fakeExecutor{returnFn: func(ctx context.Context, id string) error { return errors.New("boom") }}
	h := NewRunInstanceHandler(fx, repo, gate)

	payload, _ := (&RunInstancePayload{InstanceID: "i-4"}).Marshal()
	err := h.ProcessTask(context.Background(), asynq.NewTask(TaskTypeRunInstance, payload))

	require.Error(t, err)
	assert.Equal(t, 1, gate.releaseHit)
}
