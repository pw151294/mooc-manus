package models

import (
	"encoding/json"
	"mooc-manus/internal/infra/models"
)

type ToolCallResult struct {
	Success bool
	Message string
	Data    any
}

type ToolSchema struct {
	Name        string         // 函数名称
	Description string         // 函数描述
	Parameters  map[string]any // 函数参数结构
}
type ToolFunctionDO struct {
	FunctionID   string
	ProviderID   string
	FunctionName string
	FunctionDesc string
	Schema       ToolSchema
}

func ConvertToolFunctionDo2POs(dos []ToolFunctionDO) ([]models.ToolFunctionPO, error) {
	var pos []models.ToolFunctionPO
	for _, do := range dos {
		po, err := ConvertToolFunctionDO2PO(do)
		if err != nil {
			return nil, err
		}
		pos = append(pos, po)
	}
	return pos, nil
}

func ConvertToolFunctionDO2PO(do ToolFunctionDO) (models.ToolFunctionPO, error) {
	params, err := json.Marshal(do.Schema.Parameters)
	if err != nil {
		return models.ToolFunctionPO{}, err
	}
	return models.ToolFunctionPO{
		ID:           do.FunctionID,
		ProviderID:   do.ProviderID,
		FunctionName: do.FunctionName,
		FunctionDesc: do.FunctionDesc,
		Parameters:   string(params),
	}, nil
}

func ConvertToolFunctionPO2DO(po models.ToolFunctionPO) (ToolFunctionDO, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(po.Parameters), &params); err != nil {
		return ToolFunctionDO{}, err
	}
	return ToolFunctionDO{
		FunctionID:   po.ID,
		ProviderID:   po.ProviderID,
		FunctionName: po.FunctionName,
		FunctionDesc: po.FunctionDesc,
		Schema: ToolSchema{
			Name:        po.FunctionName,
			Description: po.FunctionDesc,
			Parameters:  params,
		},
	}, nil
}

func ConvertToolCallResult2Text(result ToolCallResult) string {
	text, ok := result.Data.(string)
	if !ok {
		dataBytes, _ := json.Marshal(result.Data)
		text = string(dataBytes)
	}
	return text
}
