package events

const (
	EventTypeTitle            = "title"
	EventTypeMessageEnd       = "message_end"
	EventTypeMessage          = "message"
	EventTypeToolCallStart    = "tool_call_start"
	EventTypeToolCallComplete = "tool_call_complete"
	EventTypeToolCallFail     = "tool_call_fail"
	EventTypeWait             = "wait"
	EventTypeError            = "error"
	EventTypeDone             = "done"

	EventTypePlanCreateSuccess = "plan_create_success"
	EventTypePlanUpdateSuccess = "plan_update_success"
	EventTypePlanUpdateFailed  = "plan_update_failed"
	EventTypePlanCompleted     = "plan_completed"
	EventTypeStepStart         = "step_start"
	EventTypeStepComplete      = "step_complete"
	EventTypeStepFail          = "step_fail"
)

type ToolEventStatus string

const (
	ToolEventStatusCalling   ToolEventStatus = "calling"
	ToolEventStatusCompleted ToolEventStatus = "completed"
	ToolEventStatusFailed    ToolEventStatus = "failed"
)

// PlanEventStatus 规划事件状态
type PlanEventStatus string

const (
	PlanCreated   PlanEventStatus = "created"   // 已创建
	PlanUpdated   PlanEventStatus = "updated"   // 已更新
	PlanFailed    PlanEventStatus = "failed"    // 已完成
	PlanCompleted PlanEventStatus = "completed" // 已完成
)

// StepEventStatus 步骤事件状态
type StepEventStatus string

const (
	StepStarted   StepEventStatus = "started"   // 已开始
	StepCompleted StepEventStatus = "completed" // 已完成
	StepFailed    StepEventStatus = "failed"    // 已失败
)
