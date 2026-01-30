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

func ConvertPlanCreateRequest2ChatRequest(planCreateRequest AgentPlanCreateRequest) AgentChatRequest {
	request := AgentChatRequest{}
	request.ApiKey = planCreateRequest.ApiKey
	request.ConversationId = planCreateRequest.ConversationId
	request.Query = planCreateRequest.Query
	request.AppConfigId = planCreateRequest.AppConfigId
	request.Files = planCreateRequest.Files
	return request
}
