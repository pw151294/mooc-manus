package models

import "time"

type A2AServerConfigPO struct {
	ID          string    `gorm:"type:varchar(36);primary_key" json:"id"`
	A2AConfigID string    `gorm:"type:varchar(36);not null;index" json:"a2aConfigId"`
	BaseURL     string    `gorm:"type:varchar(255);not null" json:"baseUrl"`
	Enabled     bool      `gorm:"type:boolean;not null" json:"enabled"`
	ExtInfo     string    `gorm:"type:jsonb" json:"extInfo"`
	CreatedAt   time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdatedAt   time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
}

func (A2AServerConfigPO) TableName() string {
	return "a2a_server_config"
}
