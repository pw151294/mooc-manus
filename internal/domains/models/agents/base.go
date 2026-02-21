package agents

import (
	"mooc-manus/internal/domains/models/file"
)

type ChatRequest struct {
	Streaming      bool
	SystemPrompt   string
	ConversationId string
	Query          string
	AppConfigId    string
	FunctionIds    []string
	ProviderIds    []string
	Files          []file.File
}

type AgentPlanCreateRequest struct {
	ConversationId string
	Query          string
	AppConfigId    string
	Files          []file.File
}

type AgentPlanUpdateRequest struct {
	ConversationId string
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
	request.Query = planCreateRequest.Query
	request.AppConfigId = planCreateRequest.AppConfigId
	request.Files = planCreateRequest.Files
	return request
}

func ConvertPlanUpdateRequest2ChatRequest(planUpdateRequest AgentPlanUpdateRequest) ChatRequest {
	request := ChatRequest{}
	request.ConversationId = planUpdateRequest.ConversationId
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
