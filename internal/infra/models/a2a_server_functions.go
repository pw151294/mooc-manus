package models

import "time"

type A2AServerFunctionPO struct {
	ID                string    `gorm:"column:id;type:varchar(36);primary_key" json:"id"`
	A2AServerConfigID string    `gorm:"column:a2a_server_config_id;type:varchar(36);not null;index" json:"a2aServerConfigId"`
	FunctionID        string    `gorm:"column:function_id;type:varchar(36);not null;index" json:"functionId"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime" json:"createTime"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updateTime"`
}

func (A2AServerFunctionPO) TableName() string {
	return "a2a_server_functions"
}
