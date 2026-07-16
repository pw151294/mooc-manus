package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"mooc-manus/internal/domains/models/tracing"
)

type fakeSpanRepo struct {
	nodes []*tracing.SpanNode
	err   error
	list  []*tracing.TraceSummary
}

func (r *fakeSpanRepo) BatchInsert(context.Context, []*tracing.Span) error { return nil }
func (r *fakeSpanRepo) FindByTraceID(_ context.Context, _ string) ([]*tracing.SpanNode, error) {
	return r.nodes, r.err
}
func (r *fakeSpanRepo) ListTraces(context.Context, tracing.TraceFilter, int, int) ([]*tracing.TraceSummary, int64, error) {
	return r.list, int64(len(r.list)), nil
}
func (r *fakeSpanRepo) ListByConversationID(context.Context, string) ([]*tracing.Span, error) {
	return nil, nil
}

func TestTraceService_GetTraceDetail_HappyPath(t *testing.T) {
	repo := &fakeSpanRepo{
		nodes: []*tracing.SpanNode{
			{SpanID: 0, ParentSpanID: -1, SpanType: string(tracing.SpanTypeAgentRoot), StartTime: 100, EndTime: 200, LatencyMs: 100, Tags: map[string]interface{}{}},
			{SpanID: 1, ParentSpanID: 0, SpanType: string(tracing.SpanTypeAgentRound), Tags: map[string]interface{}{}, IsError: true},
		},
	}
	svc := NewTraceApplicationService(repo)
	dto, err := svc.GetTraceDetail(context.Background(), "t1")
	assert.NoError(t, err)
	assert.Equal(t, int32(2), dto.SpanCount)
	assert.True(t, dto.IsError)
	assert.Equal(t, int32(0), dto.Root.SpanID)
	assert.Len(t, dto.Root.Children, 1)
}

func TestTraceService_GetTraceDetail_NotFound(t *testing.T) {
	repo := &fakeSpanRepo{nodes: nil}
	svc := NewTraceApplicationService(repo)
	_, err := svc.GetTraceDetail(context.Background(), "t1")
	assert.ErrorIs(t, err, ErrTraceNotFound)
}

func TestTraceService_GetTraceDetail_RepoErr(t *testing.T) {
	repo := &fakeSpanRepo{err: errors.New("db down")}
	svc := NewTraceApplicationService(repo)
	_, err := svc.GetTraceDetail(context.Background(), "t1")
	assert.Error(t, err)
}
