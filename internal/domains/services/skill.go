package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/infra/external/file_storage"
	infra "mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/repositories"
	"mooc-manus/pkg/logger"
	"mooc-manus/pkg/skillerr"
	"mooc-manus/pkg/skillmd"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type SkillDomainService interface {
	DraftSave(req *dtos.SkillDraftSaveRequest, files []*multipart.FileHeader) (models.SkillDO, error)
	Publish(req *dtos.SkillPublishRequest, files []*multipart.FileHeader) (models.SkillDO, error)
	Update(req *dtos.SkillUpdateRequest) (models.SkillDO, error)
	Delete(skillId string) error
	List(req *dtos.SkillListRequest) ([]models.SkillDO, int64, error)
	ListAll(req *dtos.SkillListAllRequest) ([]models.SkillDO, error)
	Detail(skillId string) (models.SkillDO, error)
	WithVersion() ([]models.SkillDO, error)
	FileDownload(fileKey string) (io.ReadCloser, string, error)
	GetById(skillId string) (models.SkillDO, error)
	GetReleasedVersions(skillId string) ([]models.SkillVersionDO, error)
	GetLatestVersion(skillId string) (*models.SkillVersionDO, error)
	GetOrCreateCustomProvider() (string, error)
}

type SkillDomainServiceImpl struct {
	skillRepo    repositories.SkillRepository
	versionRepo  repositories.SkillVersionRepository
	providerRepo repositories.SkillProviderRepository
	storage      file_storage.FileStorage
}

func NewSkillDomainService(
	skillRepo repositories.SkillRepository,
	versionRepo repositories.SkillVersionRepository,
	providerRepo repositories.SkillProviderRepository,
	storage file_storage.FileStorage,
) SkillDomainService {
	return &SkillDomainServiceImpl{
		skillRepo:    skillRepo,
		versionRepo:  versionRepo,
		providerRepo: providerRepo,
		storage:      storage,
	}
}

// GetOrCreateCustomProvider 获取或创建 CUSTOM Provider；ZIP 导入也依赖此方法
func (s *SkillDomainServiceImpl) GetOrCreateCustomProvider() (string, error) {
	const customName = "custom"
	po, exists, err := s.providerRepo.GetByName(customName)
	if err != nil {
		return "", err
	}
	if exists {
		return po.ID, nil
	}
	newID := uuid.New().String()
	newPO := models.ConvertSkillProviderDO2PO(models.SkillProviderDO{
		SkillProviderID: newID,
		ProviderName:    customName,
		ProviderType:    models.ProviderTypeCustom,
		AuthType:        models.AuthTypeNone,
		Status:          models.StatusActive,
	})
	if err := s.providerRepo.Create(newPO); err != nil {
		return "", err
	}
	return newID, nil
}

// validateSkillFiles 业务规则 §5.3：skillFiles 必含 SKILL.md
func validateSkillFiles(files []models.SkillFileStructure) error {
	if len(files) == 0 {
		return fmt.Errorf("skillFiles is required: %w", skillerr.ErrInvalidInput)
	}
	for _, f := range files {
		if f.Name == "SKILL.md" {
			return nil
		}
	}
	return fmt.Errorf("skill.md not found in files: %w", skillerr.ErrInvalidInput)
}

// validateSkillName 业务规则 §4.5
func validateSkillName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("skill name is required: %w", skillerr.ErrInvalidInput)
	}
	if len(name) > dtos.SkillNameMaxLength {
		return fmt.Errorf("skill name too long (max %d): %w", dtos.SkillNameMaxLength, skillerr.ErrInvalidInput)
	}
	return nil
}

// validateSkillDesc 业务规则 §4.5
func validateSkillDesc(desc string) error {
	if strings.TrimSpace(desc) == "" {
		return fmt.Errorf("skill description is required: %w", skillerr.ErrInvalidInput)
	}
	if len(desc) > dtos.SkillDescMaxLength {
		return fmt.Errorf("skill description too long (max %d): %w", dtos.SkillDescMaxLength, skillerr.ErrInvalidInput)
	}
	return nil
}

