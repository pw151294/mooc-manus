package dtos

import "mooc-manus/internal/domains/models/agents"

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

func ConvertAgentChatClientRequest2Request(clientRequest AgentChatClientRequest) agents.AgentChatRequest {
	request := agents.AgentChatRequest{}
	request.Streaming = clientRequest.Streaming
	request.ConversationId = clientRequest.ConversationId
	request.SystemPrompt = clientRequest.SystemPrompt
	request.ApiKey = clientRequest.ApiKey
	request.Query = clientRequest.Query
	request.AppConfigId = clientRequest.AppConfigId
	request.FunctionIds = clientRequest.FunctionIds
	//request.Files = ? todo 此处需要设计转换的格式
	return request
}
