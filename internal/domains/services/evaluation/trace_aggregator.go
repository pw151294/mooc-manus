package evaluation

import (
	"context"

	"go.uber.org/zap"

	"mooc-manus/internal/domains/models/tracing"
	"mooc-manus/pkg/logger"
)

// Metrics 是评测系统按 conversation 聚合出的关键指标，
// 由 InstanceExecutor 落库到 RunInstance 供打分策略使用。
type Metrics struct {
	AgentLatencyMs   int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	TraceID          string
	// Degraded 表示未找到 AGENT_ROOT span，指标缺 AgentLatencyMs；
	// 保留 token 汇总，允许上层继续跑分（对齐 spec §11 风险 5：降级不报错）。
	Degraded bool
}

// LLM usage tag key（M3 阶段落地：span.tags 用 llm.io.* 前缀，
// 脱敏正则打的是 llm.usage.* 前缀，聚合读的是 io.* 三键）
const (
	tagPromptUnits     = "llm.io.prompt_units"
	tagCompletionUnits = "llm.io.completion_units"
	tagTotalUnits      = "llm.io.total_units"
)

// TraceAggregator 从 ai_span 表按 conversationID 聚合评测指标。
type TraceAggregator struct {
	spanRepo tracing.SpanRepository
}

// NewTraceAggregator 构造 aggregator。
func NewTraceAggregator(spanRepo tracing.SpanRepository) *TraceAggregator {
	return &TraceAggregator{spanRepo: spanRepo}
}

// Aggregate 按 conversationID 聚合 AgentLatencyMs / token 三项指标：
//   - AgentLatencyMs：AGENT_ROOT span 的最大 LatencyMs（多个 root 时取最大，并告警）
//   - PromptTokens / CompletionTokens / TotalTokens：LLM_CALL span 的 llm.io.*_units tag 之和
//   - TraceID：任一 AGENT_ROOT 的 trace_id；缺 root 时回退到首条 span 的 trace_id
//   - Degraded：无 AGENT_ROOT 时置 true（spec §3.4 + §11 风险 5）
func (a *TraceAggregator) Aggregate(ctx context.Context, conversationID string) (*Metrics, error) {
	spans, err := a.spanRepo.ListByConversationID(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	m := &Metrics{}
	rootCount := 0
	for _, s := range spans {
		if s == nil {
			continue
		}
		switch s.SpanType {
		case tracing.SpanTypeAgentRoot:
			rootCount++
			if int64(s.LatencyMs) > m.AgentLatencyMs {
				m.AgentLatencyMs = int64(s.LatencyMs)
			}
			if m.TraceID == "" {
				m.TraceID = s.TraceID
			}
		case tracing.SpanTypeLLMCall:
			tags := s.TagsSnapshot()
			m.PromptTokens += tagAsInt64(tags, tagPromptUnits)
			m.CompletionTokens += tagAsInt64(tags, tagCompletionUnits)
			m.TotalTokens += tagAsInt64(tags, tagTotalUnits)
		}
	}

	if rootCount == 0 {
		m.Degraded = true
		logger.Warn("聚合时未找到 AGENT_ROOT span，指标降级",
			zap.String("conversation_id", conversationID),
			zap.Int("span_count", len(spans)))
	}
	if rootCount > 1 {
		logger.Warn("同一 conversation 出现多个 AGENT_ROOT，取最大 LatencyMs",
			zap.String("conversation_id", conversationID),
			zap.Int("root_count", rootCount))
	}
	// 降级路径下若 TraceID 仍为空，回退挑第一条 span 的 trace_id 供排查。
	if m.TraceID == "" && len(spans) > 0 && spans[0] != nil {
		m.TraceID = spans[0].TraceID
	}
	return m, nil
}

// tagAsInt64 从 tags map 提取 int64；兼容 json.Unmarshal 默认 float64 与手工 int/int64。
// 未命中或类型不匹配返回 0（LLM_CALL 缺 usage tag 视为该轮无 token）。
func tagAsInt64(tags map[string]interface{}, key string) int64 {
	if tags == nil {
		return 0
	}
	v, ok := tags[key]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	}
	return 0
}
