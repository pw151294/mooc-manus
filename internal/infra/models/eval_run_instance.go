package models

import (
	"time"

	"gorm.io/datatypes"
)

type EvalRunInstancePO struct {
	ID                    string         `gorm:"type:uuid;primaryKey"`
	TaskID                string         `gorm:"type:uuid;index:idx_task_status,priority:1;uniqueIndex:uk_task_case_snap,priority:1;constraint:OnDelete:CASCADE"`
	CaseID                string         `gorm:"type:uuid;uniqueIndex:uk_task_case_snap,priority:2"`
	CaseSnapshot          datatypes.JSON `gorm:"type:jsonb"`
	AgentConfigSnapshotID string         `gorm:"type:uuid;uniqueIndex:uk_task_case_snap,priority:3;constraint:OnDelete:RESTRICT"`
	Status                string         `gorm:"type:varchar(24);index:idx_task_status,priority:2;index:idx_status_heartbeat,priority:1"`
	Attempt               int
	ConversationID        string `gorm:"type:varchar(64)"`
	MessageID             string `gorm:"type:varchar(64)"`
	TraceID               string `gorm:"type:varchar(64)"`
	QueuedAt              *time.Time
	StartedAt             *time.Time
	FinishedAt            *time.Time
	HeartbeatAt           *time.Time `gorm:"index:idx_status_heartbeat,priority:2"`
	DeadlineAt            *time.Time
	WorkerID              string `gorm:"type:varchar(64)"`
	ErrorMessage          string `gorm:"type:text"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (EvalRunInstancePO) TableName() string {
	return "eval_run_instance"
}
