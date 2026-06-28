package events

import (
	"testing"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/llm"
)

func TestOnToolCallStart_FieldMapping(t *testing.T) {
	tc := llm.ToolCall{ID: "tc-1", Name: "search", Arguments: `{"q":"go"}`}
	ev := OnToolCallStart(tc, "web_provider").(*ToolEvent)

	if ev.ToolCallID != "tc-1" {
		t.Fatalf("ToolCallID mismatch: %s", ev.ToolCallID)
	}
	if ev.FunctionName != "search" {
		t.Fatalf("FunctionName mismatch: %s", ev.FunctionName)
	}
	if ev.FunctionArgs != `{"q":"go"}` {
		t.Fatalf("FunctionArgs mismatch: %s", ev.FunctionArgs)
	}
	if ev.ToolName != "web_provider" {
		t.Fatalf("ToolName mismatch: %s", ev.ToolName)
	}
	if ev.Status != ToolEventStatusCalling {
		t.Fatalf("Status mismatch: %s", ev.Status)
	}
}

func TestOnToolCallComplete_CarriesResult(t *testing.T) {
	tc := llm.ToolCall{ID: "tc-1", Name: "search", Arguments: "{}"}
	result := &models.ToolCallResult{Success: true, Message: "ok"}

	ev := OnToolCallComplete(tc, "web_provider", result).(*ToolEvent)
	if ev.FunctionResult == nil || !ev.FunctionResult.Success {
		t.Fatalf("result not propagated: %+v", ev.FunctionResult)
	}
	if ev.Status != ToolEventStatusCompleted {
		t.Fatalf("status mismatch: %s", ev.Status)
	}
}

func TestOnToolCallFail_StatusFailed(t *testing.T) {
	tc := llm.ToolCall{ID: "tc-1", Name: "search", Arguments: "{}"}
	result := &models.ToolCallResult{Success: false, Message: "boom"}
	ev := OnToolCallFail(tc, "web_provider", result).(*ToolEvent)
	if ev.Status != ToolEventStatusFailed {
		t.Fatalf("status mismatch: %s", ev.Status)
	}
}
