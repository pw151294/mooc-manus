package models

import "time"

type SkillProviderPO struct {
	ID           string    `gorm:"column:skill_provider_id;type:varchar(36);primary_key" json:"skillProviderId"`
	ProviderName string    `gorm:"column:provider_name;type:varchar(128);not null;uniqueIndex" json:"providerName"`
	ProviderType string    `gorm:"column:provider_type;type:varchar(32);not null" json:"providerType"`
	AuthType     string    `gorm:"column:auth_type;type:varchar(32)" json:"authType"`
	RepoURL      string    `gorm:"column:repo_url;type:varchar(512)" json:"repoUrl"`
	Status       string    `gorm:"column:status;type:varchar(32);not null;default:ACTIVE" json:"status"`
	Creator      string    `gorm:"column:creator;type:varchar(64)" json:"creator"`
	Updator      string    `gorm:"column:updator;type:varchar(64)" json:"updator"`
	ExtInfo      string    `gorm:"column:ext_info;type:jsonb" json:"extInfo"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (SkillProviderPO) TableName() string {
	return "skill_provider"
}
