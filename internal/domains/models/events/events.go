package events

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/file"
	"time"
)

type MessageEvent struct {
	BaseEvent
	Timestamp   time.Time   `json:"timestamp"`
	Role        string      `json:"role"`        // 消息角色: "user" 或 "assistant"
	Message     string      `json:"message"`     // 消息本身
	Attachments []file.File `json:"attachments"` // 附件列表信息
}

// PlanEvent 规划事件类型
type PlanEvent struct {
	BaseEvent
	Plan   agents.Plan     `json:"plan"`   // 规划信息
	Status PlanEventStatus `json:"status"` // 规划事件状态
}

// StepEvent 子任务/步骤事件
type StepEvent struct {
	BaseEvent
	Step   agents.Step     `json:"step"`   // 步骤信息
	Status StepEventStatus `json:"status"` // 步骤执行的状态
}

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

type WaitEvent struct {
	BaseEvent
}

type ErrorEvent struct {
	BaseEvent
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error"` // 错误信息
}

type DoneEvent struct {
	BaseEvent
	Timestamp time.Time `json:"timestamp"`
}

type TitleEvent struct {
	BaseEvent
	Timestamp time.Time `json:"timestamp"`
	Title     string    `json:"title"` // 标题
}
