package agents

type AgentChatRequest struct {
	Streaming      bool
	ApiKey         string
	SystemPrompt   string
	ConversationId string
	Query          string
	AppConfigId    string
	FunctionIds    []string
	Files          []interface{}
}
