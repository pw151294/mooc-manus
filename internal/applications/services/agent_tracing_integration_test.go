// Package services 的 Agent Tracing 集成测试。
//
// 本文件覆盖 Phase 5 · Task 5.1「Chat 主流程 tracing 集成测试」的核心用例，
// 直接驱动 BaseAgent.InvokeToolCalls / 手工 round span，绕开 Application.Chat 的
// SSE writer 复杂 mock。当前必须真正 PASS 的两个用例：
//   - TestChat_HappyPath_SpanStructure
//   - TestChat_LoopContextPropagation_RoundParentIsRoot
//
// 其余用例暂 t.Skip，等 mock 能力补齐后再打开。
package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/llm"
	"mooc-manus/internal/domains/models/tracing"
)

// -----------------------------------------------------------------------------
// captureRepo 是本文件专用的 in-memory SpanRepository fake：
// BatchInsert 把 span 追加到内部切片，供断言使用。
// -----------------------------------------------------------------------------
type captureRepo struct {
	mu    sync.Mutex
	spans []*tracing.Span
}

func (r *captureRepo) BatchInsert(_ context.Context, spans []*tracing.Span) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans = append(r.spans, spans...)
	return nil
}

func (r *captureRepo) FindByTraceID(_ context.Context, _ string) ([]*tracing.SpanNode, error) {
	return nil, nil
}

func (r *captureRepo) ListTraces(_ context.Context, _ tracing.TraceFilter, _ int, _ int) ([]*tracing.TraceSummary, int64, error) {
	return nil, 0, nil
}

// snapshot 返回当前累积 span 的浅拷贝，避免并发读写切片。
func (r *captureRepo) snapshot() []*tracing.Span {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*tracing.Span, len(r.spans))
	copy(out, r.spans)
	return out
}

// countByType 统计 spans 中指定 SpanType 的个数。
func countByType(spans []*tracing.Span, st tracing.SpanType) int {
	n := 0
	for _, s := range spans {
		if s.SpanType == st {
			n++
		}
	}
	return n
}

// findTag 在 tags 中查找指定 key，返回 value 与是否存在。
func findTag(s *tracing.Span, key string) (interface{}, bool) {
	tags := s.TagsSnapshot()
	v, ok := tags[key]
	return v, ok
}

// -----------------------------------------------------------------------------
// TestChat_HappyPath_SpanStructure
// 用例 A：InvokeToolCalls 两个正常 tool call 时，span 结构必须是：
//   - 1 个 AGENT_ROOT
//   - 1 个 TOOL_BATCH（batch.tool_calls_count = 2）
//   - 2 个 TOOL_CALL（tool.name = "dummy"）
// -----------------------------------------------------------------------------
func TestChat_HappyPath_SpanStructure(t *testing.T) {
	repo := &captureRepo{}
	tr := tracing.NewTracer(repo,
		tracing.WithBatchSize(1),
		tracing.WithFlushInterval(50*time.Millisecond),
		tracing.WithBufferCapacity(1000))
	tracing.SetGlobal(tr)
	defer func() {
		_ = tr.Shutdown(context.Background())
		tracing.SetGlobal(nil)
	}()

	// 构造 agent 与 tool。mockTool.SupportsRiskAssessment=false 会跳过 HITL 闸门，
	// 走 InvokeTool 直接返回 success 的 happy path。
	tool := newMockTool("dummy", false)
	agent := buildAgent("mid-tracing-hp", nil, tool)

	// 起 root span，把 ctx 传给 InvokeToolCalls
	ctx, rootSpan := tr.StartRootSpan(context.Background(), "trace-hp")

	// 事件通道背后开一个 drainer，避免 InvokeToolCalls 阻塞发送
	eventCh := make(chan events.AgentEvent, 32)
	eventsDone := make(chan []events.AgentEvent, 1)
	go drainEvents(eventCh, eventsDone)

	toolCalls := []llm.ToolCall{
		{ID: "tc1", Name: "dummy", Arguments: "{}"},
		{ID: "tc2", Name: "dummy", Arguments: "{}"},
	}
	agent.InvokeToolCalls(ctx, toolCalls, eventCh)
	close(eventCh)
	<-eventsDone

	// 结束 root，触发 commit -> flush
	rootSpan.End()
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("tracer shutdown: %v", err)
	}

	spans := repo.snapshot()
	if len(spans) < 4 {
		t.Fatalf("spans total = %d, want >= 4", len(spans))
	}

	assert.Equal(t, 1, countByType(spans, tracing.SpanTypeAgentRoot), "AGENT_ROOT 应为 1 个")
	assert.Equal(t, 1, countByType(spans, tracing.SpanTypeToolBatch), "TOOL_BATCH 应为 1 个")
	assert.Equal(t, 2, countByType(spans, tracing.SpanTypeToolCall), "TOOL_CALL 应为 2 个")

	// 校验 TOOL_BATCH 上的 batch.tool_calls_count tag
	var batch *tracing.Span
	for _, s := range spans {
		if s.SpanType == tracing.SpanTypeToolBatch {
			batch = s
			break
		}
	}
	if batch == nil {
		t.Fatal("未找到 TOOL_BATCH span")
	}
	if v, ok := findTag(batch, "batch.tool_calls_count"); !ok {
		t.Fatal("TOOL_BATCH 缺 batch.tool_calls_count tag")
	} else {
		assert.EqualValues(t, 2, v, "batch.tool_calls_count 应为 2")
	}

	// 校验每个 TOOL_CALL 的 tool.name = "dummy"
	for _, s := range spans {
		if s.SpanType != tracing.SpanTypeToolCall {
			continue
		}
		v, ok := findTag(s, "tool.name")
		if !ok {
			t.Fatalf("TOOL_CALL span %d 缺 tool.name tag", s.SpanID)
		}
		assert.Equal(t, "dummy", v, "tool.name 应为 dummy")
	}
}

