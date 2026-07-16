package evaluation

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"mooc-manus/internal/domains/models/tracing"
)

// stubSpanRepoForExecutor 独立于 aggregator 测试的桩，避免与 stubSpanRepo 冲突
type stubSpanRepoForExecutor struct {
	spans []*tracing.Span
	err   error
}

func (r *stubSpanRepoForExecutor) BatchInsert(_ context.Context, _ []*tracing.Span) error {
	return nil
}
func (r *stubSpanRepoForExecutor) FindByTraceID(_ context.Context, _ string) ([]*tracing.SpanNode, error) {
	return nil, nil
}
func (r *stubSpanRepoForExecutor) ListTraces(_ context.Context, _ tracing.TraceFilter, _, _ int) ([]*tracing.TraceSummary, int64, error) {
	return nil, 0, nil
}
func (r *stubSpanRepoForExecutor) ListByConversationID(_ context.Context, _ string) ([]*tracing.Span, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.spans, nil
}

// TestExecutor_FinalizeVerify_AggregatesMetricsAndBackfillsTraceID
// 覆盖 4.5.4：finalize 阶段应聚合 metrics、把 trace_id 回填到 instance、result 落库、cleanup 触发。
func TestExecutor_FinalizeVerify_AggregatesMetricsAndBackfillsTraceID(t *testing.T) {
	inst := makeInst("exit 0")
	chat := &stubChatRunner{res: InternalChatResult{LastAssistantMsg: "ok"}}

	instRepo := &stubInstRepo{inst: inst}
	taskRepo := &stubTaskRepo{}
	resultRepo := &stubResultRepo{}
	snapRepo := &stubSnapshotRepo{}
	skill := &stubSkillExecutor{}
	native := &stubNativeProvider{workspaceDir: t.TempDir()}

	spanRepo := &stubSpanRepoForExecutor{
		spans: []*tracing.Span{
			tracing.NewSpanForQuery(&tracing.AiSpanPOFields{
				TraceID:   "tr-x",
				SpanType:  string(tracing.SpanTypeAgentRoot),
				LatencyMs: 1234,
			}),
			tracing.NewSpanForQuery(&tracing.AiSpanPOFields{
				TraceID:  "tr-x",
				SpanType: string(tracing.SpanTypeLLMCall),
				Tags: map[string]interface{}{
					"llm.io.prompt_units":     float64(11),
					"llm.io.completion_units": float64(22),
					"llm.io.total_units":      float64(33),
				},
			}),
		},
	}
	agg := NewTraceAggregator(spanRepo, zap.NewNop())

	e := NewInstanceExecutor(
		instRepo, taskRepo, resultRepo, snapRepo,
		NewVerifyRunner(2*time.Second, 4096),
		chat, agg, nil,
		skill, native,
		"worker-1", 50*time.Millisecond, 2*time.Second,
		zap.NewNop(),
	)

	if err := e.Execute(context.Background(), "inst-1"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	last := resultRepo.lastResult()
	if last == nil || !last.Passed {
		t.Fatalf("expected passed=true result, got %+v", last)
	}
	if last.AgentLatencyMs != 1234 {
		t.Fatalf("AgentLatencyMs=1234 expected, got %d", last.AgentLatencyMs)
	}
	if last.PromptTokens != 11 || last.CompletionTokens != 22 || last.TotalTokens != 33 {
		t.Fatalf("token metrics wrong: %+v", last)
	}

	// trace_id 回填
	if len(instRepo.updateTraceCalls) != 1 || instRepo.updateTraceCalls[0] != "tr-x" {
		t.Fatalf("expected trace_id backfill to 'tr-x', got %+v", instRepo.updateTraceCalls)
	}
	// cleanup
	if len(skill.cleaned) != 1 || skill.cleaned[0] != "msg-1" {
		t.Fatalf("skill cleanup wrong: %+v", skill.cleaned)
	}
	if len(native.cleaned) != 1 || native.cleaned[0] != "msg-1" {
		t.Fatalf("native cleanup wrong: %+v", native.cleaned)
	}
	// task recount 触发一次
	if taskRepo.recountCalls.Load() != 1 {
		t.Fatalf("expected 1 recount, got %d", taskRepo.recountCalls.Load())
	}
}

// TestExecutor_FinalizeVerify_DegradedNoRootAcceptable
// 覆盖 §11 风险 5：aggregator 降级（无 AGENT_ROOT）也不能阻塞落 result。
// 此外还验证：即便 metrics.TraceID 存在（fallback 到第一条 span 的 trace_id），也会回填。
func TestExecutor_FinalizeVerify_DegradedNoRootAcceptable(t *testing.T) {
	inst := makeInst("exit 0")
	chat := &stubChatRunner{res: InternalChatResult{LastAssistantMsg: "ok"}}

	instRepo := &stubInstRepo{inst: inst}
	taskRepo := &stubTaskRepo{}
	resultRepo := &stubResultRepo{}
	snapRepo := &stubSnapshotRepo{}
	skill := &stubSkillExecutor{}
	native := &stubNativeProvider{workspaceDir: t.TempDir()}

	// 只有 LLM_CALL，没有 AGENT_ROOT → aggregator 降级但 TraceID fallback
	spanRepo := &stubSpanRepoForExecutor{
		spans: []*tracing.Span{
			tracing.NewSpanForQuery(&tracing.AiSpanPOFields{
				TraceID:  "tr-y",
				SpanType: string(tracing.SpanTypeLLMCall),
				Tags: map[string]interface{}{
					"llm.io.total_units": float64(7),
				},
			}),
		},
	}
	agg := NewTraceAggregator(spanRepo, zap.NewNop())

	e := NewInstanceExecutor(
		instRepo, taskRepo, resultRepo, snapRepo,
		NewVerifyRunner(2*time.Second, 4096),
		chat, agg, nil,
		skill, native,
		"worker-1", 50*time.Millisecond, 2*time.Second,
		zap.NewNop(),
	)

	if err := e.Execute(context.Background(), "inst-1"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	last := resultRepo.lastResult()
	if last == nil || !last.Passed {
		t.Fatalf("expected passed=true even on degraded metrics, got %+v", last)
	}
	if last.TotalTokens != 7 {
		t.Fatalf("expected total tokens=7 to be preserved on degrade, got %d", last.TotalTokens)
	}
	// AgentLatency 应保持 0（降级）
	if last.AgentLatencyMs != 0 {
		t.Fatalf("expected AgentLatencyMs=0 on degrade, got %d", last.AgentLatencyMs)
	}
	// trace_id 仍然应回填（挑第一条 span）
	if len(instRepo.updateTraceCalls) != 1 || instRepo.updateTraceCalls[0] != "tr-y" {
		t.Fatalf("expected trace_id backfill to 'tr-y', got %+v", instRepo.updateTraceCalls)
	}
}

// TestExecutor_FinalizeVerify_HeartbeatFires 验证：执行期间心跳 goroutine 定时刷 heartbeat_at
func TestExecutor_FinalizeVerify_HeartbeatFires(t *testing.T) {
	inst := makeInst("sleep 0.2; exit 0")
	chat := &stubChatRunner{
		res:   InternalChatResult{LastAssistantMsg: "ok"},
		delay: 200 * time.Millisecond, // 让 chat 慢一点，给心跳留几次触发窗口
	}

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
		"worker-1", 30*time.Millisecond, 2*time.Second,
		zap.NewNop(),
	)

	if err := e.Execute(context.Background(), "inst-1"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// 期待心跳至少触发一次（保守）
	if instRepo.heartbeatCount.Load() < 1 {
		t.Fatalf("expected >=1 heartbeat, got %d", instRepo.heartbeatCount.Load())
	}
}

// TestExecutor_FinalizeError_TruncatesLongMessage 验证 truncate 生效
func TestExecutor_FinalizeError_TruncatesLongMessage(t *testing.T) {
	if got := truncate("abcdef", 3); got != "abc\n[truncated]" {
		t.Fatalf("truncate mismatch: %q", got)
	}
	if got := truncate("short", 100); got != "short" {
		t.Fatalf("no truncate expected: %q", got)
	}
}
