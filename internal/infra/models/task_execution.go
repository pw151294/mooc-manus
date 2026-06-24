package models

import "time"

type TaskExecutionPO struct {
	TaskID     string     `gorm:"column:task_id;type:varchar(100);primary_key" json:"taskId"`
	AppID      string     `gorm:"column:app_id;type:varchar(64);not null;index" json:"appId"`
	AppType    string     `gorm:"column:app_type;type:varchar(64);not null" json:"appType"`
	Status     string     `gorm:"column:status;type:varchar(32);not null;default:PROCESSING;index" json:"status"`
	Stage      string     `gorm:"column:stage;type:varchar(32)" json:"stage"`
	Progress   int        `gorm:"column:progress;type:integer;not null;default:0" json:"progress"`
	ExtInfo    string     `gorm:"column:ext_info;type:jsonb" json:"extInfo"`
	Creator    string     `gorm:"column:creator;type:varchar(64)" json:"creator"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime;index" json:"createdAt"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	ArchivedAt *time.Time `gorm:"column:archived_at" json:"archivedAt"`
}

func (TaskExecutionPO) TableName() string {
	return "task_execution"
}