// -----------------------------------------------------------------------------
// TestChat_LoopContextPropagation_RoundParentIsRoot
// 用例 B：ReAct 主循环 N 轮时，每轮 AGENT_ROUND 的 ParentSpanID 都应等于 root.SpanID。
// 直接用 StartSpanFromContext 手工造 2 轮 round span 模拟。
// -----------------------------------------------------------------------------
func TestChat_LoopContextPropagation_RoundParentIsRoot(t *testing.T) {
	repo := &captureRepo{}
	tr := tracing.NewTracer(repo,
		tracing.WithBatchSize(1),
		tracing.WithFlushInterval(50*time.Millisecond),
		tracing.WithBufferCapacity(1000))
	tracing.SetGlobal(tr)
	defer func() {
		_ = tr.Shutdown(context.Background())
		tracing.SetGlobal(nil)
	}()

	ctx, rootSpan := tr.StartRootSpan(context.Background(), "trace-loop")
	// 模拟 2 轮 ReAct：每轮从 root ctx 派生一个 AGENT_ROUND span
	for round := 1; round <= 2; round++ {
		_, roundSpan := tracing.StartSpanFromContext(ctx, tracing.SpanTypeAgentRound, "")
		roundSpan.SetTag("round.index", round)
		roundSpan.End()
	}
	rootSpan.End()
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("tracer shutdown: %v", err)
	}

	spans := repo.snapshot()
	rootID := rootSpan.SpanID // 0

	roundCount := 0
	for _, s := range spans {
		if s.SpanType != tracing.SpanTypeAgentRound {
			continue
		}
		roundCount++
		assert.Equal(t, rootID, s.ParentSpanID,
			"AGENT_ROUND span %d parent=%d, want root %d", s.SpanID, s.ParentSpanID, rootID)
	}
	if roundCount != 2 {
		t.Fatalf("AGENT_ROUND span count = %d, want 2", roundCount)
	}
}

// -----------------------------------------------------------------------------
// 以下用例暂缓：等 mockTool / mockInvoker 支持失败注入 / 取消 / 迭代溢出 / 子智能体
// 等能力后再打开。
// -----------------------------------------------------------------------------

func TestChat_ToolError_IsErrorFlag(t *testing.T) {
	t.Skip("TODO: mock tool 失败注入待补")
}

func TestChat_ContextCancel_RootSpanClosed(t *testing.T) {
	t.Skip("TODO")
}

func TestChat_MaxIterationsExceeded_RootIsError(t *testing.T) {
	t.Skip("TODO")
}

func TestChat_HITLDangerousTool_SpanTags(t *testing.T) {
	t.Skip("TODO")
}

func TestChat_SubagentCall_SpanType(t *testing.T) {
	t.Skip("TODO")
}

func TestChat_TracerBufferFull_BusinessUnaffected(t *testing.T) {
	t.Skip("TODO")
}
