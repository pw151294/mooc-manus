package models

import (
	"encoding/json"
	"time"

	infra "mooc-manus/internal/infra/models"

	"github.com/google/uuid"
)

// SkillProviderDO Skill 提供者聚合根
type SkillProviderDO struct {
	SkillProviderID string
	ProviderName    string
	ProviderType    string // ProviderTypeGit / ProviderTypeZip / ProviderTypeCustom
	AuthType        string // AuthTypeHttpToken / AuthTypeNone
	RepoURL         string
	Status          string // StatusActive / StatusDisabled
	Creator         string
	Updator         string
	ExtInfo         SkillProviderExtInfo
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SkillProviderExtInfo Provider 扩展信息（预留，未来可承载 Git 仓库认证 secret 引用、扫描配置等）
type SkillProviderExtInfo struct {
	// 预留字段，当前无标准 Key
}

// IsActive 判断是否启用状态
func (p *SkillProviderDO) IsActive() bool {
	return p.Status == StatusActive
}

// Disable 状态置为禁用
func (p *SkillProviderDO) Disable() {
	p.Status = StatusDisabled
}

// Enable 状态置为启用
func (p *SkillProviderDO) Enable() {
	p.Status = StatusActive
}

func ConvertSkillProviderDO2PO(do SkillProviderDO) infra.SkillProviderPO {
	if do.SkillProviderID == "" {
		do.SkillProviderID = uuid.New().String()
	}
	if do.Status == "" {
		do.Status = StatusActive
	}
	extBytes, _ := json.Marshal(do.ExtInfo)
	return infra.SkillProviderPO{
		ID:           do.SkillProviderID,
		ProviderName: do.ProviderName,
		ProviderType: do.ProviderType,
		AuthType:     do.AuthType,
		RepoURL:      do.RepoURL,
		Status:       do.Status,
		Creator:      do.Creator,
		Updator:      do.Updator,
		ExtInfo:      string(extBytes),
	}
}

func ConvertSkillProviderPO2DO(po infra.SkillProviderPO) SkillProviderDO {
	var ext SkillProviderExtInfo
	if po.ExtInfo != "" {
		_ = json.Unmarshal([]byte(po.ExtInfo), &ext)
	}
	return SkillProviderDO{
		SkillProviderID: po.ID,
		ProviderName:    po.ProviderName,
		ProviderType:    po.ProviderType,
		AuthType:        po.AuthType,
		RepoURL:         po.RepoURL,
		Status:          po.Status,
		Creator:         po.Creator,
		Updator:         po.Updator,
		ExtInfo:         ext,
		CreatedAt:       po.CreatedAt,
		UpdatedAt:       po.UpdatedAt,
	}
}
