package dtos

// ResumeClientRequest HITL 用户决策回投请求
type ResumeClientRequest struct {
	MessageId  string `json:"messageId"  binding:"required"`
	ToolCallId string `json:"toolCallId" binding:"required"`
	Decision   string `json:"decision"   binding:"required,oneof=approve reject"`
	Feedback   string `json:"feedback,omitempty"` // 仅 decision=reject 时可选
}

// ResumeResult HITL Resume 返回
// Status: "accepted"（200） | "already_decided"（409） | "not_found"（404）
type ResumeResult struct {
	Status string `json:"status"`
}
