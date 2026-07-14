package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpanFromContext_Empty(t *testing.T) {
	s := SpanFromContext(context.Background())
	assert.NotNil(t, s)
	s.SetTag("k", "v")
	s.AddLog("INFO", "x", nil)
	s.SetAgentName("n")
	s.End()
}

func TestContextWithSpan_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := &Span{TraceID: "t1", SpanID: 5}
	ctx2 := contextWithSpan(ctx, s)
	got := SpanFromContext(ctx2)
	assert.Equal(t, "t1", got.TraceID)
	assert.Equal(t, int32(5), got.SpanID)
}

func TestSha256Prefix(t *testing.T) {
	got := Sha256Prefix("hello world", 8)
	assert.Len(t, got, 8)
	assert.NotEqual(t, "hello wo", got)
}
