package tracing

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSpan_LifecycleBasic(t *testing.T) {
	s := newTestSpan("trace-1", 1, 0, SpanTypeLLMCall, "")
	s.SetTag("k1", "v1")
	s.AddLog("INFO", "started", nil)
	time.Sleep(2 * time.Millisecond)
	s.End()

	assert.Greater(t, s.LatencyMs, int32(0))
	assert.Equal(t, "v1", s.tags["k1"])
	assert.Len(t, s.logs, 1)
	assert.Equal(t, "started", s.logs[0].Msg)
}

func TestSpan_EndIdempotent(t *testing.T) {
	committed := 0
	s := newTestSpanWithCommit("trace-1", 1, 0, SpanTypeLLMCall, "", func(*Span) { committed++ })
	s.End()
	s.End()
	assert.Equal(t, 1, committed)
}

func TestSpan_ConcurrentSetTag(t *testing.T) {
	s := newTestSpan("trace-1", 1, 0, SpanTypeLLMCall, "")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.SetTag("k", i)
			s.AddLog("INFO", "x", nil)
		}(i)
	}
	wg.Wait()
}

func TestSpan_SensitiveTagMasking(t *testing.T) {
	s := newTestSpan("t", 1, 0, SpanTypeLLMCall, "")
	s.SetTag("api_key", "sk-xxx")
	s.SetTag("Authorization", "Bearer yyy")
	s.SetTag("some_password", "pw")
	s.SetTag("normal_key", "keep")

	assert.Equal(t, "***", s.tags["api_key"])
	assert.Equal(t, "***", s.tags["Authorization"])
	assert.Equal(t, "***", s.tags["some_password"])
	assert.Equal(t, "keep", s.tags["normal_key"])
}

func TestSpan_LongValueTruncation(t *testing.T) {
	s := newTestSpan("t", 1, 0, SpanTypeAgentRoot, "")
	long := strings.Repeat("x", 2000)
	s.SetTag("user.query", long)
	v := s.tags["user.query"].(string)
	assert.LessOrEqual(t, len(v), MaxUserQueryBytes)
}

func TestSpan_MarkError(t *testing.T) {
	s := newTestSpan("t", 1, 0, SpanTypeToolCall, "fileRead")
	s.MarkError()
	assert.True(t, s.IsError)
	assert.Empty(t, s.logs, "MarkError 不再自动写日志，错误详情由调用方 AddLog 记录")
}

func newTestSpan(traceID string, spanID, parentSpanID int32, spanType SpanType, opName string) *Span {
	return newTestSpanWithCommit(traceID, spanID, parentSpanID, spanType, opName, func(*Span) {})
}

func newTestSpanWithCommit(traceID string, spanID, parentSpanID int32, spanType SpanType, opName string, commit func(*Span)) *Span {
	return &Span{
		TraceID:       traceID,
		SpanID:        spanID,
		ParentSpanID:  parentSpanID,
		SpanType:      spanType,
		OperationName: opName,
		StartTime:     time.Now().UnixNano(),
		tags:          make(map[string]interface{}),
		logs:          make([]LogEntry, 0),
		commitFn:      commit,
	}
}
