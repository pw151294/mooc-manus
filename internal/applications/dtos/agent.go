package dtos

import (
	"mooc-manus/internal/domains/models/agents"
)

type AgentChatClientRequest struct {
	Streaming      bool          `json:"streaming"`
	ApiKey         string        `json:"apiKey"`
	SystemPrompt   string        `json:"systemPrompt"`
	Query          string        `json:"query"`
	ConversationId string        `json:"conversationId"`
	AppConfigId    string        `json:"appConfigId"`
	FunctionIds    []string      `json:"functionIds"`
	Files          []interface{} `json:"files"`
}

type AgentPlanCreateClientRequest struct {
	ApiKey         string        `json:"apiKey"`
	ConversationId string        `json:"conversationId"`
	Query          string        `json:"query"`
	AppConfigId    string        `json:"appConfigId"`
	Files          []interface{} `json:"files"`
}

type AgentPlanUpdateClientRequest struct {
	ApiKey         string `json:"apiKey"`
	ConversationId string `json:"conversationId"`
	AppConfigId    string `json:"appConfigId"`
	PlanId         string `json:"planId"`
	StepId         string `json:"stepId"`
}

func convertInterfaces2Files(datas []interface{}) []agents.File {
	files := make([]agents.File, 0, len(datas))
	for _, data := range datas {
		file, ok := data.(agents.File)
		if ok {
			files = append(files, file)
		}
	}
	return files
}

func ConvertAgentChatClientRequest2Request(clientRequest AgentChatClientRequest) agents.AgentChatRequest {
	request := agents.AgentChatRequest{}
	request.Streaming = clientRequest.Streaming
	request.ConversationId = clientRequest.ConversationId
	request.SystemPrompt = clientRequest.SystemPrompt
	request.ApiKey = clientRequest.ApiKey
	request.Query = clientRequest.Query
	request.AppConfigId = clientRequest.AppConfigId
	request.FunctionIds = clientRequest.FunctionIds
	request.Files = convertInterfaces2Files(clientRequest.Files)
	return request
}

func ConvertPlanCreateClientRequest2ChatRequest(clientRequest AgentPlanCreateClientRequest) agents.AgentChatRequest {
	request := agents.AgentChatRequest{}
	request.Streaming = true
	request.ApiKey = clientRequest.ApiKey
	request.ConversationId = clientRequest.ConversationId
	request.Query = clientRequest.Query
	request.AppConfigId = clientRequest.AppConfigId
	request.Files = convertInterfaces2Files(clientRequest.Files)
	return request
}

func ConvertPlanCreateClientRequest2DORequest(clientRequest AgentPlanCreateClientRequest) agents.AgentPlanCreateRequest {
	request := agents.AgentPlanCreateRequest{}
	request.Query = clientRequest.Query
	request.ApiKey = clientRequest.ApiKey
	request.ConversationId = clientRequest.ConversationId
	request.AppConfigId = clientRequest.AppConfigId
	request.Files = convertInterfaces2Files(clientRequest.Files)

	return request
}

func ConvertPlanUpdateClientRequest2DORequest(clientRequest AgentPlanUpdateClientRequest) agents.AgentPlanUpdateRequest {
	request := agents.AgentPlanUpdateRequest{}
	request.ApiKey = clientRequest.ApiKey
	request.ConversationId = clientRequest.ConversationId
	request.AppConfigId = clientRequest.AppConfigId
	request.PlanId = clientRequest.PlanId
	request.StepId = clientRequest.StepId

	return request
}
