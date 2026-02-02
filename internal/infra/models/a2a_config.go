package models

import "time"

type A2AConfigPO struct {
	ID          string    `gorm:"type:varchar(36);primary_key" json:"id"`
	AppConfigID string    `gorm:"type:varchar(36);not null;index" json:"appConfigId"`
	ExtInfo     string    `gorm:"type:jsonb" json:"extInfo"`
	CreatedAt   time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdatedAt   time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
}

func (A2AConfigPO) TableName() string {
	return "a2a_config"
}
