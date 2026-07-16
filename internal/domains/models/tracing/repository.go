package tracing

import "context"

// TraceFilter 用于 ListTraces 的过滤条件（可选字段留零值即忽略）
type TraceFilter struct {
	ConversationID string
	AgentName      string
	IsError        *bool
	StartTimeFrom  int64
	StartTimeTo    int64
}

// TraceSummary 是 trace 列表项的摘要，仅取根 span 的关键信息
type TraceSummary struct {
	TraceID          string
	ConversationID   string
	AgentName        string
	StartTime        int64
	DurationMs       int32
	SpanCount        int32
	IsError          bool
	UserQueryPreview string
}

// SpanRepository 抽象 span 的持久化能力。真实实现在 infra 层，测试可用 fake。
// SpanNode 完整定义在 tree.go。
type SpanRepository interface {
	BatchInsert(ctx context.Context, spans []*Span) error
	FindByTraceID(ctx context.Context, traceID string) ([]*SpanNode, error)
	ListTraces(ctx context.Context, filter TraceFilter, page, pageSize int) ([]*TraceSummary, int64, error)
	ListByConversationID(ctx context.Context, conversationID string) ([]*Span, error)
}
