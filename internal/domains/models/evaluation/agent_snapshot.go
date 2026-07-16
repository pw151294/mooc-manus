package evaluation

import (
	"time"

	appconfig "mooc-manus/internal/domains/models"
)

// ModelSnapshot 冻结 AppConfigDO.ModelConfig 的全量字段。
// 评测冻结时机的模型配置一旦快照生成便不可变，后续 InternalChatRunner
// 会依赖这些字段发起 LLM 调用（provider / baseurl / apikey / model / 温度 / max tokens）。
type ModelSnapshot struct {
	Provider    string
	BaseUrl     string
	ApiKey      string
	ModelName   string
	Temperature float64
	MaxTokens   int64
}

// AgentA2AServer 冻结 A2A 远端 agent 的接入信息（对应 AppConfigDO.A2AServerConfigs）。
// 只保留评测重放需要的最小字段集合。
type AgentA2AServer struct {
	ID          string
	Name        string
	URL         string
	Description string
}

// AgentSnapshot 是评测系统的 Agent 配置快照（不可变）。
// spec §2.5 要求：任务提交时冻结 AppConfig，防止后续修改污染进行中/已完成的评测。
type AgentSnapshot struct {
	ID                string
	SourceAppConfigID string
	Model             ModelSnapshot
	SystemPrompt      string // 评测评估时走 ChatRequest.SystemPrompt 传入
	ToolsConfig       map[string]any
	MCPConfig         map[string]any
	A2AConfig         []AgentA2AServer
	CreatedAt         time.Time
}

// 评测 AgentConfig 兜底默认值：snapshot 目前未冻结 AgentConfig，
// 评测目标是让智能体正常执行到收敛，用一组合理默认即可（对齐生产常规值）。
const (
	defaultMaxIterations    = 20
	defaultMaxRetries       = 3
	defaultMaxSearchResults = 10
)

// ToAppConfig 将不可变 AgentSnapshot 还原为 AppConfigDO，供 InternalChatRunner
// 通过 ChatRequest.ConfigOverride 注入到 BaseAgent（spec §3.5）。
// 注意：AgentConfig 用兜底默认值；FunctionIds 未在 snapshot 冻结，A2A 项保留空切片。
func (s *AgentSnapshot) ToAppConfig() *appconfig.AppConfigDO {
	if s == nil {
		return nil
	}
	a2a := make([]appconfig.A2AServerConfig, 0, len(s.A2AConfig))
	for _, x := range s.A2AConfig {
		a2a = append(a2a, appconfig.A2AServerConfig{
			ID:          x.ID,
			Name:        x.Name,
			Description: x.Description,
			Url:         x.URL,
			// FunctionIds 未在 snapshot 冻结，留空
		})
	}
	return &appconfig.AppConfigDO{
		AppConfigID: s.SourceAppConfigID,
		ModelConfig: appconfig.ModelConfig{
			Provider:    s.Model.Provider,
			BaseUrl:     s.Model.BaseUrl,
			ApiKey:      s.Model.ApiKey,
			ModelName:   s.Model.ModelName,
			Temperature: s.Model.Temperature,
			MaxTokens:   s.Model.MaxTokens,
		},
		AgentConfig: appconfig.AgentConfig{
			MaxIterations:    defaultMaxIterations,
			MaxRetries:       defaultMaxRetries,
			MaxSearchResults: defaultMaxSearchResults,
		},
		A2AServerConfigs: a2a,
	}
}
