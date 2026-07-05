package dtos

import (
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/file"
)

type SkillRef struct {
	SkillID   string `json:"skillId"`
	Version   string `json:"version"`
	SkillName string `json:"skillName"` // Skill 名称，用于 executeSkill 工具按名称查找
}

type ChatClientRequest struct {
	Streaming      bool          `json:"streaming"`
	ApiKey         string        `json:"apiKey"`
	SystemPrompt   string        `json:"systemPrompt"`
	Query          string        `json:"query"`
	ConversationId string        `json:"conversationId"`
	AppConfigId    string        `json:"appConfigId"`
	FunctionIds    []string      `json:"functionIds"`
	ProviderIds    []string      `json:"providerIds"`
	SkillRefs      []SkillRef    `json:"skillRefs"`
	Files          []interface{} `json:"file"`
	PlanMode       bool          `json:"planMode"`
}

type AgentPlanCreateClientRequest struct {
	ApiKey         string        `json:"apiKey"`
	ConversationId string        `json:"conversationId"`
	Query          string        `json:"query"`
	AppConfigId    string        `json:"appConfigId"`
	Files          []interface{} `json:"file"`
}

type AgentPlanUpdateClientRequest struct {
	ApiKey         string `json:"apiKey"`
	ConversationId string `json:"conversationId"`
	AppConfigId    string `json:"appConfigId"`
	PlanId         string `json:"planId"`
	StepId         string `json:"stepId"`
}

// StopMessageClientRequest 终止单条流式消息的请求
type StopMessageClientRequest struct {
	MessageId string `json:"messageId" binding:"required"`
}

// StopConversationClientRequest 终止整个会话的请求
type StopConversationClientRequest struct {
	ConversationId string `json:"conversationId" binding:"required"`
}

// StopMessageResult StopMessage 接口响应
// Cleaned 明细记录每一类资源的回收结果，方便前端展示与排障
type StopMessageResult struct {
	MessageId string                 `json:"messageId"`
	Cleaned   StopMessageCleanDetail `json:"cleaned"`
}

// StopMessageCleanDetail 单条消息资源回收明细
// sse=true 表示 CloseChat 前该 messageId 仍是活跃连接（本次真正切断）
// skill / nativeWorkspace 是"清理动作是否成功"，任一失败仅日志告警不影响 200 返回
type StopMessageCleanDetail struct {
	SSE             bool `json:"sse"`
	Skill           bool `json:"skill"`
	NativeWorkspace bool `json:"nativeWorkspace"`
}

// StopConversationResult StopConversation 接口响应
// Messages 列出本次被清理的活跃 messageId（顺序不保证）
type StopConversationResult struct {
	ConversationId string                      `json:"conversationId"`
	Cleaned        StopConversationCleanDetail `json:"cleaned"`
}

type StopConversationCleanDetail struct {
	Memory   bool     `json:"memory"`
	Messages []string `json:"messages"`
}

func convertInterfaces2Files(datas []interface{}) []file.File {
	files := make([]file.File, 0, len(datas))
	for _, data := range datas {
		f, ok := data.(file.File)
		if ok {
			files = append(files, f)
		}
	}
	return files
}

func ConvertChatClientRequest2Request(clientRequest ChatClientRequest) agents.ChatRequest {
	request := agents.ChatRequest{}
	request.Streaming = clientRequest.Streaming
	request.ConversationId = clientRequest.ConversationId
	request.SystemPrompt = clientRequest.SystemPrompt
	request.Query = clientRequest.Query
	request.AppConfigId = clientRequest.AppConfigId
	request.FunctionIds = clientRequest.FunctionIds
	request.ProviderIds = clientRequest.ProviderIds
	request.Files = convertInterfaces2Files(clientRequest.Files)
	request.PlanMode = clientRequest.PlanMode
	// 转换 SkillRefs
	skillRefs := make([]agents.SkillRef, 0, len(clientRequest.SkillRefs))
	for _, ref := range clientRequest.SkillRefs {
		skillRefs = append(skillRefs, agents.SkillRef{
			SkillID:   ref.SkillID,
			Version:   ref.Version,
			SkillName: ref.SkillName,
		})
	}
	request.SkillRefs = skillRefs
	return request
}

func ConvertPlanCreateClientRequest2ChatRequest(clientRequest AgentPlanCreateClientRequest) agents.ChatRequest {
	request := agents.ChatRequest{}
	request.Streaming = true
	request.ConversationId = clientRequest.ConversationId
	request.Query = clientRequest.Query
	request.AppConfigId = clientRequest.AppConfigId
	request.Files = convertInterfaces2Files(clientRequest.Files)
	return request
}

func ConvertPlanCreateClientRequest2DORequest(clientRequest AgentPlanCreateClientRequest) agents.AgentPlanCreateRequest {
	request := agents.AgentPlanCreateRequest{}
	request.Query = clientRequest.Query
	request.ConversationId = clientRequest.ConversationId
	request.AppConfigId = clientRequest.AppConfigId
	request.Files = convertInterfaces2Files(clientRequest.Files)

	return request
}

func ConvertPlanUpdateClientRequest2DORequest(clientRequest AgentPlanUpdateClientRequest) agents.AgentPlanUpdateRequest {
	request := agents.AgentPlanUpdateRequest{}
	request.ConversationId = clientRequest.ConversationId
	request.AppConfigId = clientRequest.AppConfigId
	request.PlanId = clientRequest.PlanId
	request.StepId = clientRequest.StepId

	return request
}
