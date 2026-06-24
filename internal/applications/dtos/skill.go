package dtos

import (
	"time"

	"mooc-manus/internal/domains/models"
)

// SkillInfo Skill 详情/列表/创建/更新/草稿/发布响应 DTO
type SkillInfo struct {
	SkillID         string             `json:"skillId"`
	SkillProviderID string             `json:"skillProviderId"`
	ProviderName    string             `json:"providerName,omitempty"`
	SkillName       string             `json:"skillName"`
	Description     string             `json:"description,omitempty"`
	Icon            *models.Icon       `json:"icon,omitempty"`
	ImageURL        string             `json:"imageUrl,omitempty"`
	Status          string             `json:"status"`
	LatestVersion   *SkillVersionInfo  `json:"latestVersion,omitempty"`
	VersionCount    int                `json:"versionCount"`
	Files           []models.SkillFile `json:"files,omitempty"`    // 详情接口：优先正式版本，否则 draft
	Versions        []SkillVersionInfo `json:"versions,omitempty"` // 详情接口：全部正式版本，倒序
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

// SkillPageDTO 分页响应（Skill 模块新增范式，仅用于 /skill/list）
type SkillPageDTO struct {
	Total    int64       `json:"total"`
	PageSize int         `json:"pageSize"`
	PageNum  int         `json:"pageNum"`
	Records  []SkillInfo `json:"records"`
}

// SkillWithVersionInfo /skill/with/version 接口的响应项
type SkillWithVersionInfo struct {
	SkillID         string             `json:"skillId"`
	SkillProviderID string             `json:"skillProviderId"`
	ProviderName    string             `json:"providerName,omitempty"`
	SkillName       string             `json:"skillName"`
	Description     string             `json:"description,omitempty"`
	Icon            *models.Icon       `json:"icon,omitempty"`
	ImageURL        string             `json:"imageUrl,omitempty"`
	Status          string             `json:"status"`
	VersionCount    int                `json:"versionCount"`
	Versions        []SkillVersionInfo `json:"versions"`
	Creator         string             `json:"creator,omitempty"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

// ============================================================
// 请求 DTO（9 个）
// 草稿暂存 / 发布走 multipart/form-data：以下结构供 Service 层使用，Handler 内由 PostForm 拼装
// ============================================================

// SkillDraftSaveRequest 草稿暂存请求（form-data 解析后填充）
type SkillDraftSaveRequest struct {
	SkillID     string                      // 为空时新建
	SkillName   string                      // 可为空，发布时按 SKILL.md 解析
	Description string                      // 可为空，发布时按 SKILL.md 解析
	Icon        string                      // 图标 JSON 字符串
	ImageURL    string                      // 自定义图片 URL
	SkillFiles  []models.SkillFileStructure // 文件结构列表（JSON 字符串解析后）
}

// SkillPublishRequest 发布请求（form-data 解析后填充）
type SkillPublishRequest struct {
	SkillID            string
	ProviderID         string
	SkillName          string
	Description        string
	VersionDescription string
	Icon               string
	ImageURL           string
	SkillFiles         []models.SkillFileStructure
}

// SkillUpdateRequest 编辑 Skill 基础元数据
type SkillUpdateRequest struct {
	SkillID     string `json:"skillId" binding:"required"`
	SkillName   string `json:"skillName"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// SkillDeleteRequest 删除 Skill
type SkillDeleteRequest struct {
	SkillID string `json:"skillId" binding:"required"`
}

// SkillListRequest 分页查询请求
type SkillListRequest struct {
	ProviderID string `json:"providerId"`
	SkillName  string `json:"skillName"`
	Keyword    string `json:"keyword"`
	Status     string `json:"status"`
	PageNum    int    `json:"pageNum"`
	PageSize   int    `json:"pageSize"`
}

// SkillListAllRequest 全量查询请求（与 SkillListRequest 同字段，仅忽略分页字段）
type SkillListAllRequest struct {
	ProviderID string `json:"providerId"`
	SkillName  string `json:"skillName"`
	Keyword    string `json:"keyword"`
	Status     string `json:"status"`
}

// SkillDetailRequest 详情请求
type SkillDetailRequest struct {
	SkillID string `json:"skillId" binding:"required"`
}

// SkillWithVersionRequest 编辑态选择器请求（无业务字段，使用空结构占位）
type SkillWithVersionRequest struct{}

// SkillFileDownloadQuery /file/download 的 query 参数
type SkillFileDownloadQuery struct {
	FileKey string `form:"fileKey" binding:"required"`
}

// ============================================================
// Convert 函数
// ============================================================

// ConvertSkillDO2Info 将 Skill DO 转为响应 DTO（VersionCount / LatestVersion / Versions / Files / ProviderName 由 Service 填充）
func ConvertSkillDO2Info(do models.SkillDO, providerName string) SkillInfo {
	return SkillInfo{
		SkillID:         do.SkillID,
		SkillProviderID: do.SkillProviderID,
		ProviderName:    providerName,
		SkillName:       do.SkillName,
		Description:     do.Description,
		Icon:            do.ExtInfo.Icon,
		ImageURL:        do.ExtInfo.ImageURL,
		Status:          do.Status,
		CreatedAt:       do.CreatedAt,
		UpdatedAt:       do.UpdatedAt,
	}
}

// ConvertSkillDO2WithVersion 将 Skill DO 转为编辑态选择器 DTO
func ConvertSkillDO2WithVersion(do models.SkillDO, providerName string, versions []SkillVersionInfo) SkillWithVersionInfo {
	return SkillWithVersionInfo{
		SkillID:         do.SkillID,
		SkillProviderID: do.SkillProviderID,
		ProviderName:    providerName,
		SkillName:       do.SkillName,
		Description:     do.Description,
		Icon:            do.ExtInfo.Icon,
		ImageURL:        do.ExtInfo.ImageURL,
		Status:          do.Status,
		VersionCount:    len(versions),
		Versions:        versions,
		Creator:         do.Creator,
		CreatedAt:       do.CreatedAt,
		UpdatedAt:       do.UpdatedAt,
	}
}

// ApplyDefaultPaging 给 SkillListRequest 应用默认分页与上限截断
func (r *SkillListRequest) ApplyDefaultPaging() {
	if r.PageNum <= 0 {
		r.PageNum = SkillListDefaultPageNum
	}
	if r.PageSize <= 0 {
		r.PageSize = SkillListDefaultPageSize
	}
	if r.PageSize > SkillListMaxPageSize {
		r.PageSize = SkillListMaxPageSize
	}
}
