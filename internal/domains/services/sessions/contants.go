package sessions

type SessionStatus string

const (
	SessionStatusPending   SessionStatus = "pending"   // 等待任务
	SessionStatusRunning   SessionStatus = "running"   // 运行中
	SessionStatusWaiting   SessionStatus = "waiting"   // 等待人类响应
	SessionStatusCompleted SessionStatus = "completed" // 已完成
)
