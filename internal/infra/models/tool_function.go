package models

import "time"

type ToolFunctionPO struct {
	ID           string    `gorm:"type:varchar(36);primary_key" json:"id"`
	ProviderID   string    `gorm:"type:varchar(36);not null;index" json:"providerId"`
	FunctionName string    `gorm:"type:varchar(255);not null" json:"functionName"`
	FunctionDesc string    `gorm:"type:text" json:"functionDesc"`
	Parameters   string    `gorm:"type:jsonb" json:"parameters"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (ToolFunctionPO) TableName() string {
	return "tool_function"
}
