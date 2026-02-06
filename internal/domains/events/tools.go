package events

import (
	"mooc-manus/internal/domains/models"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go"
)

type ToolEvent struct {
	BaseEvent
	Timestamp      time.Time              `json:"timestamp"`
	ToolCallID     string                 `json:"tool_call_id"`    // 工具调用ID
	ToolName       string                 `json:"tool_name"`       // 工具集(provider)名称
	FunctionName   string                 `json:"function_name"`   // LLM调用的函数名称
	FunctionArgs   string                 `json:"function_args"`   // LLM生成的工具调用参数
	FunctionResult *models.ToolCallResult `json:"function_result"` // 工具调用结果
	Status         ToolEventStatus        `json:"status"`          // 工具调用状态
	// todo ToolContent    ToolContent            `json:"tool_content"`    // 工具扩展内容
}

type BrowserToolContent struct {
	Screenshot string `json:"screenshot"` // 浏览器快照截图
}

type McpToolContent struct {
	Result interface{} `json:"result"` // 任意类型的结果
}

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
