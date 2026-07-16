package models

import (
	"time"

	"gorm.io/datatypes"
)

type EvalAgentSnapshotPO struct {
	ID                string         `gorm:"type:uuid;primaryKey"`
	SourceAppConfigID string         `gorm:"type:uuid;index"`
	Model             datatypes.JSON `gorm:"type:jsonb"`
	SystemPrompt      string         `gorm:"type:text"`
	ToolsConfig       datatypes.JSON `gorm:"type:jsonb"`
	MCPConfig         datatypes.JSON `gorm:"type:jsonb"`
	A2AConfig         datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt         time.Time
}

func (EvalAgentSnapshotPO) TableName() string {
	return "eval_agent_snapshot"
}
