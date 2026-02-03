package dtos

import (
	"mooc-manus/internal/domains/models"

	"github.com/google/uuid"
)

type A2AServerConfig struct {
	ServerConfigId string   `json:"serverConfigId"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Url            string   `json:"url"`
	FunctionIds    []string `json:"functionIds"`
}

type CreateA2AServersRequest struct {
	AppConfigId      string            `json:"appConfigId"`
	A2AServerConfigs []A2AServerConfig `json:"a2aServerConfigs"`
}

type UpdateA2AServersRequest struct {
	AppConfigId      string            `json:"appConfigId"`
	A2AServerConfigs []A2AServerConfig `json:"a2aServerConfigs"`
}

type DeleteA2AServersRequest struct {
	A2AServerConfigIds []string `json:"a2aServerConfigIds"`
}

type AgentSkillDTO struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples"`
}

type A2AServerConfigDTO struct {
	ID          string          `json:"id"`
	AppConfigID string          `json:"appConfigId"`
	BaseURL     string          `json:"baseUrl"`
	Enabled     bool            `json:"enabled"`
	ExtInfo     map[string]any  `json:"extInfo"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Skills      []AgentSkillDTO `json:"skills"`
}

func ConvertAgentSkillDO2DTO(do models.AgentSkill) AgentSkillDTO {
	return AgentSkillDTO{
		ID:          do.ID,
		Name:        do.Name,
		Description: do.Description,
		Tags:        do.Tags,
		Examples:    do.Examples,
	}
}

func ConvertA2AServerConfigDO2DTO(do models.A2AServerConfigDO) A2AServerConfigDTO {
	skills := make([]AgentSkillDTO, 0, len(do.Skills))
	for _, skill := range do.Skills {
		skills = append(skills, ConvertAgentSkillDO2DTO(skill))
	}

	return A2AServerConfigDTO{
		ID:          do.ID,
		AppConfigID: do.AppConfigID,
		BaseURL:     do.BaseURL,
		Enabled:     do.Enabled,
		ExtInfo:     do.ExtInfo,
		Name:        do.Name,
		Description: do.Description,
		Skills:      skills,
	}
}

func ConvertUpdateA2ARequest2AppConfigDO(request UpdateA2AServersRequest) models.AppConfigDO {
	serverConfigs := make([]models.A2AServerConfig, len(request.A2AServerConfigs))
	for i, config := range request.A2AServerConfigs {
		serverConfigs[i] = models.A2AServerConfig{
			ID:          config.ServerConfigId,
			Name:        config.Name,
			Description: config.Description,
			Url:         config.Url,
			FunctionIds: config.FunctionIds,
		}
	}
	return models.AppConfigDO{
		AppConfigID:      request.AppConfigId,
		A2AServerConfigs: serverConfigs,
	}
}

func ConvertCreateA2AServersRequest2AppConfigDO(request CreateA2AServersRequest) models.AppConfigDO {
	serverConfigs := make([]models.A2AServerConfig, len(request.A2AServerConfigs))
	for i, config := range request.A2AServerConfigs {
		serverConfigs[i] = models.A2AServerConfig{
			ID:          uuid.New().String(),
			Name:        config.Name,
			Description: config.Description,
			Url:         config.Url,
			FunctionIds: config.FunctionIds,
		}
	}
	return models.AppConfigDO{
		AppConfigID:      request.AppConfigId,
		A2AServerConfigs: serverConfigs,
	}
}
