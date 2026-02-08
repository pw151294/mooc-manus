package events

import (
	"mooc-manus/internal/domains/models"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go"
)

func convert2ToolEvent(toolCall openai.ChatCompletionMessageToolCall, toolName string) ToolEvent {
	toolEvent := ToolEvent{}
	toolEvent.ID = uuid.New().String()
	toolEvent.CreatedAt = time.Now()
	toolEvent.Timestamp = time.Now()
	toolEvent.ToolCallID = toolCall.ID
	toolEvent.ToolName = toolName
	toolEvent.FunctionName = toolCall.Function.Name
	toolEvent.FunctionArgs = toolCall.Function.Arguments

	return toolEvent
}

func OnToolCallStart(toolCall openai.ChatCompletionMessageToolCall, toolName string) AgentEvent {
	toolEvent := convert2ToolEvent(toolCall, toolName)
	toolEvent.Type = EventTypeToolCallStart
	toolEvent.Status = ToolEventStatusCalling
	return &toolEvent
}
func OnToolCallComplete(toolCall openai.ChatCompletionMessageToolCall, toolName string, result *models.ToolCallResult) AgentEvent {
	toolEvent := convert2ToolEvent(toolCall, toolName)
	toolEvent.Type = EventTypeToolCallComplete
	toolEvent.Status = ToolEventStatusCompleted
	toolEvent.FunctionResult = result
	return &toolEvent
}
func OnToolCallFail(toolCall openai.ChatCompletionMessageToolCall, toolName string, result *models.ToolCallResult) AgentEvent {
	toolEvent := convert2ToolEvent(toolCall, toolName)
	toolEvent.Type = EventTypeToolCallFail
	toolEvent.Status = ToolEventStatusFailed
	toolEvent.FunctionResult = result
	return &toolEvent
}
