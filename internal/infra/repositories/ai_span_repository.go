package repositories

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"mooc-manus/internal/domains/models/tracing"
	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"
)

type AiSpanRepositoryImpl struct {
	dbCli *gorm.DB
}

func NewAiSpanRepository() tracing.SpanRepository {
	return &AiSpanRepositoryImpl{dbCli: storage.GetPostgresClient()}
}

func (r *AiSpanRepositoryImpl) BatchInsert(ctx context.Context, spans []*tracing.Span) error {
	if len(spans) == 0 {
		return nil
	}
	pos := make([]models.AiSpanPO, 0, len(spans))
	for _, s := range spans {
		pos = append(pos, spanToPO(s))
	}
	return r.dbCli.WithContext(ctx).CreateInBatches(&pos, 100).Error
}

func (r *AiSpanRepositoryImpl) FindByTraceID(ctx context.Context, traceID string) ([]*tracing.SpanNode, error) {
	var pos []models.AiSpanPO
	err := r.dbCli.WithContext(ctx).
		Where("trace_id = ?", traceID).
		Order("span_id ASC").
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	nodes := make([]*tracing.SpanNode, 0, len(pos))
	for _, po := range pos {
		nodes = append(nodes, poToNode(&po))
	}
	return nodes, nil
}

// ListByConversationID 按 conversation_id 拉取全部 span，用于 TraceAggregator。
// 按 start_time ASC 排序，保证同一 trace 内部 root/子 span 相对顺序可用。
func (r *AiSpanRepositoryImpl) ListByConversationID(ctx context.Context, conversationID string) ([]*tracing.Span, error) {
	var pos []models.AiSpanPO
	if err := r.dbCli.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("start_time ASC").
		Find(&pos).Error; err != nil {
		return nil, err
	}
	out := make([]*tracing.Span, 0, len(pos))
	for i := range pos {
		out = append(out, poToSpan(&pos[i]))
	}
	return out, nil
}

// poToSpan 将 PO 反序列化为 tracing.Span（查询路径专用）。
// tags / logs 是 jsonb 字符串，反序列化失败时降级为空值，不阻塞聚合。
func poToSpan(po *models.AiSpanPO) *tracing.Span {
	var tags map[string]interface{}
	if po.Tags != "" {
		_ = json.Unmarshal([]byte(po.Tags), &tags)
	}
	var logs []tracing.LogEntry
	if po.Logs != "" {
		_ = json.Unmarshal([]byte(po.Logs), &logs)
	}
	return tracing.NewSpanForQuery(&tracing.AiSpanPOFields{
		TraceID:        po.TraceID,
		SpanID:         po.SpanID,
		ParentSpanID:   po.ParentSpanID,
		SpanType:       po.SpanType,
		OperationName:  po.OperationName,
		ConversationID: po.ConversationID,
		AgentName:      po.AgentName,
		StartTime:      po.StartTime,
		EndTime:        po.EndTime,
		LatencyMs:      po.LatencyMs,
		IsError:        po.IsError,
		Tags:           tags,
		Logs:           logs,
	})
}

func (r *AiSpanRepositoryImpl) ListTraces(ctx context.Context, filter tracing.TraceFilter, page, pageSize int) ([]*tracing.TraceSummary, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	// 从 root 行（parent_span_id=-1）出发做分页
	q := r.dbCli.WithContext(ctx).Model(&models.AiSpanPO{}).Where("parent_span_id = ?", -1)
	if filter.ConversationID != "" {
		q = q.Where("conversation_id = ?", filter.ConversationID)
	}
	if filter.AgentName != "" {
		q = q.Where("agent_name = ?", filter.AgentName)
	}
	if filter.StartTimeFrom > 0 {
		q = q.Where("start_time >= ?", filter.StartTimeFrom)
	}
	if filter.StartTimeTo > 0 {
		q = q.Where("start_time <= ?", filter.StartTimeTo)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var roots []models.AiSpanPO
	if err := q.Order("start_time DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&roots).Error; err != nil {
		return nil, 0, err
	}
	// 对每条 root 聚合其 trace 全体行的 span_count + is_error
	ids := make([]string, 0, len(roots))
	for _, root := range roots {
		ids = append(ids, root.TraceID)
	}
	type aggRow struct {
		TraceID   string
		SpanCount int32
		AnyError  bool
	}
	aggs := []aggRow{}
	if len(ids) > 0 {
		if err := r.dbCli.WithContext(ctx).Model(&models.AiSpanPO{}).
			Select("trace_id, COUNT(*) AS span_count, BOOL_OR(is_error) AS any_error").
			Where("trace_id IN ?", ids).
			Group("trace_id").
			Scan(&aggs).Error; err != nil {
			return nil, 0, err
		}
	}
	aggMap := make(map[string]aggRow, len(aggs))
	for _, a := range aggs {
		aggMap[a.TraceID] = a
	}
	// is_error 过滤（本期在应用层做，避免复杂子查询）
	summaries := make([]*tracing.TraceSummary, 0, len(roots))
	for _, po := range roots {
		agg := aggMap[po.TraceID]
		if filter.IsError != nil && *filter.IsError != agg.AnyError {
			continue
		}
		summaries = append(summaries, &tracing.TraceSummary{
			TraceID:          po.TraceID,
			ConversationID:   po.ConversationID,
			AgentName:        po.AgentName,
			StartTime:        po.StartTime,
			DurationMs:       po.LatencyMs,
			SpanCount:        agg.SpanCount,
			IsError:          agg.AnyError,
			UserQueryPreview: extractUserQueryPreview(po.Tags),
		})
	}
	return summaries, total, nil
}

func spanToPO(s *tracing.Span) models.AiSpanPO {
	tagsBytes, _ := json.Marshal(s.TagsSnapshot())
	logsBytes, _ := json.Marshal(s.LogsSnapshot())
	return models.AiSpanPO{
		TraceID:        s.TraceID,
		SpanID:         s.SpanID,
		ParentSpanID:   s.ParentSpanID,
		SpanType:       string(s.SpanType),
		OperationName:  s.OperationName,
		ConversationID: s.ConversationID,
		AgentName:      s.AgentName,
		StartTime:      s.StartTime,
		EndTime:        s.EndTime,
		LatencyMs:      s.LatencyMs,
		IsError:        s.IsError,
		Tags:           string(tagsBytes),
		Logs:           string(logsBytes),
	}
}

func poToNode(po *models.AiSpanPO) *tracing.SpanNode {
	var tags map[string]interface{}
	_ = json.Unmarshal([]byte(nz(po.Tags)), &tags)
	var logs []tracing.LogEntry
	_ = json.Unmarshal([]byte(nz(po.Logs)), &logs)
	if tags == nil {
		tags = map[string]interface{}{}
	}
	return &tracing.SpanNode{
		SpanID:        po.SpanID,
		ParentSpanID:  po.ParentSpanID,
		SpanType:      po.SpanType,
		OperationName: po.OperationName,
		StartTime:     po.StartTime,
		EndTime:       po.EndTime,
		LatencyMs:     po.LatencyMs,
		IsError:       po.IsError,
		Tags:          tags,
		Logs:          logs,
	}
}

func nz(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

func extractUserQueryPreview(tagsJSON string) string {
	if tagsJSON == "" {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(tagsJSON), &m); err != nil {
		return ""
	}
	if v, ok := m["user.query"].(string); ok {
		if len(v) > 80 {
			return v[:80]
		}
		return v
	}
	return ""
}
