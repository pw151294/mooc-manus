// Package tracing 提供智能体链路追踪的 Span 值对象与常量定义。
package tracing

import (
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

type SpanType string

const (
	SpanTypeAgentRoot    SpanType = "AGENT_ROOT"
	SpanTypeAgentRound   SpanType = "AGENT_ROUND"
	SpanTypeLLMCall      SpanType = "LLM_CALL"
	SpanTypeToolBatch    SpanType = "TOOL_BATCH"
	SpanTypeToolCall     SpanType = "TOOL_CALL"
	SpanTypeSubagentCall SpanType = "SUBAGENT_CALL"
)

const (
	MaxUserQueryBytes    = 1024
	MaxToolArgsBytes     = 2048
	MaxToolResultPreview = 512

	MaskedValue = "***"
)

type LogEntry struct {
	Ts    int64                  `json:"ts"`
	Level string                 `json:"level"`
	Msg   string                 `json:"msg"`
	Extra map[string]interface{} `json:"extra,omitempty"`
}

type Span struct {
	TraceID        string
	SpanID         int32
	ParentSpanID   int32
	SpanType       SpanType
	OperationName  string
	ConversationID string
	AgentName      string
	StartTime      int64
	EndTime        int64
	LatencyMs      int32
	IsError        bool

	tags  map[string]interface{}
	logs  []LogEntry
	mu    sync.Mutex
	ended atomic.Bool

	// commitFn 用于把 span 提交到 Tracer 队列；单测里可注入
	commitFn func(*Span)
}

var (
	sensitiveRegexOnce sync.Once
	sensitiveRegex     *regexp.Regexp
)

func sensitiveKeyRegex() *regexp.Regexp {
	sensitiveRegexOnce.Do(func() {
		sensitiveRegex = regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret|authorization)`)
	})
	return sensitiveRegex
}

func (s *Span) SetTag(key string, val interface{}) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tags == nil {
		s.tags = make(map[string]interface{})
	}
	if sensitiveKeyRegex().MatchString(key) {
		s.tags[key] = MaskedValue
		return
	}
	if str, ok := val.(string); ok {
		limit := maxLenForKey(key)
		if limit > 0 && len(str) > limit {
			str = str[:limit]
		}
		s.tags[key] = str
		return
	}
	s.tags[key] = val
}

func maxLenForKey(key string) int {
	switch key {
	case "user.query":
		return MaxUserQueryBytes
	case "tool.arguments":
		return MaxToolArgsBytes
	case "tool.result_preview":
		return MaxToolResultPreview
	}
	return 0
}

// SetAgentName 独立列写入（避开 tags 走独立列）
func (s *Span) SetAgentName(name string) {
	if s == nil {
		return
	}
	s.AgentName = name
}

// SetConversationID 独立列写入
func (s *Span) SetConversationID(id string) {
	if s == nil {
		return
	}
	s.ConversationID = id
}

func (s *Span) AddLog(level, msg string, extra map[string]interface{}) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, LogEntry{
		Ts:    time.Now().UnixNano(),
		Level: level,
		Msg:   msg,
		Extra: extra,
	})
}

func (s *Span) SetError(err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	s.IsError = true
	s.logs = append(s.logs, LogEntry{
		Ts:    time.Now().UnixNano(),
		Level: "ERROR",
		Msg:   err.Error(),
	})
	s.mu.Unlock()
}

func (s *Span) End() {
	if s == nil {
		return
	}
	if !s.ended.CompareAndSwap(false, true) {
		return
	}
	now := time.Now().UnixNano()
	s.EndTime = now
	if s.StartTime > 0 {
		s.LatencyMs = int32((now - s.StartTime) / int64(time.Millisecond))
	}
	if s.commitFn != nil {
		s.commitFn(s)
	}
}

// TagsSnapshot 返回 tags 的浅拷贝，供 tracer / repository 序列化使用
func (s *Span) TagsSnapshot() map[string]interface{} {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]interface{}, len(s.tags))
	for k, v := range s.tags {
		out[k] = v
	}
	return out
}

// LogsSnapshot 返回 logs 拷贝
func (s *Span) LogsSnapshot() []LogEntry {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LogEntry, len(s.logs))
	copy(out, s.logs)
	return out
}
