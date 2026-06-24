package models

import (
	"encoding/json"
	"time"

	infra "mooc-manus/internal/infra/models"

	"github.com/google/uuid"
)

// SkillDO Skill 聚合根
type SkillDO struct {
	SkillID         string
	SkillName       string
	SkillProviderID string
	Description     string
	LatestVersionID string
	Status          string // StatusActive / StatusDisabled
	Creator         string
	Updator         string
	ExtInfo         SkillExtInfo
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Icon Skill 图标值对象
type Icon struct {
	Icon           string `json:"icon"`
	IconBackground string `json:"iconBackground"`
	IconType       string `json:"iconType"`
}

// SkillExtInfo Skill 表的 ext_info JSON 结构
type SkillExtInfo struct {
	Icon     *Icon  `json:"icon,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
}

// UpdateLatestVersion 更新最新版本指针（领域行为）
func (s *SkillDO) UpdateLatestVersion(versionID string) {
	s.LatestVersionID = versionID
}

// ClearLatestVersion 清空最新版本指针（如最新版本被删除时）
func (s *SkillDO) ClearLatestVersion() {
	s.LatestVersionID = ""
}

// IsActive 判断状态是否启用
func (s *SkillDO) IsActive() bool {
	return s.Status == StatusActive
}

// Disable 状态置为禁用
func (s *SkillDO) Disable() {
	s.Status = StatusDisabled
}

// Enable 状态置为启用
func (s *SkillDO) Enable() {
	s.Status = StatusActive
}

func ConvertSkillDO2PO(do SkillDO) infra.SkillPO {
	if do.SkillID == "" {
		do.SkillID = uuid.New().String()
	}
	if do.Status == "" {
		do.Status = StatusActive
	}
	extBytes, _ := json.Marshal(do.ExtInfo)
	return infra.SkillPO{
		ID:              do.SkillID,
		SkillName:       do.SkillName,
		SkillProviderID: do.SkillProviderID,
		Description:     do.Description,
		LatestVersionID: do.LatestVersionID,
		Status:          do.Status,
		Creator:         do.Creator,
		Updator:         do.Updator,
		ExtInfo:         string(extBytes),
	}
}

func ConvertSkillPO2DO(po infra.SkillPO) SkillDO {
	var ext SkillExtInfo
	if po.ExtInfo != "" {
		_ = json.Unmarshal([]byte(po.ExtInfo), &ext)
	}
	return SkillDO{
		SkillID:         po.ID,
		SkillName:       po.SkillName,
		SkillProviderID: po.SkillProviderID,
		Description:     po.Description,
		LatestVersionID: po.LatestVersionID,
		Status:          po.Status,
		Creator:         po.Creator,
		Updator:         po.Updator,
		ExtInfo:         ext,
		CreatedAt:       po.CreatedAt,
		UpdatedAt:       po.UpdatedAt,
	}
}
