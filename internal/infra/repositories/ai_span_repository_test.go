//go:build integration

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mooc-manus/internal/domains/models/tracing"
)

// 需要在本地起一个 PostgreSQL 并已应用 ai_span 表
func TestAiSpanRepository_BatchInsertAndFind(t *testing.T) {
	repo := NewAiSpanRepository()
	now := time.Now().UnixNano()
	span := &tracing.Span{
		TraceID:      "trace-repo-test-1",
		SpanID:       0,
		ParentSpanID: -1,
		SpanType:     tracing.SpanTypeAgentRoot,
		StartTime:    now,
		EndTime:      now + int64(time.Millisecond)*10,
		LatencyMs:    10,
	}
	span.SetTag("k", "v")
	span.AddLog("INFO", "start", nil)

	err := repo.BatchInsert(context.Background(), []*tracing.Span{span})
	assert.NoError(t, err)

	nodes, err := repo.FindByTraceID(context.Background(), "trace-repo-test-1")
	assert.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Equal(t, int32(0), nodes[0].SpanID)
	assert.Equal(t, "v", nodes[0].Tags["k"])
}
