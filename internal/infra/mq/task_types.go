package mq

// 评测系统 asynq 任务与队列常量（spec §5）
// 队列命名带 `eval:` 前缀，避免和其他业务（如日志、导出）共享同一 asynq 实例时冲突。
const (
	// TaskTypeRunInstance 单个评测实例执行任务类型
	TaskTypeRunInstance = "eval:run_instance"

	// QueueDefault 常规入队走的队列（首次任务）
	QueueDefault = "eval:default"

	// QueueHigh 高优先级队列（重试、人工重跑等短 backoff 场景）
	QueueHigh = "eval:high"
)
