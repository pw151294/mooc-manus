package tools

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/llm"
)

type Tool interface {
	GetTools() []llm.Tool
	HasTool(funcName string) bool
	Invoke(funcName, funcArgs string) models.ToolCallResult
	Init() error
	ProviderName() string
	SupportsRiskAssessment() bool // 【HITL 新增】true 表示该工具的 arguments 需要读 risk_level / risk_reason
}

func InitTools(providers []models.ToolProviderDO, proId2Funcs map[string][]models.ToolFunctionDO, srvCfgs []models.A2AServerConfigDO) ([]Tool, error) {
	if len(providers) == 0 {
		return nil, nil
	}

	tools := make([]Tool, 0, len(providers))
	for _, provider := range providers {
		proId := provider.ProviderID
		if funcs, ok := proId2Funcs[proId]; ok {
			var tool Tool
			switch provider.ProviderType {
			case "MCP":
				tool = NewMcpTool(provider, funcs)
			case "CUSTOM":
				tool = NewCustomTool(provider, funcs)
			case "A2A":
				tool = NewA2ATool(provider, funcs, srvCfgs)
			}
			if tool != nil {
				if err := tool.Init(); err != nil {
					return nil, err
				}
				tools = append(tools, tool)
			}
		}
	}

	return tools, nil
}

type BaseTool struct {
	providerId   string
	providerName string
	providerType string
	functions    []models.ToolFunctionDO
}

func (t *BaseTool) GetTools() []llm.Tool {
	params := make([]llm.Tool, 0, len(t.functions))
	for _, function := range t.functions {
		params = append(params, convertDO2Tool(function))
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

// SupportsRiskAssessment 默认返回 false；需要接入 HITL 的工具（如 BashExecTool）自行覆写为 true
func (t *BaseTool) SupportsRiskAssessment() bool { return false }
