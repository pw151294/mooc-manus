package dtos

import (
	"time"

	"mooc-manus/internal/domains/models"
)

// SkillVersionInfo 版本详情/列表响应 DTO
type SkillVersionInfo struct {
	SkillVersionID string             `json:"skillVersionId"`
	SkillID        string             `json:"skillId"`
	SkillName      string             `json:"skillName,omitempty"`
	Version        string             `json:"version"`
	Description    string             `json:"description,omitempty"`
	Metadata       models.SkillMetadata `json:"metadata,omitempty"`
	SkillFiles     []models.SkillFile `json:"skillFiles,omitempty"`
	ZipFilePath    string             `json:"zipFilePath,omitempty"`
	Creator        string             `json:"creator,omitempty"`
	PublishedBy    string             `json:"publishedBy,omitempty"` // 兼容文档字段，等价于 Creator
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

// VersionCreateRequest 程序化创建版本
type VersionCreateRequest struct {
	SkillID string `json:"skillId" binding:"required"`
	Version string `json:"version" binding:"required"` // 必填，调用方决定版本号
}

// VersionValidateRequest 标记某版本为最新版本
type VersionValidateRequest struct {
	VersionID  string                 `json:"versionId" binding:"required"`
	TestInputs map[string]interface{} `json:"testInputs,omitempty"` // 预留，当前实现不消费
}

// VersionDeleteRequest 删除指定版本
type VersionDeleteRequest struct {
	SkillID string `json:"skillId" binding:"required"`
	Version string `json:"version" binding:"required"`
}

// VersionListRequest 列出某 Skill 的全部正式版本
type VersionListRequest struct {
	SkillID string `json:"skillId" binding:"required"`
}

// VersionDetailRequest 版本详情
type VersionDetailRequest struct {
	SkillID string `json:"skillId" binding:"required"`
	Version string `json:"version" binding:"required"`
}

// VersionLatestRequest 获取最新已发布版本
type VersionLatestRequest struct {
	SkillID string `json:"skillId" binding:"required"`
}

// VersionRollbackRequest 回滚到目标版本（生成一个新版本继承其内容）
type VersionRollbackRequest struct {
	SkillID        string `json:"skillId" binding:"required"`
	TargetVersion  string `json:"targetVersion" binding:"required"`
	PubVersionType string `json:"pubVersionType,omitempty"` // 占位字段，当前固定按 patch 递增
}

// VersionExportRequest 导出版本为 ZIP
type VersionExportRequest struct {
	SkillID string `json:"skillId" binding:"required"`
	Version string `json:"version" binding:"required"`
}

// ConvertSkillVersionDO2Info 将版本 DO 转为响应 DTO（skillName 由 Service 层填充）
func ConvertSkillVersionDO2Info(do models.SkillVersionDO, skillName string) SkillVersionInfo {
	return SkillVersionInfo{
		SkillVersionID: do.SkillVersionID,
		SkillID:        do.SkillID,
		SkillName:      skillName,
		Version:        do.Version,
		Description:    do.Description,
		Metadata:       do.Metadata,
		SkillFiles:     do.SkillFiles,
		ZipFilePath:    do.ExtInfo.ZipFilePath,
		Creator:        do.Creator,
		PublishedBy:    do.Creator,
		CreatedAt:      do.CreatedAt,
		UpdatedAt:      do.UpdatedAt,
	}
}
