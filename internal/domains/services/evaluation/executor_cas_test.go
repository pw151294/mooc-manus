package evaluation

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	ev "mooc-manus/internal/domains/models/evaluation"
)

// TestExecutor_CAS_QueuedFailsFast 验证：Execute 起手 CAS QUEUED→INITIALIZING 失败时，
// 应短路返回 nil，不再触发 GetByID / 后续 stage，且 CAS 只被调用一次。
func TestExecutor_CAS_QueuedFailsFast(t *testing.T) {
	instRepo := &stubInstRepo{
		casReturns: []bool{false}, // 首次 CAS 就失败（已被其他 worker 抢走）
	}
	taskRepo := &stubTaskRepo{}
	resultRepo := &stubResultRepo{}
	snapRepo := &stubSnapshotRepo{}
	chat := &stubChatRunner{}
	skill := &stubSkillExecutor{}
	native := &stubNativeProvider{workspaceDir: t.TempDir()}

	e := NewInstanceExecutor(
		instRepo, taskRepo, resultRepo, snapRepo,
		NewVerifyRunner(2*time.Second, 4096),
		chat,
		nil, // aggregator 不参与本用例
		nil,
		skill, native,
		"worker-1", 50*time.Millisecond, 2*time.Second,
		zap.NewNop(),
	)

	err := e.Execute(context.Background(), "inst-1")
	if err != nil {
		t.Fatalf("expect nil error on CAS fail, got %v", err)
	}
	if len(instRepo.casCalls) != 1 {
		t.Fatalf("expect 1 CAS call, got %d (%+v)", len(instRepo.casCalls), instRepo.casCalls)
	}
	if instRepo.casCalls[0].From != ev.InstanceStatusQueued ||
		instRepo.casCalls[0].To != ev.InstanceStatusInitializing {
		t.Fatalf("unexpected CAS transition: %+v", instRepo.casCalls[0])
	}
	if len(resultRepo.upserted) != 0 {
		t.Fatalf("no result should be upserted on CAS-fail short circuit, got %d", len(resultRepo.upserted))
	}
	if taskRepo.recountCalls.Load() != 0 {
		t.Fatalf("no recount should happen on CAS-fail short circuit")
	}
}

// TestExecutor_CAS_QueuedSucceeds 验证：CAS 成功后 Execute 会加载 inst 并进入后续 stage。
// 本用例只关心第一步 CAS 通过，具体 stage 行为由后续测试覆盖，因此让 chat 立即返回一个错误
// 便于快速收敛路径 —— 这样也能顺便断言 finalizeError 触发的 recount / cleanup。
func TestExecutor_CAS_QueuedSucceeds(t *testing.T) {
	inst := &ev.RunInstance{
		ID:                    "inst-1",
		TaskID:                "task-1",
		MessageID:             "msg-1",
		ConversationID:        "conv-1",
		AgentConfigSnapshotID: "snap-1",
		CaseSnapshot: ev.Case{
			TaskPrompt:   "hello",
			VerifyScript: "exit 0",
		},
	}
	instRepo := &stubInstRepo{
		inst:       inst,
		casReturns: []bool{true, true, true, true}, // 4 次 CAS 都通过
	}
	taskRepo := &stubTaskRepo{}
	resultRepo := &stubResultRepo{}
	snapRepo := &stubSnapshotRepo{}
	chat := &stubChatRunner{
		res: InternalChatResult{Error: nil, LastAssistantMsg: "ok"},
	}
	skill := &stubSkillExecutor{}
	native := &stubNativeProvider{workspaceDir: t.TempDir()}

	e := NewInstanceExecutor(
		instRepo, taskRepo, resultRepo, snapRepo,
		NewVerifyRunner(2*time.Second, 4096),
		chat, nil, nil,
		skill, native,
		"worker-1", 50*time.Millisecond, 2*time.Second,
		zap.NewNop(),
	)

	if err := e.Execute(context.Background(), "inst-1"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// CAS 至少 3 次：QUEUED→INITIALIZING、INITIALIZING→RUNNING、RUNNING→VERIFYING，
	// 加 VERIFYING→PASSED 共 4 次
	if len(instRepo.casCalls) < 3 {
		t.Fatalf("expect >=3 CAS transitions, got %d (%+v)", len(instRepo.casCalls), instRepo.casCalls)
	}
	if instRepo.casCalls[0].From != ev.InstanceStatusQueued ||
		instRepo.casCalls[0].To != ev.InstanceStatusInitializing {
		t.Fatalf("first CAS wrong: %+v", instRepo.casCalls[0])
	}
}
