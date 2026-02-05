package agents

type ChatRequest struct {
	Streaming      bool
	ApiKey         string
	SystemPrompt   string
	ConversationId string
	Query          string
	AppConfigId    string
	FunctionIds    []string
	ProviderIds    []string
	Files          []File
}

type AgentPlanCreateRequest struct {
	ApiKey         string
	ConversationId string
	Query          string
	AppConfigId    string
	Files          []File
}

type AgentPlanUpdateRequest struct {
	ApiKey         string
	ConversationId string
	AppConfigId    string
	PlanId         string
	StepId         string
}

type AgentExecuteRequest struct {
	ApiKey         string
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
	request.ApiKey = planCreateRequest.ApiKey
	request.ConversationId = planCreateRequest.ConversationId
	request.Query = planCreateRequest.Query
	request.AppConfigId = planCreateRequest.AppConfigId
	request.Files = planCreateRequest.Files
	return request
}

func ConvertPlanUpdateRequest2ChatRequest(planUpdateRequest AgentPlanUpdateRequest) ChatRequest {
	request := ChatRequest{}
	request.ApiKey = planUpdateRequest.ApiKey
	request.ConversationId = planUpdateRequest.ConversationId
	request.AppConfigId = planUpdateRequest.AppConfigId
	return request
}

func ConvertAgentExecuteRequest2ChatRequest(executeRequest AgentExecuteRequest) ChatRequest {
	request := ChatRequest{}
	request.ApiKey = executeRequest.ApiKey
	request.ConversationId = executeRequest.ConversationId
	request.AppConfigId = executeRequest.AppConfigId
	request.FunctionIds = executeRequest.FunctionIds
	request.ProviderIds = executeRequest.ProviderIds

	return request
}
