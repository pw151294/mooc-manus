package models

import (
	"encoding/json"
	"mooc-manus/internal/infra/models"
)

type A2AServerConfigDO struct {
	ID          string
	AppConfigID string
	BaseURL     string
	Enabled     bool
	ExtInfo     map[string]any
	Name        string
	Description string
	Skills      []AgentSkill
}

type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples"`
}

func ConvertA2AServerConfigPO2DO(po models.A2AServerConfigPO) (A2AServerConfigDO, error) {
	var extInfo map[string]any
	if err := json.Unmarshal([]byte(po.ExtInfo), &extInfo); err != nil {
		return A2AServerConfigDO{}, err
	}
	return A2AServerConfigDO{
		ID:          po.ID,
		AppConfigID: po.AppConfigID,
		BaseURL:     po.BaseURL,
		Enabled:     po.Enabled,
		ExtInfo:     extInfo,
		Name:        po.Name,
		Description: po.Description,
	}, nil
}

func ConvertA2AServerConfigPOs2DOs(pos []models.A2AServerConfigPO) ([]A2AServerConfigDO, error) {
	dos := make([]A2AServerConfigDO, 0, len(pos))
	for _, po := range pos {
		do, err := ConvertA2AServerConfigPO2DO(po)
		if err != nil {
			return nil, err
		}
		dos = append(dos, do)
	}
	return dos, nil
}

func ConvertA2AServerConfig2DO(config A2AServerConfig) A2AServerConfigDO {
	return A2AServerConfigDO{
		ID:          config.ID,
		Name:        config.Name,
		Description: config.Description,
		BaseURL:     config.Url,
		Enabled:     true,             // 默认启用
		ExtInfo:     map[string]any{}, // 默认扩展信息为空
	}
}

func ConvertAppConfigDO2A2AServerConfigDO(do AppConfigDO) []A2AServerConfigDO {
	serverConfigs := make([]A2AServerConfigDO, 0, len(do.A2AServerConfigs))
	for _, serverConfig := range do.A2AServerConfigs {
		serverConfigDO := ConvertA2AServerConfig2DO(serverConfig)
		serverConfigDO.AppConfigID = do.AppConfigID
		serverConfigs = append(serverConfigs, serverConfigDO)
	}
	return serverConfigs
}

func ConvertA2AServerConfigDO2PO(do A2AServerConfigDO) (models.A2AServerConfigPO, error) {
	extInfo, err := json.Marshal(do.ExtInfo)
	if err != nil {
		return models.A2AServerConfigPO{}, err
	}
	return models.A2AServerConfigPO{
		ID:          do.ID,
		AppConfigID: do.AppConfigID,
		BaseURL:     do.BaseURL,
		Enabled:     do.Enabled,
		ExtInfo:     string(extInfo),
		Name:        do.Name,
		Description: do.Description,
	}, nil
}

func ConvertA2AServerConfigDOs2POs(dos []A2AServerConfigDO) ([]models.A2AServerConfigPO, error) {
	pos := make([]models.A2AServerConfigPO, 0, len(dos))
	for _, do := range dos {
		po, err := ConvertA2AServerConfigDO2PO(do)
		if err != nil {
			return nil, err
		}
		pos = append(pos, po)
	}
	return pos, nil
}

func ConvertToolFunction2AgentSkill(do ToolFunctionDO) AgentSkill {
	// Extract tags from the parameter keys
	tags := make([]string, 0)
	if props, ok := do.Schema.Parameters["properties"]; ok {
		if propsMap, ok := props.(map[string]any); ok {
			for key := range propsMap {
				tags = append(tags, key)
			}
		}
	}

	return AgentSkill{
		ID:          do.FunctionID,
		Name:        do.FunctionName,
		Description: do.FunctionDesc,
		Tags:        tags,
	}
}

func ConvertToolFunctions2AgentSkills(functions []ToolFunctionDO) []AgentSkill {
	skills := make([]AgentSkill, 0, len(functions))
	for _, function := range functions {
		skills = append(skills, ConvertToolFunction2AgentSkill(function))
	}
	return skills
}
