package services

import (
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"mooc-manus/config"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/sse"
	"mooc-manus/pkg/logger"
)

// TestMain 初始化全局 logger，StopMessage / StopConversation 内部 logger.Info 依赖之
func TestMain(m *testing.M) {
	tmpLogDir, _ := os.MkdirTemp("", "agent-stop-test-log-*")
	_ = logger.InitGlobalLogger(config.LoggerConfig{
		Level:  "info",
		Format: "console",
		Output: "stdout",
		LogDir: tmpLogDir,
	})
	code := m.Run()
	_ = os.RemoveAll(tmpLogDir)
	os.Exit(code)
}

// fakeSkillExecutor 记录 CleanupMessage 调用序列，可注入错误
type fakeSkillExecutor struct {
	cleaned  []string
	failWith error
	calls    atomic.Int32
}

func (f *fakeSkillExecutor) Execute(ctx tools.SkillExecutionContext, bashCommand string) ([]tools.SkillExecutionResult, error) {
	return nil, nil
}

func (f *fakeSkillExecutor) CleanupMessage(messageID string) error {
	f.calls.Add(1)
	f.cleaned = append(f.cleaned, messageID)
	return f.failWith
}

// fakeNativeToolsProvider 记录 Cleanup 调用序列，可注入错误
type fakeNativeToolsProvider struct {
	cleaned  []string
	failWith error
	calls    atomic.Int32
}

func (f *fakeNativeToolsProvider) BuildTools(messageId, conversationId string) ([]tools.Tool, error) {
	return nil, nil
}

func (f *fakeNativeToolsProvider) Cleanup(messageId string) error {
	f.calls.Add(1)
	f.cleaned = append(f.cleaned, messageId)
	return f.failWith
}

func (f *fakeNativeToolsProvider) ConversationPlanDir(conversationId string) string {
	return "/tmp/plans/" + conversationId
}

func (f *fakeNativeToolsProvider) MessageWorkspaceDir(messageId string) string {
	return "/tmp/native-workspace/" + messageId
}

func newSvc(skillFail, nativeFail error) (*BaseAgentApplicationServiceImpl, *fakeSkillExecutor, *fakeNativeToolsProvider) {
	skill := &fakeSkillExecutor{failWith: skillFail}
	native := &fakeNativeToolsProvider{failWith: nativeFail}
	svc := &BaseAgentApplicationServiceImpl{
		agentDomainSvc:      nil, // Stop 路径不依赖 domain svc
		skillExecutor:       skill,
		nativeToolsProvider: native,
	}
	return svc, skill, native
}

func TestStopMessage_NotFound_ReturnsNoopCleanFalse(t *testing.T) {
	svc, skill, native := newSvc(nil, nil)

	result := svc.StopMessage("unknown-mid")

	if result.MessageId != "unknown-mid" {
		t.Fatalf("messageId not echoed, got %q", result.MessageId)
	}
	// sse=false（不存在），skill/native 仍会尝试清理（幂等）返回 true
	if result.Cleaned.SSE {
		t.Fatal("SSE should be false for unknown messageId")
	}
	if !result.Cleaned.Skill {
		t.Fatal("Skill should be true (cleanup is idempotent no-op)")
	}
	if !result.Cleaned.NativeWorkspace {
		t.Fatal("NativeWorkspace should be true")
	}
	if skill.calls.Load() != 1 || native.calls.Load() != 1 {
		t.Fatal("cleanup should be attempted even when messageId unknown to sse")
	}
}

func TestStopMessage_ActiveSSE_ClosesConnection(t *testing.T) {
	svc, _, _ := newSvc(nil, nil)

	// 先建一条真实 SSE 连接
	mid := sse.StartChat(newFakeSSEWriter(), "conv-active")
	defer sse.CloseChat(mid) // 兜底，避免用例遗留

	result := svc.StopMessage(mid)

	if !result.Cleaned.SSE {
		t.Fatal("SSE should be true for an active messageId")
	}
	if sse.HasMessage(mid) {
		t.Fatal("HasMessage should be false after StopMessage")
	}
}

// TestStopMessage_SubCleanupError_DoesNotBlockOthers 单一子清理返错时其他步骤仍然执行
func TestStopMessage_SubCleanupError_DoesNotBlockOthers(t *testing.T) {
	svc, skill, native := newSvc(errors.New("skill fail"), nil)

	mid := sse.StartChat(newFakeSSEWriter(), "conv-partial")
	defer sse.CloseChat(mid)

	result := svc.StopMessage(mid)

	if !result.Cleaned.SSE {
		t.Fatal("SSE closed step should still succeed")
	}
	if result.Cleaned.Skill {
		t.Fatal("Skill should be false since cleanup returned error")
	}
	if !result.Cleaned.NativeWorkspace {
		t.Fatal("Native cleanup should still run after skill cleanup fails")
	}
	if skill.calls.Load() != 1 || native.calls.Load() != 1 {
		t.Fatal("both cleanups should have been attempted exactly once")
	}
}

// TestStopConversation_CleansAllActiveMessages 覆盖多 messageId + memory
func TestStopConversation_CleansAllActiveMessages(t *testing.T) {
	svc, skill, native := newSvc(nil, nil)

	cid := "conv-multi"
	m1 := sse.StartChat(newFakeSSEWriter(), cid)
	m2 := sse.StartChat(newFakeSSEWriter(), cid)

	result := svc.StopConversation(cid)

	if result.ConversationId != cid {
		t.Fatalf("conversationId not echoed, got %q", result.ConversationId)
	}
	if !result.Cleaned.Memory {
		t.Fatal("Memory should be true after DeleteMemory")
	}
	if len(result.Cleaned.Messages) != 2 {
		t.Fatalf("Cleaned.Messages should list both mids, got %v", result.Cleaned.Messages)
	}
	if sse.HasMessage(m1) || sse.HasMessage(m2) {
		t.Fatal("both messages should be closed after StopConversation")
	}
	if skill.calls.Load() != 2 || native.calls.Load() != 2 {
		t.Fatalf("each cleanup should run once per messageId, skill=%d native=%d",
			skill.calls.Load(), native.calls.Load())
	}
}

func TestStopConversation_EmptyConversationId_IsNoop(t *testing.T) {
	svc, skill, native := newSvc(nil, nil)
	result := svc.StopConversation("")
	if result.Cleaned.Memory {
		t.Fatal("empty conversationId should not touch memory")
	}
	if skill.calls.Load() != 0 || native.calls.Load() != 0 {
		t.Fatal("empty conversationId should skip all cleanup")
	}
}
