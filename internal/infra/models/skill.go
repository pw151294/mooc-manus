package models

import "time"

type SkillPO struct {
	ID              string    `gorm:"column:skill_id;type:varchar(36);primary_key" json:"skillId"`
	SkillName       string    `gorm:"column:skill_name;type:varchar(120);not null;uniqueIndex" json:"skillName"`
	SkillProviderID string    `gorm:"column:skill_provider_id;type:varchar(36);not null;index" json:"skillProviderId"`
	Description     string    `gorm:"column:description;type:varchar(3000)" json:"description"`
	LatestVersionID string    `gorm:"column:latest_version_id;type:varchar(36)" json:"latestVersionId"`
	Status          string    `gorm:"column:status;type:varchar(32);not null;default:ACTIVE" json:"status"`
	Creator         string    `gorm:"column:creator;type:varchar(64)" json:"creator"`
	Updator         string    `gorm:"column:updator;type:varchar(64)" json:"updator"`
	ExtInfo         string    `gorm:"column:ext_info;type:jsonb" json:"extInfo"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (SkillPO) TableName() string {
	return "skill"
}
