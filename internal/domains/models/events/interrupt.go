package events

import (
	"time"

	"github.com/google/uuid"

	"mooc-manus/internal/domains/models/llm"
)

// ToolInterruptEvent 抛出于工具调用被判定为高危、需要用户决策时。
// 与 ToolEvent 平级独立结构体：字段差异较大（多 RiskLevel / RiskReason，无 FunctionResult）
type ToolInterruptEvent struct {
	BaseEvent
	Timestamp    time.Time       `json:"timestamp"`
	ToolCallID   string          `json:"tool_call_id"`
	ToolName     string          `json:"tool_name"`     // provider 名，如 "native"
	FunctionName string          `json:"function_name"` // 如 "bashExec"
	FunctionArgs string          `json:"function_args"` // 原始 arguments JSON
	RiskLevel    string          `json:"risk_level"`    // 当前恒为 "dangerous"
	RiskReason   string          `json:"risk_reason"`   // LLM 给出的风险说明
	Status       ToolEventStatus `json:"status"`        // 恒为 "interrupted"
}

func OnToolCallInterrupt(toolCall llm.ToolCall, toolName, riskLevel, riskReason string) AgentEvent {
	ev := ToolInterruptEvent{
		Timestamp:    time.Now(),
		ToolCallID:   toolCall.ID,
		ToolName:     toolName,
		FunctionName: toolCall.Name,
		FunctionArgs: toolCall.Arguments,
		RiskLevel:    riskLevel,
		RiskReason:   riskReason,
		Status:       ToolEventStatusInterrupted,
	}
	ev.ID = uuid.New().String()
	ev.CreatedAt = time.Now()
	ev.Type = EventTypeToolCallInterrupt
	return &ev
}
