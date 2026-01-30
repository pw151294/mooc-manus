package agents

type AgentChatRequest struct {
	Streaming      bool
	ApiKey         string
	SystemPrompt   string
	ConversationId string
	Query          string
	AppConfigId    string
	FunctionIds    []string
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

func ConvertPlanCreateRequest2ChatRequest(planCreateRequest AgentPlanCreateRequest) AgentChatRequest {
	request := AgentChatRequest{}
	request.ApiKey = planCreateRequest.ApiKey
	request.ConversationId = planCreateRequest.ConversationId
	request.Query = planCreateRequest.Query
	request.AppConfigId = planCreateRequest.AppConfigId
	request.Files = planCreateRequest.Files
	return request
}

func ConvertPlanUpdateRequest2ChatRequest(planUpdateRequest AgentPlanUpdateRequest) AgentChatRequest {
	request := AgentChatRequest{}
	request.ApiKey = planUpdateRequest.ApiKey
	request.ConversationId = planUpdateRequest.ConversationId
	request.AppConfigId = planUpdateRequest.AppConfigId
	return request
}
