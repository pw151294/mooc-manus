package evaluation

import "time"

type AgentSnapshot struct {
	ID                 string
	SourceAppConfigID  string
	Model              string
	SystemPrompt       string
	ToolsConfig        map[string]any
	MCPConfig          map[string]any
	A2AConfig          map[string]any
	CreatedAt          time.Time
}
