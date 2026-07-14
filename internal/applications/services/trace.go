package services

import (
	"context"
	"errors"

	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/models/tracing"
)

// ErrTraceNotFound 表示查询的 trace 不存在
var ErrTraceNotFound = errors.New("trace not found")

// TraceApplicationService 提供智能体链路追踪的应用层查询能力
type TraceApplicationService interface {
	GetTraceDetail(ctx context.Context, traceID string) (*dtos.TraceDetailDTO, error)
	ListTraces(ctx context.Context, req dtos.TraceListRequest) (*dtos.TraceListDTO, error)
}

// TraceApplicationServiceImpl 是 TraceApplicationService 的默认实现
type TraceApplicationServiceImpl struct {
	repo tracing.SpanRepository
}

// NewTraceApplicationService 构造 TraceApplicationService 实例
func NewTraceApplicationService(repo tracing.SpanRepository) TraceApplicationService {
	return &TraceApplicationServiceImpl{repo: repo}
}

// GetTraceDetail 根据 traceID 拉取完整 span 树
func (s *TraceApplicationServiceImpl) GetTraceDetail(ctx context.Context, traceID string) (*dtos.TraceDetailDTO, error) {
	nodes, err := s.repo.FindByTraceID(ctx, traceID)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrTraceNotFound
	}
	root, err := tracing.BuildSpanTree(nodes)
	if err != nil {
		return nil, err
	}
	anyErr := false
	for _, n := range nodes {
		if n.IsError {
			anyErr = true
			break
		}
	}
	var convID, agentName string
	for _, n := range nodes {
		if n.SpanID == 0 && n.ParentSpanID == -1 {
			if v, ok := n.Tags["conversation_id"].(string); ok {
				convID = v
			}
			if v, ok := n.Tags["agent.name"].(string); ok {
				agentName = v
			}
		}
	}
	return &dtos.TraceDetailDTO{
		TraceID:        traceID,
		ConversationID: convID,
		AgentName:      agentName,
		StartTime:      root.StartTime,
		EndTime:        root.EndTime,
		DurationMs:     root.LatencyMs,
		IsError:        anyErr,
		SpanCount:      int32(len(nodes)),
		Root:           root,
	}, nil
}

// ListTraces 分页查询 trace 摘要
func (s *TraceApplicationServiceImpl) ListTraces(ctx context.Context, req dtos.TraceListRequest) (*dtos.TraceListDTO, error) {
	filter := tracing.TraceFilter{
		ConversationID: req.ConversationID,
		AgentName:      req.AgentName,
		IsError:        req.IsError,
		StartTimeFrom:  req.StartTimeFrom,
		StartTimeTo:    req.StartTimeTo,
	}
	page, pageSize := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	list, total, err := s.repo.ListTraces(ctx, filter, page, pageSize)
	if err != nil {
		return nil, err
	}
	out := make([]*dtos.TraceSummaryDTO, 0, len(list))
	for _, t := range list {
		out = append(out, &dtos.TraceSummaryDTO{
			TraceID:          t.TraceID,
			ConversationID:   t.ConversationID,
			AgentName:        t.AgentName,
			StartTime:        t.StartTime,
			DurationMs:       t.DurationMs,
			SpanCount:        t.SpanCount,
			IsError:          t.IsError,
			UserQueryPreview: t.UserQueryPreview,
		})
	}
	return &dtos.TraceListDTO{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Traces:   out,
	}, nil
}
