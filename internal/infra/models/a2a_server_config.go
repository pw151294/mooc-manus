package models

import "time"

type A2AServerConfigPO struct {
	ID          string    `gorm:"column:id;type:varchar(36);primary_key" json:"id"`
	AppConfigID string    `gorm:"column:app_config_id;type:varchar(36);not null;index" json:"appConfigId"`
	BaseURL     string    `gorm:"column:base_url;type:varchar(255);not null" json:"baseUrl"`
	Enabled     bool      `gorm:"column:enabled;type:boolean;not null" json:"enabled"`
	ExtInfo     string    `gorm:"column:ext_info;type:jsonb" json:"extInfo"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"createTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updateTime"`
	Name        string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Description string    `gorm:"column:description;type:text" json:"description"`
}

func (A2AServerConfigPO) TableName() string {
	return "a2a_server_config"
}