// uploadFiles 上传文件到 OSS；返回完整的 SkillFile 列表
func (s *SkillDomainServiceImpl) uploadFiles(
	skillId, version string,
	fileHeaders []*multipart.FileHeader,
	fileStructures []models.SkillFileStructure,
) ([]models.SkillFile, error) {
	// name → header 映射
	headerMap := make(map[string]*multipart.FileHeader, len(fileHeaders))
	for _, fh := range fileHeaders {
		headerMap[fh.Filename] = fh
	}
	result := make([]models.SkillFile, 0, len(fileStructures))
	for _, fs := range fileStructures {
		fh, ok := headerMap[fs.Name]
		if !ok {
			continue // 未上传新内容的条目保留旧 fileKey（由调用方决定是否传入）
		}
		f, err := fh.Open()
		if err != nil {
			return nil, fmt.Errorf("open file %s failed: %w", fs.Name, err)
		}
		key := fmt.Sprintf(dtos.SkillFilePathTemplate, skillId, version, fs.Path)
		checksum, err := s.storage.PutObject(dtos.SkillBucketName, key, f, fh.Size, "application/octet-stream")
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("upload file %s failed: %w", fs.Name, err)
		}
		suffix := ""
		if idx := strings.LastIndexByte(fs.Name, '.'); idx >= 0 {
			suffix = fs.Name[idx+1:]
		}
		result = append(result, models.SkillFile{
			Path:     fs.Path,
			FileKey:  key,
			Suffix:   suffix,
			Size:     fh.Size,
			Checksum: checksum,
		})
	}
	return result, nil
}

// DraftSave 业务规则 §5.3
func (s *SkillDomainServiceImpl) DraftSave(req *dtos.SkillDraftSaveRequest, files []*multipart.FileHeader) (models.SkillDO, error) {
	if err := validateSkillFiles(req.SkillFiles); err != nil {
		return models.SkillDO{}, err
	}

	// 从上传文件中的 SKILL.md frontmatter 解析 name / description
	// 新建（skillId 为空）必需；更新可选，未上传时沿用 DB 既有值
	isNew := req.SkillID == ""
	md, parsed, err := skillmd.ExtractFromUploads(files, isNew)
	if err != nil {
		return models.SkillDO{}, err
	}
	if parsed {
		req.SkillName = md.Name
		req.Description = md.Description
	} else {
		existing, getErr := s.skillRepo.GetById(req.SkillID)
		if getErr != nil {
			if errors.Is(getErr, gorm.ErrRecordNotFound) {
				return models.SkillDO{}, fmt.Errorf("skill not found: %s: %w", req.SkillID, skillerr.ErrNotFound)
			}
			return models.SkillDO{}, getErr
		}
		req.SkillName = existing.SkillName
		req.Description = existing.Description
	}

	if err := validateSkillName(req.SkillName); err != nil {
		return models.SkillDO{}, err
	}
	if err := validateSkillDesc(req.Description); err != nil {
		return models.SkillDO{}, err
	}

	var skillDO models.SkillDO

	if isNew {
		providerID, err := s.GetOrCreateCustomProvider()
		if err != nil {
			return models.SkillDO{}, err
		}
		// 唯一性校验
		_, exists, err := s.skillRepo.GetByName(req.SkillName)
		if err != nil {
			return models.SkillDO{}, err
		}
		if exists {
			return models.SkillDO{}, fmt.Errorf("skill name duplicate: %s: %w", req.SkillName, skillerr.ErrDuplicate)
		}
		skillDO = models.SkillDO{
			SkillID:         uuid.New().String(),
			SkillName:       req.SkillName,
			Description:     req.Description,
			SkillProviderID: providerID,
			Status:          models.StatusActive,
		}
	} else {
		po, err := s.skillRepo.GetById(req.SkillID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return models.SkillDO{}, fmt.Errorf("skill not found: %s: %w", req.SkillID, skillerr.ErrNotFound)
			}
			return models.SkillDO{}, err
		}
		skillDO = models.ConvertSkillPO2DO(po)
		skillDO.SkillName = req.SkillName
		skillDO.Description = req.Description
	}

	// 解析 icon
	if req.Icon != "" {
		var icon models.Icon
		if err := json.Unmarshal([]byte(req.Icon), &icon); err == nil {
			skillDO.ExtInfo.Icon = &icon
		}
	}
	skillDO.ExtInfo.ImageURL = req.ImageURL

	// 上传文件
	uploadedFiles, err := s.uploadFiles(skillDO.SkillID, models.SkillDraftVersionString, files, req.SkillFiles)
	if err != nil {
		return models.SkillDO{}, err
	}

	err = s.skillRepo.Transaction(func(txSkill repositories.SkillRepository, txVersion repositories.SkillVersionRepository, _ repositories.SkillProviderRepository) error {
		// 保存/更新 Skill
		skillPO := models.ConvertSkillDO2PO(skillDO)
		if isNew {
			if err := txSkill.Create(skillPO); err != nil {
				return err
			}
		} else {
			skillPO.LatestVersionID = "" // 草稿覆盖后清空最新版本（需重新发布）
			if err := txSkill.Update(skillPO); err != nil {
				return err
			}
		}
		// 保存 draft 版本
		_, exists, err := txVersion.GetDraftBySkillID(skillDO.SkillID)
		if err != nil {
			return err
		}
		draftDO := models.SkillVersionDO{
			SkillID:    skillDO.SkillID,
			Version:    models.SkillDraftVersionString,
			SkillFiles: uploadedFiles,
		}
		if parsed {
			draftDO.Metadata = models.SkillMetadata{Name: md.Name, Description: md.Description}
		}
		draftPO := models.ConvertSkillVersionDO2PO(draftDO)
		if exists {
			return txVersion.Update(draftPO)
		}
		return txVersion.Create(draftPO)
	})
	if err != nil {
		return models.SkillDO{}, err
	}
	return skillDO, nil
}

