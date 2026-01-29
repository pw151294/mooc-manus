package models

import "time"

type ToolProviderPO struct {
	ID                string    `gorm:"type:varchar(36);primary_key" json:"id"`
	ProviderName      string    `gorm:"type:varchar(255);not null;uniqueIndex" json:"providerName"`
	ProviderType      string    `gorm:"type:varchar(100);not null" json:"providerType"`
	ProviderDesc      string    `gorm:"type:text" json:"providerDesc"`
	ProviderURL       string    `gorm:"type:varchar(255)" json:"providerUrl"`
	ProviderTransport string    `gorm:"type:varchar(100)" json:"providerTransport"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (ToolProviderPO) TableName() string {
	return "tool_provider"
}
