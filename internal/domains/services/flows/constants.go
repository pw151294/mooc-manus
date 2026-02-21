package flows

type FlowStatus string

const (
	FlowStatusIdle        FlowStatus = "idle"
	FlowStatusPlanning    FlowStatus = "planning"
	FlowStatusExecuting   FlowStatus = "executing"
	FlowStatusUpdating    FlowStatus = "updating"
	FlowStatusSummarizing FlowStatus = "summarizing"
	FlowStatusCompleted   FlowStatus = "completed"
)

type SessionStatus string

const (
	SessionStatusPending   SessionStatus = "pending"   // 等待任务
	SessionStatusRunning   SessionStatus = "running"   // 运行中
	SessionStatusWaiting   SessionStatus = "waiting"   // 等待人类响应
	SessionStatusCompleted SessionStatus = "completed" // 已完成
)

const flowStatusTransitionLogPattern = "flow status change from %s to %s"
