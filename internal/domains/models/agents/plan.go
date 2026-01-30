package agents

type ExecutionStatus string

const (
	Pending   ExecutionStatus = "PENDING"   // 空闲或者等待中
	Running   ExecutionStatus = "RUNNING"   // 执行中
	Completed ExecutionStatus = "COMPLETED" // 执行完成
	Failed    ExecutionStatus = "FAILED"    // 执行失败
)

type Step struct {
	ID          string          `json:"id"`               // 子任务ID
	Description string          `json:"description"`      // 步骤的描述信息
	Status      ExecutionStatus `json:"status"`           // 子任务的执行状态
	Result      string          `json:"result,omitempty"` // 执行结果
	Error       string          `json:"error,omitempty"`  // 错误信息
	Success     bool            `json:"success"`          // 子任务是否执行成功
	Attachments []string        `json:"attachments"`      // 附件列表信息
}

type Plan struct {
	ID       string          `json:"id"`              // 计划ID
	Title    string          `json:"title"`           // 任务标题
	Goal     string          `json:"goal"`            // 任务目标
	Language string          `json:"language"`        // 工作语言
	Steps    []Step          `json:"steps"`           // 步骤/子任务列表
	Message  string          `json:"message"`         // 用户传递的消息
	Status   ExecutionStatus `json:"status"`          // 规划的状态
	Error    string          `json:"error,omitempty"` // 错误信息
}
