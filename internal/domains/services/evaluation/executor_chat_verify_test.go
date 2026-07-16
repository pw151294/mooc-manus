package evaluation

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	ev "mooc-manus/internal/domains/models/evaluation"
)

// makeInst 构造最小可用 inst，仅需变体特定字段可覆盖
func makeInst(verifyScript string) *ev.RunInstance {
	return &ev.RunInstance{
		ID:                    "inst-1",
		TaskID:                "task-1",
		MessageID:             "msg-1",
		ConversationID:        "conv-1",
		AgentConfigSnapshotID: "snap-1",
		CaseSnapshot: ev.Case{
			TaskPrompt:   "do it",
			VerifyScript: verifyScript,
		},
	}
}

// newTestExecutor 构造 executor，注入常用桩
func newTestExecutor(t *testing.T, inst *ev.RunInstance, chat *stubChatRunner) (
	*InstanceExecutor, *stubInstRepo, *stubTaskRepo, *stubResultRepo,
	*stubSkillExecutor, *stubNativeProvider,
) {
	t.Helper()
	instRepo := &stubInstRepo{inst: inst}
	taskRepo := &stubTaskRepo{}
	resultRepo := &stubResultRepo{}
	snapRepo := &stubSnapshotRepo{}
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
	return e, instRepo, taskRepo, resultRepo, skill, native
}

// TestExecutor_ChatError 验证：chat 返回 Result.Error 时，走 RUNNING→FAILED，
// verify_script 不应被执行（通过 recount / cleanup 反射链路收敛正确）。
func TestExecutor_ChatError(t *testing.T) {
	inst := makeInst("exit 0")
	chat := &stubChatRunner{
		res: InternalChatResult{Error: errors.New("agent boom")},
	}
	e, instRepo, taskRepo, resultRepo, skill, native := newTestExecutor(t, inst, chat)

	if err := e.Execute(context.Background(), "inst-1"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	last := resultRepo.lastResult()
	if last == nil || last.Passed {
		t.Fatalf("expected failed result, got %+v", last)
	}
	if !contains(last.ErrorLog, "agent_error: agent boom") {
		t.Fatalf("ErrorLog missing agent boom: %q", last.ErrorLog)
	}
	// CAS 中应包含 RUNNING→FAILED（第 3 个 CAS）
	if len(instRepo.casCalls) < 3 {
		t.Fatalf("expect >=3 CAS calls: %+v", instRepo.casCalls)
	}
	if instRepo.casCalls[2].From != ev.InstanceStatusRunning ||
		instRepo.casCalls[2].To != ev.InstanceStatusFailed {
		t.Fatalf("expected RUNNING→FAILED at #3, got %+v", instRepo.casCalls[2])
	}
	if taskRepo.recountCalls.Load() != 1 {
		t.Fatalf("expected 1 recount")
	}
	if len(skill.cleaned) == 0 || len(native.cleaned) == 0 {
		t.Fatalf("cleanup missing")
	}
}

// TestExecutor_ChatTimeout 验证：chat 返回 DidTimeout=true 时走 RUNNING→TIMEOUT
func TestExecutor_ChatTimeout(t *testing.T) {
	inst := makeInst("exit 0")
	chat := &stubChatRunner{
		res: InternalChatResult{DidTimeout: true},
	}
	e, instRepo, _, resultRepo, _, _ := newTestExecutor(t, inst, chat)

	if err := e.Execute(context.Background(), "inst-1"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	last := resultRepo.lastResult()
	if last == nil || last.Passed {
		t.Fatalf("expected failed result, got %+v", last)
	}
	if !contains(last.ErrorLog, "agent_chat_timeout") {
		t.Fatalf("ErrorLog missing timeout marker: %q", last.ErrorLog)
	}
	if len(instRepo.casCalls) < 3 {
		t.Fatalf("expect >=3 CAS calls: %+v", instRepo.casCalls)
	}
	if instRepo.casCalls[2].From != ev.InstanceStatusRunning ||
		instRepo.casCalls[2].To != ev.InstanceStatusTimeout {
		t.Fatalf("expected RUNNING→TIMEOUT at #3, got %+v", instRepo.casCalls[2])
	}
}

// TestExecutor_VerifyExit0 验证：verify exit=0 时走 VERIFYING→PASSED，result.Passed=true
func TestExecutor_VerifyExit0(t *testing.T) {
	inst := makeInst("exit 0")
	chat := &stubChatRunner{res: InternalChatResult{LastAssistantMsg: "ok"}}
	e, instRepo, taskRepo, resultRepo, _, _ := newTestExecutor(t, inst, chat)

	if err := e.Execute(context.Background(), "inst-1"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	last := resultRepo.lastResult()
	if last == nil || !last.Passed {
		t.Fatalf("expected passed result, got %+v", last)
	}
	if last.VerifyExitCode != 0 {
		t.Fatalf("expected VerifyExitCode=0, got %d", last.VerifyExitCode)
	}
	// 4 次 CAS：QUEUED→INIT, INIT→RUN, RUN→VERIFYING, VERIFYING→PASSED
	if len(instRepo.casCalls) != 4 {
		t.Fatalf("expected 4 CAS, got %d: %+v", len(instRepo.casCalls), instRepo.casCalls)
	}
	if instRepo.casCalls[3].From != ev.InstanceStatusVerifying ||
		instRepo.casCalls[3].To != ev.InstanceStatusPassed {
		t.Fatalf("expected VERIFYING→PASSED at #4, got %+v", instRepo.casCalls[3])
	}
	if taskRepo.recountCalls.Load() != 1 {
		t.Fatalf("expected 1 recount, got %d", taskRepo.recountCalls.Load())
	}
}

// TestExecutor_VerifyExit1 验证：verify exit=1 时走 VERIFYING→FAILED
// 且 stderr 汇总到 ErrorLog
func TestExecutor_VerifyExit1(t *testing.T) {
	inst := makeInst("echo bad 1>&2; exit 1")
	chat := &stubChatRunner{res: InternalChatResult{LastAssistantMsg: "ok"}}
	e, instRepo, _, resultRepo, _, _ := newTestExecutor(t, inst, chat)

	if err := e.Execute(context.Background(), "inst-1"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	last := resultRepo.lastResult()
	if last == nil || last.Passed {
		t.Fatalf("expected failed result, got %+v", last)
	}
	if last.VerifyExitCode != 1 {
		t.Fatalf("expected VerifyExitCode=1, got %d", last.VerifyExitCode)
	}
	if !contains(last.VerifyStderr, "bad") {
		t.Fatalf("expected stderr to contain 'bad', got %q", last.VerifyStderr)
	}
	// #4 CAS：VERIFYING→FAILED
	if len(instRepo.casCalls) < 4 {
		t.Fatalf("expected >=4 CAS, got %+v", instRepo.casCalls)
	}
	if instRepo.casCalls[3].From != ev.InstanceStatusVerifying ||
		instRepo.casCalls[3].To != ev.InstanceStatusFailed {
		t.Fatalf("expected VERIFYING→FAILED at #4, got %+v", instRepo.casCalls[3])
	}
}
