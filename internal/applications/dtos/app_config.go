package dtos

import (
	"mooc-manus/internal/domains/models"

	"github.com/google/uuid"
)

type AppConfigDTO struct {
	AppConfigID      string  `json:"appConfigId"`
	BaseUrl          string  `json:"baseUrl"`
	ModelName        string  `json:"modelName"`
	Temperature      float64 `json:"temperature"`
	MaxTokens        int64   `json:"maxTokens"`
	MaxIterations    int     `json:"maxIterations"`
	MaxRetries       int     `json:"maxRetries"`
	MaxSearchResults int     `json:"maxSearchResults"`
}
type AppConfigCreateRequest struct {
	BaseUrl          string  `json:"baseUrl" binding:"required"`
	ApiKey           string  `json:"apiKey" binding:"required"`
	ModelName        string  `json:"modelName" binding:"required"`
	Temperature      float64 `json:"temperature"`
	MaxTokens        int64   `json:"maxTokens"`
	MaxIterations    int     `json:"maxIterations"`
	MaxRetries       int     `json:"maxRetries"`
	MaxSearchResults int     `json:"maxSearchResults"`
}
type AppConfigUpdateRequest struct {
	AppConfigID      string  `json:"appConfigId" binding:"required"`
	BaseUrl          string  `json:"baseUrl" binding:"required"`
	ApiKey           string  `json:"apiKey"` // ApiKey is optional on update
	ModelName        string  `json:"modelName" binding:"required"`
	Temperature      float64 `json:"temperature"`
	MaxTokens        int64   `json:"maxTokens"`
	MaxIterations    int     `json:"maxIterations"`
	MaxRetries       int     `json:"maxRetries"`
	MaxSearchResults int     `json:"maxSearchResults"`
}

func ConvertAppConfigCreateRequest2DO(request AppConfigCreateRequest) models.AppConfigDO {
	if request.Temperature == 0 {
		request.Temperature = defaultModelTemperature
	}
	if request.MaxTokens == 0 {
		request.MaxTokens = defaultModelMaxTokens
	}
	if request.MaxIterations == 0 {
		request.MaxIterations = defaultAgentMaxIterations
	}
	if request.MaxRetries == 0 {
		request.MaxRetries = defaultAgentMaxRetries
	}
	if request.MaxSearchResults == 0 {
		request.MaxSearchResults = defaultAgentMaxSearchResults
	}

	return models.AppConfigDO{
		AppConfigID: uuid.New().String(),
		ModelConfig: models.ModelConfig{
			BaseUrl:     request.BaseUrl,
			ApiKey:      request.ApiKey,
			ModelName:   request.ModelName,
			Temperature: request.Temperature,
			MaxTokens:   request.MaxTokens,
		},
		AgentConfig: models.AgentConfig{
			MaxIterations:    request.MaxIterations,
			MaxRetries:       request.MaxRetries,
			MaxSearchResults: request.MaxSearchResults,
		},
	}
}

func ConvertAppConfigUpdateRequest2DO(request AppConfigUpdateRequest) models.AppConfigDO {
	if request.Temperature == 0 {
		request.Temperature = defaultModelTemperature
	}
	if request.MaxTokens == 0 {
		request.MaxTokens = defaultModelMaxTokens
	}
	if request.MaxIterations == 0 {
		request.MaxIterations = defaultAgentMaxIterations
	}
	if request.MaxRetries == 0 {
		request.MaxRetries = defaultAgentMaxRetries
	}
	if request.MaxSearchResults == 0 {
		request.MaxSearchResults = defaultAgentMaxSearchResults
	}

	return models.AppConfigDO{
		AppConfigID: request.AppConfigID,
		ModelConfig: models.ModelConfig{
			BaseUrl:     request.BaseUrl,
			ApiKey:      request.ApiKey,
			ModelName:   request.ModelName,
			Temperature: request.Temperature,
			MaxTokens:   request.MaxTokens,
		},
		AgentConfig: models.AgentConfig{
			MaxIterations:    request.MaxIterations,
			MaxRetries:       request.MaxRetries,
			MaxSearchResults: request.MaxSearchResults,
		},
	}
}

func ConvertAppConfigDO2DTO(do models.AppConfigDO) AppConfigDTO {
	return AppConfigDTO{
		AppConfigID:      do.AppConfigID,
		BaseUrl:          do.ModelConfig.BaseUrl,
		ModelName:        do.ModelConfig.ModelName,
		Temperature:      do.ModelConfig.Temperature,
		MaxTokens:        do.ModelConfig.MaxTokens,
		MaxIterations:    do.AgentConfig.MaxIterations,
		MaxRetries:       do.AgentConfig.MaxRetries,
		MaxSearchResults: do.AgentConfig.MaxSearchResults,
	}
}
