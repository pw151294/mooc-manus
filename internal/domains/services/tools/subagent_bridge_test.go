package tools

import (
	"testing"
	"time"

	"mooc-manus/internal/domains/models/events"
)

func TestSubagentEventBridge_ForwardToolEvent(t *testing.T) {
	ch := make(chan events.AgentEvent, 1)
	bridge := NewSubagentEventBridge(ch, "sub-001", "翻译任务", "英译中")

	toolEvent := &events.ToolEvent{
		BaseEvent: events.BaseEvent{
			ID:   "evt-1",
			Type: events.EventTypeToolCallComplete,
		},
		Timestamp:    time.Now(),
		ToolCallID:   "tc-1",
		ToolName:     "translator",
		FunctionName: "translate",
		FunctionArgs: `{"text":"hello"}`,
		Status:       events.ToolEventStatusCompleted,
	}

	bridge.ForwardEvent(toolEvent)

	received := <-ch
	got, ok := received.(*events.ToolEvent)
	if !ok {
		t.Fatalf("期望收到 *events.ToolEvent，实际: %T", received)
	}
	if got.Metadata == nil {
		t.Fatal("ToolEvent.Metadata 不应为 nil")
	}
	if got.Metadata["is_subagent"] != true {
		t.Errorf("is_subagent 期望 true，实际: %v", got.Metadata["is_subagent"])
	}
}

func TestSubagentEventBridge_ForwardNonToolEvent(t *testing.T) {
	ch := make(chan events.AgentEvent, 1)
	bridge := NewSubagentEventBridge(ch, "sub-002", "搜索任务", "搜索上下文")

	errEvent := &events.ErrorEvent{
		BaseEvent: events.BaseEvent{
			ID:   "evt-2",
			Type: events.EventTypeError,
		},
		Timestamp: time.Now(),
		Error:     "something went wrong",
	}

	bridge.ForwardEvent(errEvent)

	received := <-ch
	got, ok := received.(*events.ErrorEvent)
	if !ok {
		t.Fatalf("期望收到 *events.ErrorEvent，实际: %T", received)
	}
	if got.Error != "something went wrong" {
		t.Errorf("ErrorEvent.Error 期望 'something went wrong'，实际: %s", got.Error)
	}
}

func TestSubagentEventBridge_MetadataFields(t *testing.T) {
	ch := make(chan events.AgentEvent, 1)
	bridge := NewSubagentEventBridge(ch, "sub-003", "代码生成", "生成Go代码")

	toolEvent := &events.ToolEvent{
		BaseEvent: events.BaseEvent{
			ID:   "evt-3",
			Type: events.EventTypeToolCallStart,
		},
		Timestamp:    time.Now(),
		ToolCallID:   "tc-3",
		ToolName:     "codegen",
		FunctionName: "generate",
		Status:       events.ToolEventStatusCalling,
	}

	bridge.ForwardEvent(toolEvent)

	received := <-ch
	got := received.(*events.ToolEvent)

	tests := []struct {
		key  string
		want interface{}
	}{
		{"subagent_id", "sub-003"},
		{"is_subagent", true},
		{"subagent_task", "代码生成"},
		{"subagent_context", "生成Go代码"},
	}
	for _, tc := range tests {
		val, exists := got.Metadata[tc.key]
		if !exists {
			t.Errorf("Metadata 缺少 key: %s", tc.key)
			continue
		}
		if val != tc.want {
			t.Errorf("Metadata[%s] 期望 %v，实际 %v", tc.key, tc.want, val)
		}
	}
}
