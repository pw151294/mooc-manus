package tools

import (
	"mooc-manus/internal/domains/models"

	"github.com/openai/openai-go"
)

type Tool interface {
	GetTools() []openai.ChatCompletionToolParam
	HasTool(funcName string) bool
	Invoke(funcName, funcArgs string) models.ToolCallResult
	Init() error
	ProviderName() string
}

type BaseTool struct {
	providerId   string
	providerName string
	providerType string
	functions    []models.ToolFunctionDO
}

func (t *BaseTool) GetTools() []openai.ChatCompletionToolParam {
	params := make([]openai.ChatCompletionToolParam, 0, len(t.functions))
	for _, function := range t.functions {
		param := convertDO2Tool(function)
		params = append(params, param)
	}
	return params
}

func (t *BaseTool) HasTool(funcName string) bool {
	for _, function := range t.functions {
		if function.FunctionName == funcName {
			return true
		}
	}
	return false
}

func (t *BaseTool) ProviderName() string {
	return t.providerName
}
