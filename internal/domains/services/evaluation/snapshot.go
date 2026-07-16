package evaluation

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	appconfig "mooc-manus/internal/domains/models"
	ev "mooc-manus/internal/domains/models/evaluation"
)

// FreezeAppConfig 深拷贝 AppConfigDO 生成一个不可变的 AgentSnapshot。
// spec §2.5：任务提交时冻结 AppConfig，防止后续修改污染进行中/已完成的评测。
// 深拷贝语义通过 json.Marshal/Unmarshal 走一遍保证（切片元素完全脱离源引用）。
func FreezeAppConfig(cfg *appconfig.AppConfigDO) (*ev.AgentSnapshot, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil app config")
	}

	// 深拷贝 A2A 列表：通过 json 序列化/反序列化脱离源引用
	a2aBytes, err := json.Marshal(cfg.A2AServerConfigs)
	if err != nil {
		return nil, fmt.Errorf("marshal a2a configs: %w", err)
	}
	var a2aClone []appconfig.A2AServerConfig
	if err := json.Unmarshal(a2aBytes, &a2aClone); err != nil {
		return nil, fmt.Errorf("unmarshal a2a configs: %w", err)
	}

	// 映射到评测域的 AgentA2AServer（去掉 FunctionIds 等评测不关心字段）
	a2aSnap := make([]ev.AgentA2AServer, 0, len(a2aClone))
	for _, s := range a2aClone {
		a2aSnap = append(a2aSnap, ev.AgentA2AServer{
			ID:          s.ID,
			Name:        s.Name,
			URL:         s.Url,
			Description: s.Description,
		})
	}

	return &ev.AgentSnapshot{
		ID:                uuid.NewString(),
		SourceAppConfigID: cfg.AppConfigID,
		Model: ev.ModelSnapshot{
			Provider:    cfg.ModelConfig.Provider,
			BaseUrl:     cfg.ModelConfig.BaseUrl,
			ApiKey:      cfg.ModelConfig.ApiKey,
			ModelName:   cfg.ModelConfig.ModelName,
			Temperature: cfg.ModelConfig.Temperature,
			MaxTokens:   cfg.ModelConfig.MaxTokens,
		},
		SystemPrompt: "", // 预留：后续由 case / config 补
		ToolsConfig:  nil,
		MCPConfig:    nil,
		A2AConfig:    a2aSnap,
		CreatedAt:    time.Now(),
	}, nil
}
