package models

import (
	"time"

	"gorm.io/datatypes"
)

type EvalTaskPO struct {
	ID             string         `gorm:"type:uuid;primaryKey"`
	Name           string         `gorm:"type:varchar(255)"`
	CaseIDs        datatypes.JSON `gorm:"type:jsonb"`
	AgentConfigIDs datatypes.JSON `gorm:"type:jsonb"`
	Status         string         `gorm:"type:varchar(24);index"`
	TotalCount     int
	SucceededCount int
	FailedCount    int
	RunningCount   int
	CreatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
}

func (EvalTaskPO) TableName() string {
	return "eval_task"
}
