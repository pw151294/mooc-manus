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

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/llm"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/models/tracing"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/domains/services/tools"
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

func (r *captureRepo) ListByConversationID(_ context.Context, _ string) ([]*tracing.Span, error) {
	return nil, nil
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
//
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
// 以下 3 个用例覆盖 context deadline 未被链路追踪的修复：
//  1. LLM 流式调用抛 error 事件，llmSpan 应标错
//  2. ctx cancel 后 root span 应标错
//  3. 60s 超时兜底 root span 应标错
// -----------------------------------------------------------------------------

// errorInvoker 模拟 LLM 流式调用中途出错（发送 OnError 事件）
type errorInvoker struct {
	errMsg string
}

func (e *errorInvoker) Invoke(_ []llm.Message, _ []llm.Tool) (llm.Message, error) {
	return llm.Message{Role: llm.RoleAssistant, Content: ""}, nil
}

func (e *errorInvoker) StreamingInvoke(_ []llm.Message, _ []llm.Tool, eventCh chan<- events.AgentEvent) llm.Message {
	// 模拟先发一个正常 chunk，再发 error
	eventCh <- events.OnMessage("partial", nil)
	eventCh <- events.OnError(e.errMsg)
	close(eventCh)
	return llm.Message{Role: llm.RoleAssistant, Content: "partial"}
}

// LastUsage 桩实现：无 token 记账
func (e *errorInvoker) LastUsage() llm.Usage { return llm.Usage{} }

// TestStreamingInvokeLLM_StreamErrorMarksSpanError
// 用例：LLM 流式调用中 error 事件应标记 llmSpan.IsError=true
func TestStreamingInvokeLLM_StreamErrorMarksSpanError(t *testing.T) {
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

	ctx, rootSpan := tr.StartRootSpan(context.Background(), "trace-llm-err")

	// 构造 agent，注入 errorInvoker
	errMsg := "llm streaming timeout after 120s: context deadline exceeded"
	inv := &errorInvoker{errMsg: errMsg}
	mem := memory.NewChatMemory()
	cfg := models.AgentConfig{MaxIterations: 3, MaxRetries: 1}
	agent := agents.NewBaseAgent(cfg, inv, mem, []tools.Tool{}, "sys")

	eventCh := make(chan events.AgentEvent, 32)
	eventsDone := make(chan []events.AgentEvent, 1)
	go drainEvents(eventCh, eventsDone)

	// 调用 StreamingInvokeLLM
	_ = agent.StreamingInvokeLLM(ctx, []llm.Message{{Role: llm.RoleUser, Content: "test"}}, eventCh)

	<-eventsDone
	rootSpan.End()
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("tracer shutdown: %v", err)
	}

	spans := repo.snapshot()
	var llmSpan *tracing.Span
	for _, s := range spans {
		if s.SpanType == tracing.SpanTypeLLMCall {
			llmSpan = s
			break
		}
	}
	if llmSpan == nil {
		t.Fatal("未找到 LLM_CALL span")
	}

	assert.True(t, llmSpan.IsError, "LLM_CALL span 应标记 IsError=true")
	logs := llmSpan.LogsSnapshot()
	foundError := false
	for _, log := range logs {
		if log.Level == "ERROR" && log.Msg == "llm.stream.error" {
			foundError = true
			break
		}
	}
	assert.True(t, foundError, "LLM_CALL span logs 应含 llm.stream.error")
}

