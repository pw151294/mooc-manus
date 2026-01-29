package models

import "time"

type AppConfigPO struct {
	ID               string    `gorm:"type:varchar(36);primary_key" json:"id"`
	BaseUrl          string    `gorm:"type:varchar(255);not null" json:"baseUrl"`
	ApiKey           string    `gorm:"type:varchar(255);not null" json:"apiKey"`
	ModelName        string    `gorm:"type:varchar(100);not null" json:"modelName"`
	Temperature      float64   `gorm:"type:decimal(3,2);not null" json:"temperature"`
	MaxTokens        int64     `gorm:"type:integer;not null" json:"maxTokens"`
	MaxIterations    int       `gorm:"type:integer;not null" json:"maxIterations"`
	MaxRetries       int       `gorm:"type:integer;not null" json:"maxRetries"`
	MaxSearchResults int       `gorm:"type:integer;not null" json:"maxSearchResults"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt        time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (AppConfigPO) TableName() string {
	return "app_config"
}
