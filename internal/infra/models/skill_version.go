package models

import "time"

type SkillVersionPO struct {
	ID          string    `gorm:"column:skill_version_id;type:varchar(36);primary_key" json:"skillVersionId"`
	SkillID     string    `gorm:"column:skill_id;type:varchar(36);not null;index;uniqueIndex:uk_skill_version" json:"skillId"`
	Version     string    `gorm:"column:version;type:varchar(32);not null;uniqueIndex:uk_skill_version" json:"version"`
	Description string    `gorm:"column:description;type:varchar(3000)" json:"description"`
	Metadata    string    `gorm:"column:metadata;type:jsonb" json:"metadata"`
	SkillFiles  string    `gorm:"column:skill_files;type:jsonb" json:"skillFiles"`
	ExtInfo     string    `gorm:"column:ext_info;type:jsonb" json:"extInfo"`
	Creator     string    `gorm:"column:creator;type:varchar(64)" json:"creator"`
	Updator     string    `gorm:"column:updator;type:varchar(64)" json:"updator"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (SkillVersionPO) TableName() string {
	return "skill_version"
}
