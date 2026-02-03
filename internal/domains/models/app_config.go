package models

import (
	"mooc-manus/internal/infra/models"
	"mooc-manus/pkg/crypt"
)

type ModelConfig struct {
	BaseUrl     string
	ApiKey      string
	ModelName   string
	Temperature float64
	MaxTokens   int64
}
type AgentConfig struct {
	MaxIterations    int
	MaxRetries       int
	MaxSearchResults int
}

type A2AServerConfig struct {
	ID          string
	Name        string
	Description string
	Url         string
	FunctionIds []string
}

type AppConfigDO struct {
	ModelConfig      ModelConfig
	AgentConfig      AgentConfig
	A2AServerConfigs []A2AServerConfig
	AppConfigID      string
}

func ConvertAppConfigDO2PO(appConfigDO AppConfigDO) models.AppConfigPO {
	return models.AppConfigPO{
		ID:               appConfigDO.AppConfigID,
		BaseUrl:          appConfigDO.ModelConfig.BaseUrl,
		ApiKey:           crypt.EncodeMd5(appConfigDO.ModelConfig.ApiKey),
		ModelName:        appConfigDO.ModelConfig.ModelName,
		Temperature:      appConfigDO.ModelConfig.Temperature,
		MaxTokens:        appConfigDO.ModelConfig.MaxTokens,
		MaxIterations:    appConfigDO.AgentConfig.MaxIterations,
		MaxRetries:       appConfigDO.AgentConfig.MaxRetries,
		MaxSearchResults: appConfigDO.AgentConfig.MaxSearchResults,
	}
}

func ConvertAppConfigPO2DO(appConfigPO models.AppConfigPO) AppConfigDO {
	return AppConfigDO{
		AppConfigID: appConfigPO.ID,
		ModelConfig: ModelConfig{
			BaseUrl:     appConfigPO.BaseUrl,
			ApiKey:      "", // 不返回敏感信息
			ModelName:   appConfigPO.ModelName,
			Temperature: appConfigPO.Temperature,
			MaxTokens:   appConfigPO.MaxTokens,
		},
		AgentConfig: AgentConfig{
			MaxIterations:    appConfigPO.MaxIterations,
			MaxRetries:       appConfigPO.MaxRetries,
			MaxSearchResults: appConfigPO.MaxSearchResults,
		},
	}
}
