package dtos

import (
	"encoding/json"
	"mooc-manus/internal/domains/models"

	"github.com/google/uuid"
)

type AddToolFunctionRequest struct {
	ProviderID   string     `json:"providerId" binding:"required"`
	FunctionName string     `json:"functionName" binding:"required"`
	FunctionDesc string     `json:"functionDesc"`
	Parameters   Parameters `json:"parameters"`
}

type AddMcpFunctionsRequest struct {
	ProviderID   string `json:"providerId"`
	ProviderName string `json:"providerName" binding:"required"`
	ProviderDesc string `json:"providerDesc"`
	ProviderURL  string `json:"providerUrl"`
}

type UpdateToolFunctionRequest struct {
	FunctionID   string     `json:"functionId" binding:"required"`
	ProviderID   string     `json:"providerId" binding:"required"`
	FunctionName string     `json:"functionName" binding:"required"`
	FunctionDesc string     `json:"functionDesc"`
	Parameters   Parameters `json:"parameters"`
}
type ToolFunctionDTO struct {
	FunctionID   string              `json:"functionId"`
	ProviderID   string              `json:"providerId"`
	FunctionName string              `json:"functionName"`
	FunctionDesc string              `json:"functionDesc"`
	Properties   map[string]Property `json:"properties"`
}

type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
}
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

func ConvertParameters2ToolSchema(parameters Parameters) map[string]any {
	required := make([]string, 0, 0)
	properties := parameters.Properties
	for field, property := range properties {
		if property.Required {
			required = append(required, field)
		}
	}

	toolSchema := make(map[string]any)
	toolSchema["type"] = parameters.Type
	toolSchema["properties"] = properties
	toolSchema["required"] = required
	return toolSchema
}

func ConvertAddToolFunctionRequest2DO(request AddToolFunctionRequest) models.ToolFunctionDO {
	return models.ToolFunctionDO{
		FunctionID:   uuid.New().String(),
		ProviderID:   request.ProviderID,
		FunctionName: request.FunctionName,
		FunctionDesc: request.FunctionDesc,
		Schema: models.ToolSchema{
			Name:        request.FunctionName,
			Description: request.FunctionDesc,
			Parameters:  ConvertParameters2ToolSchema(request.Parameters),
		},
	}
}

func ConvertUpdateToolFuncRequest2DO(request UpdateToolFunctionRequest) models.ToolFunctionDO {
	return models.ToolFunctionDO{
		FunctionID:   request.FunctionID,
		ProviderID:   request.ProviderID,
		FunctionName: request.FunctionName,
		FunctionDesc: request.FunctionDesc,
		Schema: models.ToolSchema{
			Name:        request.FunctionName,
			Description: request.FunctionDesc,
			Parameters:  ConvertParameters2ToolSchema(request.Parameters),
		},
	}
}

func ConvertToolFunctionDO2DTO(do models.ToolFunctionDO) (ToolFunctionDTO, error) {
	var params Parameters
	// To convert map[string]any back to Parameters struct for DTO
	// we can marshal the parameters part of the schema and unmarshal it into our struct
	paramBytes, err := json.Marshal(do.Schema.Parameters)
	if err != nil {
		return ToolFunctionDTO{}, err
	}
	if err := json.Unmarshal(paramBytes, &params); err != nil {
		return ToolFunctionDTO{}, err
	}

	return ToolFunctionDTO{
		FunctionID:   do.FunctionID,
		ProviderID:   do.ProviderID,
		FunctionName: do.FunctionName,
		FunctionDesc: do.FunctionDesc,
		Properties:   params.Properties,
	}, nil
}

func ConvertAddMcpFunctionsRequest2ProviderDO(request AddMcpFunctionsRequest) models.ToolProviderDO {
	return models.ToolProviderDO{
		ProviderID:        request.ProviderID,
		ProviderName:      request.ProviderName,
		ProviderType:      "MCP",
		ProviderDesc:      request.ProviderDesc,
		ProviderURL:       request.ProviderURL,
		ProviderTransport: "streamable_http",
	}
}
