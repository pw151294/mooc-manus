package agents

import "time"

// InterruptDecisionKind 用户对高危工具的决策分类
type InterruptDecisionKind string

const (
	DecisionApprove InterruptDecisionKind = "approve"
	DecisionReject  InterruptDecisionKind = "reject"
	DecisionCancel  InterruptDecisionKind = "cancel"  // Stop 路径注入
	DecisionTimeout InterruptDecisionKind = "timeout" // 超时兜底注入
)

// InterruptDecision 用户对工具调用中断的决策结果
type InterruptDecision struct {
	Kind     InterruptDecisionKind // 决策类型
	Feedback string                // 仅 Reject 时可能非空，用于向 Agent 解释拒绝原因
}

// InterruptSnapshot 工具调用中断的快照信息
type InterruptSnapshot struct {
	ToolCallID   string    // 工具调用唯一标识
	FunctionName string    // 函数名称
	FunctionArgs string    // 函数参数（JSON 格式）
	RiskLevel    string    // 风险等级
	RiskReason   string    // 风险原因说明
	RegisteredAt time.Time // 注册时间
}

// PendingSink 是 Agent 层向 app service 反查的窄接口。
// 只暴露 Register 与 WaitTimeout；Resolve/Cancel 由 app service 内部完成。
type PendingSink interface {
	// RegisterInterrupt 注册一个工具调用中断，返回一个用于接收决策结果的 channel
	// messageId: 消息标识，用于关联中断记录
	// snap: 中断快照信息
	// 返回: 决策结果 channel 和可能的错误
	RegisterInterrupt(messageId string, snap InterruptSnapshot) (<-chan InterruptDecision, error)

	// WaitTimeout 返回等待用户决策的超时时间
	WaitTimeout() time.Duration
}