// Publish 业务规则 §5.4
func (s *SkillDomainServiceImpl) Publish(req *dtos.SkillPublishRequest, files []*multipart.FileHeader) (models.SkillDO, error) {
	// 1. 先执行 DraftSave 流程
	draftReq := &dtos.SkillDraftSaveRequest{
		SkillID:     req.SkillID,
		SkillName:   req.SkillName,
		Description: req.Description,
		Icon:        req.Icon,
		ImageURL:    req.ImageURL,
		SkillFiles:  req.SkillFiles,
	}
	skillDO, err := s.DraftSave(draftReq, files)
	if err != nil {
		return models.SkillDO{}, err
	}

	// 2. 生成新版本号
	versionStrs, err := s.versionRepo.ListReleasedVersionStrings(skillDO.SkillID)
	if err != nil {
		return models.SkillDO{}, err
	}
	newVersion := models.NextPatchVersion(models.MaxVersion(versionStrs))

	// 3. 读取 draft 文件列表
	draftPO, _, err := s.versionRepo.GetDraftBySkillID(skillDO.SkillID)
	if err != nil {
		return models.SkillDO{}, err
	}
	draftDO := models.ConvertSkillVersionPO2DO(draftPO)

	// 4. 把 draft 文件复制到新版本目录，重写 fileKey
	newFiles := make([]models.SkillFile, 0, len(draftDO.SkillFiles))
	for _, f := range draftDO.SkillFiles {
		newKey := fmt.Sprintf(dtos.SkillFilePathTemplate, skillDO.SkillID, newVersion, f.Path)
		if err := s.storage.CopyObject(dtos.SkillBucketName, f.FileKey, dtos.SkillBucketName, newKey); err != nil {
			return models.SkillDO{}, fmt.Errorf("publish failed: copy file %s: %w", f.FileKey, err)
		}
		newFile := f
		newFile.FileKey = newKey
		newFiles = append(newFiles, newFile)
	}

	// 5. 事务：保存正式版本 + 快照 + 更新 latest_version_id
	versionDesc := req.VersionDescription
	if versionDesc == "" {
		versionDesc = skillDO.Description
	}
	newVersionDO := models.SkillVersionDO{
		SkillID:     skillDO.SkillID,
		Version:     newVersion,
		Description: versionDesc,
		Metadata:    draftDO.Metadata,
		SkillFiles:  newFiles,
		Creator:     skillDO.Creator,
		ExtInfo: models.SkillVersionExtInfo{
			SnapshotSkillName: skillDO.SkillName,
			SnapshotIcon:      skillDO.ExtInfo.Icon,
			SnapshotImageURL:  skillDO.ExtInfo.ImageURL,
		},
	}
	newVersionPO := models.ConvertSkillVersionDO2PO(newVersionDO)

	err = s.skillRepo.Transaction(func(txSkill repositories.SkillRepository, txVersion repositories.SkillVersionRepository, _ repositories.SkillProviderRepository) error {
		if err := txVersion.Create(newVersionPO); err != nil {
			return err
		}
		return txSkill.UpdateLatestVersionId(skillDO.SkillID, newVersionPO.ID)
	})
	if err != nil {
		return models.SkillDO{}, err
	}
	skillDO.LatestVersionID = newVersionPO.ID

	// 6. 异步打包 ZIP（失败不阻塞发布）
	go func() {
		zipKey := fmt.Sprintf(dtos.SkillZipPublishTpl, skillDO.SkillID, newVersion, skillDO.SkillID, newVersion)
		if err := buildAndUploadZip(s.storage, newFiles, dtos.SkillBucketName, zipKey); err != nil {
			logger.Warn("publish zip failed", zap.Error(err))
		}
	}()

	return skillDO, nil
}

