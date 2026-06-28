package events

import (
	"time"

	"github.com/google/uuid"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/llm"
)

func convert2ToolEvent(toolCall llm.ToolCall, toolName string) ToolEvent {
	ev := ToolEvent{}
	ev.ID = uuid.New().String()
	ev.CreatedAt = time.Now()
	ev.Timestamp = time.Now()
	ev.ToolCallID = toolCall.ID
	ev.ToolName = toolName
	ev.FunctionName = toolCall.Name
	ev.FunctionArgs = toolCall.Arguments
	return ev
}

func OnToolCallStart(toolCall llm.ToolCall, toolName string) AgentEvent {
	ev := convert2ToolEvent(toolCall, toolName)
	ev.Type = EventTypeToolCallStart
	ev.Status = ToolEventStatusCalling
	return &ev
}

func OnToolCallComplete(toolCall llm.ToolCall, toolName string, result *models.ToolCallResult) AgentEvent {
	ev := convert2ToolEvent(toolCall, toolName)
	ev.Type = EventTypeToolCallComplete
	ev.Status = ToolEventStatusCompleted
	ev.FunctionResult = result
	return &ev
}

func OnToolCallFail(toolCall llm.ToolCall, toolName string, result *models.ToolCallResult) AgentEvent {
	ev := convert2ToolEvent(toolCall, toolName)
	ev.Type = EventTypeToolCallFail
	ev.Status = ToolEventStatusFailed
	ev.FunctionResult = result
	return &ev
}
