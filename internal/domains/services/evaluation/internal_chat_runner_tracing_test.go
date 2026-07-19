package evaluation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/tracing"
)

// fakeSpanRepo 只捕获 BatchInsert 收到的 spans，其它方法用不到就返回零值。
type fakeSpanRepo struct {
	mu    sync.Mutex
	spans []*tracing.Span
}

func (f *fakeSpanRepo) BatchInsert(_ context.Context, spans []*tracing.Span) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spans = append(f.spans, spans...)
	return nil
}
func (f *fakeSpanRepo) FindByTraceID(context.Context, string) ([]*tracing.SpanNode, error) {
	return nil, nil
}
func (f *fakeSpanRepo) ListTraces(context.Context, tracing.TraceFilter, int, int) ([]*tracing.TraceSummary, int64, error) {
	return nil, 0, nil
}
func (f *fakeSpanRepo) ListByConversationID(context.Context, string) ([]*tracing.Span, error) {
	return nil, nil
}

func (f *fakeSpanRepo) snapshot() []*tracing.Span {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*tracing.Span, len(f.spans))
	copy(out, f.spans)
	return out
}

// installTracer 装配全局 tracer 并返回卸载函数；每个用例开头 defer 立即执行。
// flushInterval 设短一点让 span 及时落 fake repo；Shutdown 排空 buffer。
func installTracer(t *testing.T) (*fakeSpanRepo, func()) {
	t.Helper()
	repo := &fakeSpanRepo{}
	tr := tracing.NewTracer(repo,
		tracing.WithBatchSize(1),
		tracing.WithFlushInterval(10*time.Millisecond),
	)
	prev := tracing.Global()
	tracing.SetGlobal(tr)
	return repo, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tr.Shutdown(ctx)
		tracing.SetGlobal(prev)
	}
}

func findRoot(spans []*tracing.Span, traceID string) *tracing.Span {
	for _, s := range spans {
		if s.TraceID == traceID && s.SpanType == tracing.SpanTypeAgentRoot {
			return s
		}
	}
	return nil
}

