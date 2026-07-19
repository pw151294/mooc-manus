package evaluation

import (
	"context"
	"testing"
	"time"

	ev "mooc-manus/internal/domains/models/evaluation"
)

// TestExecutor_InitScript_Success 验证：init_script exit=0 时进入 chat / verify 分支
func TestExecutor_InitScript_Success(t *testing.T) {
	inst := &ev.RunInstance{
		ID:                    "inst-1",
		TaskID:                "task-1",
		MessageID:             "msg-1",
		ConversationID:        "conv-1",
		AgentConfigSnapshotID: "snap-1",
		CaseSnapshot: ev.Case{
			InitScript:   "echo init-ok",
			TaskPrompt:   "do it",
			VerifyScript: "exit 0",
		},
	}
	instRepo := &stubInstRepo{inst: inst}
	taskRepo := &stubTaskRepo{}
	resultRepo := &stubResultRepo{}
	snapRepo := &stubSnapshotRepo{}
	chat := &stubChatRunner{res: InternalChatResult{LastAssistantMsg: "ok"}}
	skill := &stubSkillExecutor{}
	native := &stubNativeProvider{workspaceDir: t.TempDir()}

	e := NewInstanceExecutor(
		instRepo, taskRepo, resultRepo, snapRepo,
		NewVerifyRunner(2*time.Second, 4096),
		chat, nil, nil,
		skill, native,
		"worker-1", 50*time.Millisecond, 2*time.Second,
	)

	if err := e.Execute(context.Background(), "inst-1"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// 最终 result 必须存在且 passed=true
	last := resultRepo.lastResult()
	if last == nil {
		t.Fatalf("expected at least one result upserted")
	}
	if !last.Passed {
		t.Fatalf("expected passed=true, got %+v", last)
	}
}

// TestExecutor_InitScript_NonZeroFailsInit 验证：init_script exit!=0 时不应进入 chat，
// finalize 到 INITIALIZING→FAILED，result.ErrorLog 应含 exit code。
func TestExecutor_InitScript_NonZeroFailsInit(t *testing.T) {
	inst := &ev.RunInstance{
		ID:                    "inst-1",
		TaskID:                "task-1",
		MessageID:             "msg-1",
		ConversationID:        "conv-1",
		AgentConfigSnapshotID: "snap-1",
		CaseSnapshot: ev.Case{
			InitScript:   "echo boom 1>&2; exit 3",
			TaskPrompt:   "do it",
			VerifyScript: "exit 0",
		},
	}
	instRepo := &stubInstRepo{inst: inst}
	taskRepo := &stubTaskRepo{}
	resultRepo := &stubResultRepo{}
	snapRepo := &stubSnapshotRepo{}
	chat := &stubChatRunner{res: InternalChatResult{LastAssistantMsg: "ok"}}
	skill := &stubSkillExecutor{}
	native := &stubNativeProvider{workspaceDir: t.TempDir()}

	e := NewInstanceExecutor(
		instRepo, taskRepo, resultRepo, snapRepo,
		NewVerifyRunner(2*time.Second, 4096),
		chat, nil, nil,
		skill, native,
		"worker-1", 50*time.Millisecond, 2*time.Second,
	)

	if err := e.Execute(context.Background(), "inst-1"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	// chat 不应被调用
	if chat.last.Query != "" {
		t.Fatalf("chat should NOT be invoked on init failure, but was: %+v", chat.last)
	}

	// 最终 result.Passed=false 且 errorLog 提及 exit=3
	last := resultRepo.lastResult()
	if last == nil {
		t.Fatalf("expected result to be upserted with error info")
	}
	if last.Passed {
		t.Fatalf("expected passed=false, got %+v", last)
	}
	if !containsAll(last.ErrorLog, "init_script exit=3") {
		t.Fatalf("expected ErrorLog to contain 'init_script exit=3', got %q", last.ErrorLog)
	}

	// CAS 序列应包含 QUEUED→INITIALIZING 与 INITIALIZING→FAILED
	if len(instRepo.casCalls) < 2 {
		t.Fatalf("expect >=2 CAS calls, got %+v", instRepo.casCalls)
	}
	if instRepo.casCalls[1].From != ev.InstanceStatusInitializing ||
		instRepo.casCalls[1].To != ev.InstanceStatusFailed {
		t.Fatalf("expected INITIALIZING→FAILED, got %+v", instRepo.casCalls[1])
	}

	// cleanup 必须被调用
	if len(skill.cleaned) == 0 || len(native.cleaned) == 0 {
		t.Fatalf("expected cleanup to be called; skill=%v native=%v", skill.cleaned, native.cleaned)
	}
	// task recount 应触发
	if taskRepo.recountCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 recount, got %d", taskRepo.recountCalls.Load())
	}
}

// containsAll 简易断言 helper：s 是否包含所有 subs（顺序无关）
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

// contains substring search 避免引入 strings 包 —— 保留局部封装便于将来替换
func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
