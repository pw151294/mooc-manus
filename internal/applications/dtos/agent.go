package dtos

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/agents"
)

type ChatClientRequest struct {
	Streaming      bool          `json:"streaming"`
	ApiKey         string        `json:"apiKey"`
	SystemPrompt   string        `json:"systemPrompt"`
	Query          string        `json:"query"`
	ConversationId string        `json:"conversationId"`
	AppConfigId    string        `json:"appConfigId"`
	FunctionIds    []string      `json:"functionIds"`
	ProviderIds    []string      `json:"providerIds"`
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

func convertInterfaces2Files(datas []interface{}) []models.File {
	files := make([]models.File, 0, len(datas))
	for _, data := range datas {
		file, ok := data.(models.File)
		if ok {
			files = append(files, file)
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
