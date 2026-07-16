package models

import (
	"time"

	"gorm.io/datatypes"
)

type EvalCasePO struct {
	ID           string         `gorm:"type:uuid;primaryKey"`
	Name         string         `gorm:"type:varchar(255);uniqueIndex"`
	Description  string         `gorm:"type:text"`
	InitScript   string         `gorm:"type:text"`
	TaskPrompt   string         `gorm:"type:text"`
	VerifyScript string         `gorm:"type:text"`
	Tags         datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (EvalCasePO) TableName() string {
	return "eval_case"
}