// Update 业务规则 §5.6：仅更新基础元数据，不触发版本变更
func (s *SkillDomainServiceImpl) Update(req *dtos.SkillUpdateRequest) (models.SkillDO, error) {
	if req.SkillID == "" {
		return models.SkillDO{}, fmt.Errorf("skillId is required: %w", skillerr.ErrInvalidInput)
	}
	po, err := s.skillRepo.GetById(req.SkillID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.SkillDO{}, fmt.Errorf("skill not found: %s: %w", req.SkillID, skillerr.ErrNotFound)
		}
		return models.SkillDO{}, err
	}
	do := models.ConvertSkillPO2DO(po)
	if req.SkillName != "" {
		do.SkillName = req.SkillName
	}
	if req.Description != "" {
		do.Description = req.Description
	}
	if req.Status != "" {
		do.Status = req.Status
	}
	return do, s.skillRepo.Update(models.ConvertSkillDO2PO(do))
}

// Delete 业务规则 §5.7：事务内删除版本+Skill，事务外批量删除 OSS 文件
func (s *SkillDomainServiceImpl) Delete(skillId string) error {
	if skillId == "" {
		return fmt.Errorf("skillId is required: %w", skillerr.ErrInvalidInput)
	}
	// 收集全部文件 key
	versionPOs, err := s.versionRepo.ListBySkillID(skillId)
	if err != nil {
		return err
	}
	var fileKeys []string
	for _, vpo := range versionPOs {
		do := models.ConvertSkillVersionPO2DO(vpo)
		for _, f := range do.SkillFiles {
			if f.FileKey != "" {
				fileKeys = append(fileKeys, f.FileKey)
			}
		}
	}
	// 事务：删除版本 + Skill
	err = s.skillRepo.Transaction(func(txSkill repositories.SkillRepository, txVersion repositories.SkillVersionRepository, _ repositories.SkillProviderRepository) error {
		if err := txVersion.DeleteBySkillID(skillId); err != nil {
			return err
		}
		return txSkill.DeleteById(skillId)
	})
	if err != nil {
		return err
	}
	// 事务外删除 OSS 文件（尽力而为）
	if len(fileKeys) > 0 {
		if err := s.storage.RemoveObjects(dtos.SkillBucketName, fileKeys); err != nil {
			logger.Warn("delete oss files failed", zap.Error(err))
		}
	}
	return nil
}

func (s *SkillDomainServiceImpl) List(req *dtos.SkillListRequest) ([]models.SkillDO, int64, error) {
	req.ApplyDefaultPaging()
	nameLike := truncate(req.SkillName, dtos.SkillNameLikeMaxLength)
	if nameLike == "" {
		nameLike = truncate(req.Keyword, dtos.SkillNameLikeMaxLength)
	}
	filter := repositories.SkillListFilter{
		ProviderID: req.ProviderID,
		NameLike:   nameLike,
		Status:     req.Status,
	}
	pos, total, err := s.skillRepo.Page(filter, req.PageNum, req.PageSize)
	if err != nil {
		return nil, 0, err
	}
	return convertSkillPOs(pos), total, nil
}

