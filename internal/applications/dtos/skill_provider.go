package dtos

import (
	"time"

	"mooc-manus/internal/domains/models"
)

// SkillProviderInfo Provider 列表/详情/导入响应 DTO
type SkillProviderInfo struct {
	SkillProviderID string    `json:"skillProviderId"`
	ProviderName    string    `json:"providerName"`
	ProviderType    string    `json:"providerType"`
	AuthType        string    `json:"authType,omitempty"`
	RepoURL         string    `json:"repoUrl,omitempty"`
	Status          string    `json:"status"`
	SkillCount      int       `json:"skillCount"`
	Creator         string    `json:"creator,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// ImportGitRequest Git 仓库导入 Provider 请求
type ImportGitRequest struct {
	ProviderName string `json:"providerName" binding:"required"`
	RepoURL      string `json:"repoUrl" binding:"required"`
	AuthType     string `json:"authType"`
	AuthToken    string `json:"authToken"` // 仅当 AuthType=HTTP_TOKEN 时使用
}

// ImportZipLegacyRequest 旧版 ZIP 同步导入请求（仅创建 Provider 记录）
type ImportZipLegacyRequest struct {
	ProviderName string `json:"providerName" binding:"required"`
}

// ProviderSyncRequest Provider 同步触发请求
type ProviderSyncRequest struct {
	ProviderID string `json:"providerId" binding:"required"`
}

// ProviderDeleteRequest Provider 删除请求
type ProviderDeleteRequest struct {
	ProviderID string `json:"providerId" binding:"required"`
}

// ProviderListRequest Provider 列表查询请求
type ProviderListRequest struct {
	ProviderType string `json:"providerType"` // 占位字段，可选过滤
	Status       string `json:"status"`       // 占位字段，可选过滤
}

// ProviderDetailRequest Provider 详情请求
type ProviderDetailRequest struct {
	ProviderID string `json:"providerId" binding:"required"`
}

// ConvertImportGitRequest2DO 将 Git 导入请求转为 DO
func ConvertImportGitRequest2DO(req ImportGitRequest) models.SkillProviderDO {
	authType := req.AuthType
	if authType == "" {
		authType = models.AuthTypeNone
	}
	return models.SkillProviderDO{
		ProviderName: req.ProviderName,
		ProviderType: models.ProviderTypeGit,
		AuthType:     authType,
		RepoURL:      req.RepoURL,
		Status:       models.StatusActive,
	}
}

// ConvertImportZipLegacyRequest2DO 将旧版 ZIP 同步导入请求转为 DO
func ConvertImportZipLegacyRequest2DO(req ImportZipLegacyRequest) models.SkillProviderDO {
	return models.SkillProviderDO{
		ProviderName: req.ProviderName,
		ProviderType: models.ProviderTypeZip,
		AuthType:     models.AuthTypeNone,
		Status:       models.StatusActive,
	}
}

// ConvertSkillProviderDO2Info 将 DO 转为响应 DTO（skillCount 由 Service 层填充）
func ConvertSkillProviderDO2Info(do models.SkillProviderDO, skillCount int) SkillProviderInfo {
	return SkillProviderInfo{
		SkillProviderID: do.SkillProviderID,
		ProviderName:    do.ProviderName,
		ProviderType:    do.ProviderType,
		AuthType:        do.AuthType,
		RepoURL:         do.RepoURL,
		Status:          do.Status,
		SkillCount:      skillCount,
		Creator:         do.Creator,
		CreatedAt:       do.CreatedAt,
		UpdatedAt:       do.UpdatedAt,
	}
}
