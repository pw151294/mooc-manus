package dtos

import "mooc-manus/internal/domains/models/tracing"

type TraceDetailDTO struct {
	TraceID        string            `json:"trace_id"`
	ConversationID string            `json:"conversation_id"`
	AgentName      string            `json:"agent_name"`
	StartTime      int64             `json:"start_time"`
	EndTime        int64             `json:"end_time"`
	DurationMs     int32             `json:"duration_ms"`
	IsError        bool              `json:"is_error"`
	SpanCount      int32             `json:"span_count"`
	Root           *tracing.SpanNode `json:"root"`
}

type TraceSummaryDTO struct {
	TraceID          string `json:"trace_id"`
	ConversationID   string `json:"conversation_id"`
	AgentName        string `json:"agent_name"`
	StartTime        int64  `json:"start_time"`
	DurationMs       int32  `json:"duration_ms"`
	SpanCount        int32  `json:"span_count"`
	IsError          bool   `json:"is_error"`
	UserQueryPreview string `json:"user_query_preview"`
}

type TraceListDTO struct {
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Traces   []*TraceSummaryDTO `json:"traces"`
}

type TraceListRequest struct {
	ConversationID string `form:"conversation_id"`
	AgentName      string `form:"agent_name"`
	IsError        *bool  `form:"is_error"`
	StartTimeFrom  int64  `form:"start_time_from"`
	StartTimeTo    int64  `form:"start_time_to"`
	Page           int    `form:"page"`
	PageSize       int    `form:"page_size"`
}