func (s *SkillDomainServiceImpl) ListAll(req *dtos.SkillListAllRequest) ([]models.SkillDO, error) {
	nameLike := truncate(req.SkillName, dtos.SkillNameLikeMaxLength)
	if nameLike == "" {
		nameLike = truncate(req.Keyword, dtos.SkillNameLikeMaxLength)
	}
	filter := repositories.SkillListFilter{
		ProviderID: req.ProviderID,
		NameLike:   nameLike,
		Status:     req.Status,
	}
	pos, err := s.skillRepo.List(filter)
	if err != nil {
		return nil, err
	}
	return convertSkillPOs(pos), nil
}

func (s *SkillDomainServiceImpl) Detail(skillId string) (models.SkillDO, error) {
	return s.GetById(skillId)
}

// WithVersion 业务规则 §5.10：仅返回有至少一个正式版本的 Skill
func (s *SkillDomainServiceImpl) WithVersion() ([]models.SkillDO, error) {
	all, err := s.skillRepo.List(repositories.SkillListFilter{})
	if err != nil {
		return nil, err
	}
	var result []models.SkillDO
	for _, po := range all {
		versions, err := s.versionRepo.ListReleasedBySkillID(po.ID)
		if err != nil {
			return nil, err
		}
		if len(versions) > 0 {
			result = append(result, models.ConvertSkillPO2DO(po))
		}
	}
	return result, nil
}

// FileDownload 业务规则 §5.11：按 fileKey 下载；返回 (ReadCloser, filename, error)
func (s *SkillDomainServiceImpl) FileDownload(fileKey string) (io.ReadCloser, string, error) {
	if fileKey == "" {
		return nil, "", fmt.Errorf("fileKey is required: %w", skillerr.ErrInvalidInput)
	}
	exists, err := s.storage.Exists(dtos.SkillBucketName, fileKey)
	if err != nil {
		return nil, "", err
	}
	if !exists {
		return nil, "", fmt.Errorf("skill file missing: %s: %w", fileKey, skillerr.ErrNotFound)
	}
	rc, err := s.storage.GetObject(dtos.SkillBucketName, fileKey)
	if err != nil {
		return nil, "", err
	}
	filename := fileKey
	if idx := strings.LastIndexByte(fileKey, '/'); idx >= 0 {
		filename = fileKey[idx+1:]
	}
	return rc, filename, nil
}

func (s *SkillDomainServiceImpl) GetById(skillId string) (models.SkillDO, error) {
	po, err := s.skillRepo.GetById(skillId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.SkillDO{}, fmt.Errorf("skill not found: %s: %w", skillId, skillerr.ErrNotFound)
		}
		return models.SkillDO{}, err
	}
	return models.ConvertSkillPO2DO(po), nil
}

func (s *SkillDomainServiceImpl) GetReleasedVersions(skillId string) ([]models.SkillVersionDO, error) {
	pos, err := s.versionRepo.ListReleasedBySkillID(skillId)
	if err != nil {
		return nil, err
	}
	dos := make([]models.SkillVersionDO, 0, len(pos))
	for _, po := range pos {
		dos = append(dos, models.ConvertSkillVersionPO2DO(po))
	}
	return dos, nil
}

func (s *SkillDomainServiceImpl) GetLatestVersion(skillId string) (*models.SkillVersionDO, error) {
	skillPO, err := s.skillRepo.GetById(skillId)
	if err != nil || skillPO.LatestVersionID == "" {
		return nil, err
	}
	po, err := s.versionRepo.GetById(skillPO.LatestVersionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	do := models.ConvertSkillVersionPO2DO(po)
	return &do, nil
}

func convertSkillPOs(pos []infra.SkillPO) []models.SkillDO {
	dos := make([]models.SkillDO, 0, len(pos))
	for _, po := range pos {
		dos = append(dos, models.ConvertSkillPO2DO(po))
	}
	return dos
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
