package agents

import (
	"mooc-manus/internal/domains/models/file"
	"mooc-manus/internal/domains/models/interrupt"
)

type SkillRef struct {
	SkillID   string
	Version   string
	SkillName string // Skill 名称，用于 executeSkill 工具按名称查找
}

type ChatRequest struct {
	Streaming      bool
	SystemPrompt   string
	ConversationId string
	MessageId      string // SSE 流消息 ID，由 application 层从 sse.StartChat 注入
	Query          string
	AppConfigId    string
	FunctionIds    []string
	ProviderIds    []string
	SkillRefs      []SkillRef
	Files          []file.File
	PlanMode       bool                  // 规划模式开关：开启后框架自动注入 PlanMode 提示词并实现记忆持久化
	PendingSink    interrupt.PendingSink // HITL 审批管理器（由 application 层注入）
}

type AgentPlanCreateRequest struct {
	ConversationId string
	MessageId      string // SSE 流消息 ID，由 application 层注入，透传给 ChatRequest
	Query          string
	AppConfigId    string
	Files          []file.File
}

type AgentPlanUpdateRequest struct {
	ConversationId string
	MessageId      string // SSE 流消息 ID，由 application 层注入，透传给 ChatRequest
	AppConfigId    string
	PlanId         string
	StepId         string
}

type AgentExecuteRequest struct {
	ConversationId string
	AppConfigId    string
	PlanId         string
	StepId         string
	FunctionIds    []string
	ProviderIds    []string
	Attachments    []string
}

func ConvertPlanCreateRequest2ChatRequest(planCreateRequest AgentPlanCreateRequest) ChatRequest {
	request := ChatRequest{}
	request.ConversationId = planCreateRequest.ConversationId
	request.MessageId = planCreateRequest.MessageId
	request.Query = planCreateRequest.Query
	request.AppConfigId = planCreateRequest.AppConfigId
	request.Files = planCreateRequest.Files
	return request
}

func ConvertPlanUpdateRequest2ChatRequest(planUpdateRequest AgentPlanUpdateRequest) ChatRequest {
	request := ChatRequest{}
	request.ConversationId = planUpdateRequest.ConversationId
	request.MessageId = planUpdateRequest.MessageId
	request.AppConfigId = planUpdateRequest.AppConfigId
	return request
}

func ConvertAgentExecuteRequest2ChatRequest(executeRequest AgentExecuteRequest) ChatRequest {
	request := ChatRequest{}
	request.ConversationId = executeRequest.ConversationId
	request.AppConfigId = executeRequest.AppConfigId
	request.FunctionIds = executeRequest.FunctionIds
	request.ProviderIds = executeRequest.ProviderIds

	return request
}
