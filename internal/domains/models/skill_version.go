package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	infra "mooc-manus/internal/infra/models"

	"github.com/google/uuid"
)

// SkillVersionDO Skill 版本实体（属于 Skill 聚合）
type SkillVersionDO struct {
	SkillVersionID string
	SkillID        string
	Version        string // 'draft' 或 'vMAJOR.MINOR.PATCH'
	Description    string
	Metadata       SkillMetadata
	SkillFiles     []SkillFile
	ExtInfo        SkillVersionExtInfo
	Creator        string
	Updator        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SkillFile 单个文件元数据值对象
type SkillFile struct {
	Path     string `json:"path"`     // 相对路径
	FileKey  string `json:"fileKey"`  // OSS Key
	Suffix   string `json:"suffix"`   // 扩展名
	Size     int64  `json:"size"`     // 字节
	Checksum string `json:"checksum"` // MD5 / SHA256
}

// SkillFileStructure 前端传入的文件结构条目（仅 DTO 解析阶段使用，DO 层定义类型供 Service 引用）
type SkillFileStructure struct {
	Type string `json:"type"` // file / directory
	Path string `json:"path"`
	Name string `json:"name"`
}

// SkillMetadata SKILL.md 解析后的元数据
type SkillMetadata struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
	Version     string `json:"version,omitempty"`
	// 其他字段开放扩展
	Extra map[string]interface{} `json:"-"`
}

// SkillVersionExtInfo Skill 版本表的 ext_info JSON 结构
type SkillVersionExtInfo struct {
	ZipFilePath       string `json:"zipFilePath,omitempty"`
	SnapshotSkillName string `json:"snapshotSkillName,omitempty"`
	SnapshotIcon      *Icon  `json:"snapshotIcon,omitempty"`
	SnapshotImageURL  string `json:"snapshotImageUrl,omitempty"`
}

// IsDraft 判断是否草稿版本
func (v *SkillVersionDO) IsDraft() bool {
	return v.Version == SkillDraftVersionString
}

// ParseVersion 把 'vMAJOR.MINOR.PATCH' 解析为 (major, minor, patch)；非语义化版本返回 ok=false
func ParseVersion(version string) (major, minor, patch int, ok bool) {
	if !strings.HasPrefix(version, "v") {
		return 0, 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	var err error
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, 0, false
	}
	if minor, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, 0, false
	}
	if patch, err = strconv.Atoi(parts[2]); err != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

// FormatVersion 把 (major, minor, patch) 拼成 'vMAJOR.MINOR.PATCH'
func FormatVersion(major, minor, patch int) string {
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch)
}

// NextPatchVersion 在最新已发布版本基础上 +1 patch；historyLatest 为空 / 解析失败时回退 SkillInitialVersion
func NextPatchVersion(historyLatest string) string {
	if historyLatest == "" {
		return SkillInitialVersion
	}
	major, minor, patch, ok := ParseVersion(historyLatest)
	if !ok {
		return SkillInitialVersion
	}
	return FormatVersion(major, minor, patch+1)
}

// MaxVersion 在一组已发布版本号中按 (major, minor, patch) 字典序求最大；空集返回 ""
func MaxVersion(versions []string) string {
	var (
		bestStr                              string
		bestMajor, bestMinor, bestPatch, set = 0, 0, 0, false
	)
	for _, v := range versions {
		major, minor, patch, ok := ParseVersion(v)
		if !ok {
			continue
		}
		if !set ||
			major > bestMajor ||
			(major == bestMajor && minor > bestMinor) ||
			(major == bestMajor && minor == bestMinor && patch > bestPatch) {
			bestMajor, bestMinor, bestPatch = major, minor, patch
			bestStr = v
			set = true
		}
	}
	return bestStr
}

func ConvertSkillVersionDO2PO(do SkillVersionDO) infra.SkillVersionPO {
	if do.SkillVersionID == "" {
		do.SkillVersionID = uuid.New().String()
	}
	metaBytes, _ := json.Marshal(do.Metadata)
	filesBytes, _ := json.Marshal(do.SkillFiles)
	extBytes, _ := json.Marshal(do.ExtInfo)
	return infra.SkillVersionPO{
		ID:          do.SkillVersionID,
		SkillID:     do.SkillID,
		Version:     do.Version,
		Description: do.Description,
		Metadata:    string(metaBytes),
		SkillFiles:  string(filesBytes),
		ExtInfo:     string(extBytes),
		Creator:     do.Creator,
		Updator:     do.Updator,
	}
}

func ConvertSkillVersionPO2DO(po infra.SkillVersionPO) SkillVersionDO {
	var meta SkillMetadata
	if po.Metadata != "" {
		_ = json.Unmarshal([]byte(po.Metadata), &meta)
	}
	var files []SkillFile
	if po.SkillFiles != "" {
		_ = json.Unmarshal([]byte(po.SkillFiles), &files)
	}
	var ext SkillVersionExtInfo
	if po.ExtInfo != "" {
		_ = json.Unmarshal([]byte(po.ExtInfo), &ext)
	}
	return SkillVersionDO{
		SkillVersionID: po.ID,
		SkillID:        po.SkillID,
		Version:        po.Version,
		Description:    po.Description,
		Metadata:       meta,
		SkillFiles:     files,
		ExtInfo:        ext,
		Creator:        po.Creator,
		Updator:        po.Updator,
		CreatedAt:      po.CreatedAt,
		UpdatedAt:      po.UpdatedAt,
	}
}
