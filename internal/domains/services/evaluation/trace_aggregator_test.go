package evaluation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mooc-manus/internal/domains/models/tracing"
)

// stubSpanRepo 是本文件专用的 SpanRepository fake：
// 只实现 ListByConversationID，其余方法返回零值满足接口。
type stubSpanRepo struct {
	spans []*tracing.Span
	err   error
}

func (r *stubSpanRepo) BatchInsert(_ context.Context, _ []*tracing.Span) error {
	return nil
}

func (r *stubSpanRepo) FindByTraceID(_ context.Context, _ string) ([]*tracing.SpanNode, error) {
	return nil, nil
}

func (r *stubSpanRepo) ListTraces(_ context.Context, _ tracing.TraceFilter, _ int, _ int) ([]*tracing.TraceSummary, int64, error) {
	return nil, 0, nil
}

func (r *stubSpanRepo) ListByConversationID(_ context.Context, _ string) ([]*tracing.Span, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.spans, nil
}

// makeSpan 构造一个查询侧 Span，用于覆盖 aggregator 各分支。
func makeSpan(typ tracing.SpanType, latency int32, tags map[string]interface{}, traceID string) *tracing.Span {
	return tracing.NewSpanForQuery(&tracing.AiSpanPOFields{
		TraceID:   traceID,
		SpanType:  string(typ),
		LatencyMs: latency,
		Tags:      tags,
	})
}

// 场景 1：无 AGENT_ROOT span → 降级 (Degraded=true)，token 仍按 LLM_CALL 汇总。
func TestAggregate_NoRoot_降级(t *testing.T) {
	repo := &stubSpanRepo{spans: []*tracing.Span{
		makeSpan(tracing.SpanTypeLLMCall, 100, map[string]interface{}{
			"llm.io.prompt_units":     float64(10),
			"llm.io.completion_units": float64(20),
			"llm.io.total_units":      float64(30),
		}, "tr1"),
	}}
	agg := NewTraceAggregator(repo)
	m, err := agg.Aggregate(context.Background(), "conv-1")
	require.NoError(t, err)
	assert.True(t, m.Degraded, "无 AGENT_ROOT 应降级")
	assert.Equal(t, int64(10), m.PromptTokens)
	assert.Equal(t, int64(20), m.CompletionTokens)
	assert.Equal(t, int64(30), m.TotalTokens)
	assert.Equal(t, "tr1", m.TraceID, "降级路径下应挑第一条 span 的 trace_id")
	assert.Equal(t, int64(0), m.AgentLatencyMs)
}

// 场景 2：多个 AGENT_ROOT → 取最大 LatencyMs；Degraded=false；不报错。
func TestAggregate_MultipleRoot_取最大延迟(t *testing.T) {
	repo := &stubSpanRepo{spans: []*tracing.Span{
		makeSpan(tracing.SpanTypeAgentRoot, 500, nil, "tr2"),
		makeSpan(tracing.SpanTypeAgentRoot, 1500, nil, "tr2"),
	}}
	agg := NewTraceAggregator(repo)
	m, err := agg.Aggregate(context.Background(), "conv-2")
	require.NoError(t, err)
	assert.False(t, m.Degraded)
	assert.Equal(t, int64(1500), m.AgentLatencyMs)
	assert.Equal(t, "tr2", m.TraceID)
}

// 场景 3：AGENT_ROOT 存在但 LLM_CALL 缺 usage tag → 不降级、token=0、延迟正常。
func TestAggregate_LLMCallMissingUsage_跳过(t *testing.T) {
	repo := &stubSpanRepo{spans: []*tracing.Span{
		makeSpan(tracing.SpanTypeAgentRoot, 800, nil, "tr3"),
		makeSpan(tracing.SpanTypeLLMCall, 200, map[string]interface{}{}, "tr3"),
	}}
	agg := NewTraceAggregator(repo)
	m, err := agg.Aggregate(context.Background(), "conv-3")
	require.NoError(t, err)
	assert.False(t, m.Degraded)
	assert.Equal(t, int64(800), m.AgentLatencyMs)
	assert.Equal(t, int64(0), m.TotalTokens)
	assert.Equal(t, "tr3", m.TraceID)
}

// 场景 4：Repository 报错 → 透传错误。
func TestAggregate_RepoError_透传(t *testing.T) {
	wantErr := errors.New("boom")
	repo := &stubSpanRepo{err: wantErr}
	agg := NewTraceAggregator(repo)
	m, err := agg.Aggregate(context.Background(), "conv-err")
	assert.Nil(t, m)
	assert.ErrorIs(t, err, wantErr)
}

// 场景 5：混合 int64 / int / float64 tag 值都能加总（兼容 json.Unmarshal 默认 float64）。
func TestAggregate_TagAsInt64_兼容多类型(t *testing.T) {
	repo := &stubSpanRepo{spans: []*tracing.Span{
		makeSpan(tracing.SpanTypeAgentRoot, 100, nil, "tr5"),
		makeSpan(tracing.SpanTypeLLMCall, 50, map[string]interface{}{
			"llm.io.prompt_units":     int64(1),
			"llm.io.completion_units": int(2),
			"llm.io.total_units":      float64(3),
		}, "tr5"),
		makeSpan(tracing.SpanTypeLLMCall, 60, map[string]interface{}{
			"llm.io.prompt_units":     float64(10),
			"llm.io.completion_units": int64(20),
			"llm.io.total_units":      int(30),
		}, "tr5"),
	}}
	agg := NewTraceAggregator(repo)
	m, err := agg.Aggregate(context.Background(), "conv-5")
	require.NoError(t, err)
	assert.Equal(t, int64(11), m.PromptTokens)
	assert.Equal(t, int64(22), m.CompletionTokens)
	assert.Equal(t, int64(33), m.TotalTokens)
}