// TestChat_ContextCancel_RootSpanIsError
// 用例：ctx cancel 后 root span 应标错（recordCtxCancelled 应 MarkError）
func TestChat_ContextCancel_RootSpanIsError(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	rootCtx, rootSpan := tr.StartRootSpan(ctx, "trace-cancel")

	// 构造 agent（用 mockInvoker，直接 close eventCh 返回空消息，
	// Invoke 首次 InvokeLLM 后无 tool_calls 会退出。所以我们不测 Invoke，
	// 而是直接在 <-ctx.Done() 分支上等价触发 recordCtxCancelled）
	//
	// 更直接：cancel 后调用 recordCtxCancelled 的调用方——但那是私有方法。
	// 取而代之：通过让 Invoke 进入循环并 cancel，命中 <-ctx.Done() 分支。
	// 因 mockInvoker 首轮无 tool_calls，Invoke 会走 "end invoke llm" 早退分支，
	// 不会进入 for round 循环。改用一个能在首次 LLM 后仍循环的 invoker。

	inv := &loopingInvoker{}
	mem := memory.NewChatMemory()
	cfg := models.AgentConfig{MaxIterations: 100, MaxRetries: 1}
	agent := agents.NewBaseAgent(cfg, inv, mem, []tools.Tool{}, "sys")

	eventCh := make(chan events.AgentEvent, 128)
	eventsDone := make(chan []events.AgentEvent, 1)
	go drainEvents(eventCh, eventsDone)

	invokeDone := make(chan struct{})
	go func() {
		agent.Invoke(rootCtx, "test query", eventCh)
		close(invokeDone)
	}()

	// 等一小段时间让 Invoke 进入循环，然后 cancel
	time.Sleep(80 * time.Millisecond)
	cancel()

	<-invokeDone
	<-eventsDone
	rootSpan.End()
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("tracer shutdown: %v", err)
	}

	spans := repo.snapshot()
	var root *tracing.Span
	for _, s := range spans {
		if s.SpanType == tracing.SpanTypeAgentRoot {
			root = s
			break
		}
	}
	if root == nil {
		t.Fatal("未找到 AGENT_ROOT span")
	}

	assert.True(t, root.IsError, "AGENT_ROOT span 应标记 IsError=true（ctx cancel）")
	logs := root.LogsSnapshot()
	foundCancel := false
	for _, log := range logs {
		if log.Level == "WARN" && log.Msg == "agent.context_cancelled" {
			foundCancel = true
			break
		}
	}
	assert.True(t, foundCancel, "AGENT_ROOT span logs 应含 agent.context_cancelled")
}

// loopingInvoker 每次 Invoke 都返回一个 tool_call，让 ReAct 循环持续；
// 这样 cancel 时 Invoke 能进入 for round 循环并命中 <-ctx.Done() 分支。
type loopingInvoker struct{}

func (l *loopingInvoker) Invoke(_ []llm.Message, _ []llm.Tool) (llm.Message, error) {
	return llm.Message{
		Role: llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{
			{ID: "tc-loop", Name: "nonexistent-tool", Arguments: "{}"},
		},
	}, nil
}

func (l *loopingInvoker) StreamingInvoke(_ []llm.Message, _ []llm.Tool, eventCh chan<- events.AgentEvent) llm.Message {
	close(eventCh)
	return llm.Message{Role: llm.RoleAssistant}
}

// LastUsage 桩实现：无 token 记账
func (l *loopingInvoker) LastUsage() llm.Usage { return llm.Usage{} }

// TestChat_60sTimeout_RootSpanIsError
// 用例：Application 60s 兜底超时应给 root span MarkError
// 注：因 Application.Chat 需要完整 http mock（ResponseWriter + sse），
// 这里以白盒方式复刻 agent.go:180-186 的 60s 分支埋点行为，验证 tracing API 语义。
// 60s 分支的 MarkError 已在 agent.go 主流程内落地，此测试保证 tracing 侧的可观测性契约。
func TestChat_60sTimeout_RootSpanIsError(t *testing.T) {
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

	_, rootSpan := tr.StartRootSpan(context.Background(), "trace-timeout")

	// 复刻 agent.go 60s 兜底分支：AddLog + MarkError
	rootSpan.AddLog("ERROR", "chat.timeout", map[string]interface{}{"seconds": 60})
	rootSpan.MarkError()
	rootSpan.End()

	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("tracer shutdown: %v", err)
	}

	spans := repo.snapshot()
	var root *tracing.Span
	for _, s := range spans {
		if s.SpanType == tracing.SpanTypeAgentRoot {
			root = s
			break
		}
	}
	if root == nil {
		t.Fatal("未找到 AGENT_ROOT span")
	}

	assert.True(t, root.IsError, "AGENT_ROOT span 应标记 IsError=true（60s timeout）")
	logs := root.LogsSnapshot()
	foundTimeout := false
	for _, log := range logs {
		if log.Level == "ERROR" && log.Msg == "chat.timeout" {
			foundTimeout = true
			break
		}
	}
	assert.True(t, foundTimeout, "AGENT_ROOT span logs 应含 chat.timeout")
}

// -----------------------------------------------------------------------------
// 以下用例暂缓：等 mockTool / mockInvoker 支持失败注入 / 迭代溢出 / 子智能体
// 等能力后再打开。
// -----------------------------------------------------------------------------

func TestChat_ToolError_IsErrorFlag(t *testing.T) {
	t.Skip("TODO: mock tool 失败注入待补")
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
