package events

const (
	EventTypeTitle   = "title"
	EventTypeStep    = "step"
	EventTypeMessage = "message"
	EventTypeTool    = "tools"
	EventTypeWait    = "wait"
	EventTypeError   = "error"
	EventTypeDone    = "done"
)

type ToolEventStatus string

const (
	ToolEventStatusCalling   ToolEventStatus = "calling"
	ToolEventStatusCompleted ToolEventStatus = "completed"
	ToolEventStatusFailed    ToolEventStatus = "failed"
)