// 正常收敛：eventCh 关闭 → 至少 1 条 AGENT_ROOT span 被 BatchInsert，IsError=false，
// ConversationID / traceID / user.query tag 齐全。
func TestInternalChatRunner_Tracing_NormalCommitsRootSpan(t *testing.T) {
	repo, teardown := installTracer(t)
	defer teardown()

	fake := &fakeBaseAgent{fn: func(ctx context.Context, req agents.ChatRequest, ch chan events.AgentEvent) {
		// 断言 ctx 已挂 root span（否则 domain 层子 span 会全部 no-op）
		assert.NotNil(t, tracing.SpanFromContext(ctx), "baseAgent.Chat 收到的 ctx 必须携带 root span")
		msg := &events.MessageEvent{Role: "assistant", Message: "ok"}
		msg.Type = events.EventTypeMessage
		ch <- msg
		close(ch)
	}}
	r := NewInternalChatRunner(fake)
	res, err := r.Run(context.Background(), InternalChatReq{
		Snapshot:       makeSnapshot(),
		ConversationID: "conv-x",
		MessageID:      "msg-x",
		Query:          "hello",
		TotalTimeout:   time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, res.Error)

	// 等 fake repo 收到 span（flush interval 10ms + Shutdown drain 兜底）
	require.Eventually(t, func() bool {
		return findRoot(repo.snapshot(), "msg-x") != nil
	}, time.Second, 20*time.Millisecond)

	root := findRoot(repo.snapshot(), "msg-x")
	require.NotNil(t, root)
	assert.False(t, root.IsError)
	assert.Equal(t, "conv-x", root.ConversationID)
	tags := root.TagsSnapshot()
	assert.Equal(t, "hello", tags["user.query"])
	assert.Equal(t, "cfg-1", tags["evaluation.source_app_config_id"])
}

// ErrorEvent：首条错误应触发 rootSpan.MarkError + AddLog("eval.stream_error")。
func TestInternalChatRunner_Tracing_ErrorEventMarksRoot(t *testing.T) {
	repo, teardown := installTracer(t)
	defer teardown()

	fake := &fakeBaseAgent{fn: func(ctx context.Context, req agents.ChatRequest, ch chan events.AgentEvent) {
		e1 := &events.ErrorEvent{Error: "boom"}
		e1.Type = events.EventTypeError
		e2 := &events.ErrorEvent{Error: "second"}
		e2.Type = events.EventTypeError
		ch <- e1
		ch <- e2 // 第二条错误只补日志与否由实现决定，本用例不做强断言
		close(ch)
	}}
	r := NewInternalChatRunner(fake)
	res, err := r.Run(context.Background(), InternalChatReq{
		Snapshot:     makeSnapshot(),
		MessageID:    "msg-err",
		TotalTimeout: time.Second,
	})
	require.NoError(t, err)
	require.Error(t, res.Error)

	require.Eventually(t, func() bool {
		return findRoot(repo.snapshot(), "msg-err") != nil
	}, time.Second, 20*time.Millisecond)

	root := findRoot(repo.snapshot(), "msg-err")
	require.NotNil(t, root)
	assert.True(t, root.IsError, "首条 ErrorEvent 应把 root span 标为错误")

	// 至少有一条 log msg == "eval.stream_error" 且 extra.error 含首条错误内容
	logs := root.LogsSnapshot()
	var found bool
	var streamErrorCount int
	for _, l := range logs {
		if l.Msg == "eval.stream_error" {
			streamErrorCount++
			if e, ok := l.Extra["error"].(string); ok && e == "boom" {
				found = true
			}
		}
	}
	assert.True(t, found, "logs 中应含 eval.stream_error 且 extra.error==boom, got=%+v", logs)
	// spec Q4: MarkError + AddLog 只对首条 ErrorEvent 触发一次；第二条错误不重复记录
	assert.Equal(t, 1, streamErrorCount, "eval.stream_error 应仅记录一次（首条错误），got=%d", streamErrorCount)
}

// TotalTimeout 触发 → DidTimeout=true 且 root span IsError=true，logs 含 eval.timeout。
func TestInternalChatRunner_Tracing_TimeoutMarksRoot(t *testing.T) {
	repo, teardown := installTracer(t)
	defer teardown()

	fake := &fakeBaseAgent{fn: func(ctx context.Context, req agents.ChatRequest, ch chan events.AgentEvent) {
		<-ctx.Done()
		close(ch)
	}}
	r := NewInternalChatRunner(fake)
	res, err := r.Run(context.Background(), InternalChatReq{
		Snapshot:     makeSnapshot(),
		MessageID:    "msg-to",
		TotalTimeout: 80 * time.Millisecond,
	})
	require.NoError(t, err)
	assert.True(t, res.DidTimeout)

	require.Eventually(t, func() bool {
		return findRoot(repo.snapshot(), "msg-to") != nil
	}, time.Second, 20*time.Millisecond)

	root := findRoot(repo.snapshot(), "msg-to")
	require.NotNil(t, root)
	assert.True(t, root.IsError, "DeadlineExceeded 应把 root 标为错误")

	logs := root.LogsSnapshot()
	var found bool
	for _, l := range logs {
		if l.Msg == "eval.timeout" {
			found = true
			break
		}
	}
	assert.True(t, found, "logs 应含 eval.timeout, got=%+v", logs)
}

// 主动 Cancel（非超时）→ root.IsError 保持 false（澄清 Q6：cancel 是正常终止）。
func TestInternalChatRunner_Tracing_CancelDoesNotMarkRoot(t *testing.T) {
	repo, teardown := installTracer(t)
	defer teardown()

	fake := &fakeBaseAgent{fn: func(ctx context.Context, req agents.ChatRequest, ch chan events.AgentEvent) {
		<-ctx.Done()
		close(ch)
	}}
	r := NewInternalChatRunner(fake)

	ctx, cancel := context.WithCancel(context.Background())
	// 20ms 后主动 cancel，制造非 Deadline 的 ctx.Done() 触发
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	res, err := r.Run(ctx, InternalChatReq{
		Snapshot:     makeSnapshot(),
		MessageID:    "msg-cancel",
		TotalTimeout: 2 * time.Second, // 保证不是 Deadline 先触发
	})
	require.NoError(t, err)
	assert.False(t, res.DidTimeout, "主动 cancel 不应被识别为 timeout")

	require.Eventually(t, func() bool {
		return findRoot(repo.snapshot(), "msg-cancel") != nil
	}, time.Second, 20*time.Millisecond)

	root := findRoot(repo.snapshot(), "msg-cancel")
	require.NotNil(t, root)
	assert.False(t, root.IsError, "主动 cancel 属于正常终止，不应 MarkError")
}
