package models

import "time"

type AiSpanPO struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TraceID        string    `gorm:"column:trace_id;type:varchar(64);not null;index:idx_ai_span_trace" json:"traceId"`
	SpanID         int32     `gorm:"column:span_id;not null" json:"spanId"`
	ParentSpanID   int32     `gorm:"column:parent_span_id;not null" json:"parentSpanId"`
	SpanType       string    `gorm:"column:span_type;type:varchar(32);not null" json:"spanType"`
	OperationName  string    `gorm:"column:operation_name;type:varchar(128);not null;default:''" json:"operationName"`
	ConversationID string    `gorm:"column:conversation_id;type:varchar(64);not null;default:''" json:"conversationId"`
	AgentName      string    `gorm:"column:agent_name;type:varchar(64);not null;default:''" json:"agentName"`
	StartTime      int64     `gorm:"column:start_time;not null" json:"startTime"`
	EndTime        int64     `gorm:"column:end_time;not null;default:0" json:"endTime"`
	LatencyMs      int32     `gorm:"column:latency_ms;not null;default:0" json:"latencyMs"`
	IsError        bool      `gorm:"column:is_error;not null;default:false" json:"isError"`
	Tags           string    `gorm:"column:tags;type:jsonb" json:"tags"`
	Logs           string    `gorm:"column:logs;type:jsonb" json:"logs"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
}

func (AiSpanPO) TableName() string { return "ai_span" }
