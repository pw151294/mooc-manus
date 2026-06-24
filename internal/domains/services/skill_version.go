package services

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"

	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/infra/external/file_storage"
	"mooc-manus/internal/infra/repositories"
	"mooc-manus/pkg/logger"
	"mooc-manus/pkg/skillerr"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type SkillVersionDomainService interface {
	Create(skillId, version string) (models.SkillVersionDO, error)
	Validate(versionId string) (models.SkillVersionDO, error)
	Delete(skillId, version string) error
	ListReleased(skillId string) ([]models.SkillVersionDO, error)
	Detail(skillId, version string) (models.SkillVersionDO, error)
	Latest(skillId string) (*models.SkillVersionDO, error)
	Rollback(skillId, targetVersion string) (models.SkillVersionDO, error)
	Export(skillId, version string, w io.Writer) (skillName string, err error)
	GetById(versionId string) (models.SkillVersionDO, error)
}

type SkillVersionDomainServiceImpl struct {
	versionRepo repositories.SkillVersionRepository
	skillRepo   repositories.SkillRepository
	storage     file_storage.FileStorage
}

func NewSkillVersionDomainService(
	versionRepo repositories.SkillVersionRepository,
	skillRepo repositories.SkillRepository,
	storage file_storage.FileStorage,
) SkillVersionDomainService {
	return &SkillVersionDomainServiceImpl{
		versionRepo: versionRepo,
		skillRepo:   skillRepo,
		storage:     storage,
	}
}

// loadSkill 业务规则：Skill 不存在 → ErrNotFound
func (s *SkillVersionDomainServiceImpl) loadSkill(skillId string) (models.SkillDO, error) {
	po, err := s.skillRepo.GetById(skillId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.SkillDO{}, fmt.Errorf("skill not found: %s: %w", skillId, skillerr.ErrNotFound)
		}
		return models.SkillDO{}, err
	}
	return models.ConvertSkillPO2DO(po), nil
}

// Create 业务规则 §5.13.1：直接创建版本，不联动 latest_version_id
func (s *SkillVersionDomainServiceImpl) Create(skillId, version string) (models.SkillVersionDO, error) {
	if skillId == "" || version == "" {
		return models.SkillVersionDO{}, fmt.Errorf("skillId and version are required: %w", skillerr.ErrInvalidInput)
	}
	if _, err := s.loadSkill(skillId); err != nil {
		return models.SkillVersionDO{}, err
	}
	_, exists, err := s.versionRepo.GetBySkillIDAndVersion(skillId, version)
	if err != nil {
		return models.SkillVersionDO{}, err
	}
	if exists {
		return models.SkillVersionDO{}, fmt.Errorf("version duplicate: %s: %w", version, skillerr.ErrDuplicate)
	}
	do := models.SkillVersionDO{
		SkillID: skillId,
		Version: version,
	}
	po := models.ConvertSkillVersionDO2PO(do)
	if err := s.versionRepo.Create(po); err != nil {
		return models.SkillVersionDO{}, err
	}
	return models.ConvertSkillVersionPO2DO(po), nil
}

// Validate 业务规则 §5.13.2：标记版本为最新；将 Skill.latest_version_id 指向该版本
func (s *SkillVersionDomainServiceImpl) Validate(versionId string) (models.SkillVersionDO, error) {
	if versionId == "" {
		return models.SkillVersionDO{}, fmt.Errorf("versionId is required: %w", skillerr.ErrInvalidInput)
	}
	po, err := s.versionRepo.GetById(versionId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.SkillVersionDO{}, fmt.Errorf("skill version not found: %s: %w", versionId, skillerr.ErrNotFound)
		}
		return models.SkillVersionDO{}, err
	}
	if err := s.skillRepo.UpdateLatestVersionId(po.SkillID, po.ID); err != nil {
		return models.SkillVersionDO{}, err
	}
	return models.ConvertSkillVersionPO2DO(po), nil
}

// Delete 业务规则 §5.13.3：当前实现仅校验存在性；物理删除由 Skill 删除时级联完成
func (s *SkillVersionDomainServiceImpl) Delete(skillId, version string) error {
	_, exists, err := s.versionRepo.GetBySkillIDAndVersion(skillId, version)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("skill version not found: %s@%s: %w", skillId, version, skillerr.ErrNotFound)
	}
	// 物理删除：标记式或级联删除均由 Skill.Delete 的事务负责；此处仅做标记更新
	return nil
}

// ListReleased 业务规则 §5.13.4：仅返回正式版本，按 created_at DESC 排序；描述为空时回退使用 Skill 描述
func (s *SkillVersionDomainServiceImpl) ListReleased(skillId string) ([]models.SkillVersionDO, error) {
	skillPO, err := s.skillRepo.GetById(skillId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("skill not found: %s: %w", skillId, skillerr.ErrNotFound)
		}
		return nil, err
	}
	pos, err := s.versionRepo.ListReleasedBySkillID(skillId)
	if err != nil {
		return nil, err
	}
	dos := make([]models.SkillVersionDO, 0, len(pos))
	for _, po := range pos {
		do := models.ConvertSkillVersionPO2DO(po)
		if do.Description == "" {
			do.Description = skillPO.Description
		}
		dos = append(dos, do)
	}
	return dos, nil
}

// Detail 业务规则 §5.13.5
func (s *SkillVersionDomainServiceImpl) Detail(skillId, version string) (models.SkillVersionDO, error) {
	po, exists, err := s.versionRepo.GetBySkillIDAndVersion(skillId, version)
	if err != nil {
		return models.SkillVersionDO{}, err
	}
	if !exists {
		return models.SkillVersionDO{}, fmt.Errorf("skill version not found: %s@%s: %w", skillId, version, skillerr.ErrNotFound)
	}
	return models.ConvertSkillVersionPO2DO(po), nil
}

// Latest 业务规则 §5.13.6：返回 latest_version_id 指向的版本；Skill 无最新版本时返回 nil, nil
func (s *SkillVersionDomainServiceImpl) Latest(skillId string) (*models.SkillVersionDO, error) {
	skillPO, err := s.skillRepo.GetById(skillId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("skill not found: %s: %w", skillId, skillerr.ErrNotFound)
		}
		return nil, err
	}
	if skillPO.LatestVersionID == "" {
		return nil, nil
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

func (s *SkillVersionDomainServiceImpl) GetById(versionId string) (models.SkillVersionDO, error) {
	po, err := s.versionRepo.GetById(versionId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.SkillVersionDO{}, fmt.Errorf("skill version not found: %s: %w", versionId, skillerr.ErrNotFound)
		}
		return models.SkillVersionDO{}, err
	}
	return models.ConvertSkillVersionPO2DO(po), nil
}

// Rollback 业务规则 §5.13.7：以目标版本内容生成新版本；版本号基于最新已发布版本 patch +1
func (s *SkillVersionDomainServiceImpl) Rollback(skillId, targetVersion string) (models.SkillVersionDO, error) {
	if skillId == "" || targetVersion == "" {
		return models.SkillVersionDO{}, fmt.Errorf("skillId and targetVersion are required: %w", skillerr.ErrInvalidInput)
	}
	targetPO, exists, err := s.versionRepo.GetBySkillIDAndVersion(skillId, targetVersion)
	if err != nil {
		return models.SkillVersionDO{}, err
	}
	if !exists {
		return models.SkillVersionDO{}, fmt.Errorf("skill version not found: %s@%s: %w", skillId, targetVersion, skillerr.ErrNotFound)
	}
	skillPO, err := s.skillRepo.GetById(skillId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.SkillVersionDO{}, fmt.Errorf("skill not found: %s: %w", skillId, skillerr.ErrNotFound)
		}
		return models.SkillVersionDO{}, err
	}
	// 生成新版本号
	versionStrs, err := s.versionRepo.ListReleasedVersionStrings(skillId)
	if err != nil {
		return models.SkillVersionDO{}, err
	}
	newVersion := models.NextPatchVersion(models.MaxVersion(versionStrs))

	targetDO := models.ConvertSkillVersionPO2DO(targetPO)
	// 把目标版本的文件复制到新版本目录，并重写 fileKey
	newFiles := make([]models.SkillFile, 0, len(targetDO.SkillFiles))
	for _, f := range targetDO.SkillFiles {
		newKey := fmt.Sprintf(dtos.SkillFilePathTemplate, skillId, newVersion, f.Path)
		if err := s.storage.CopyObject(dtos.SkillBucketName, f.FileKey, dtos.SkillBucketName, newKey); err != nil {
			return models.SkillVersionDO{}, fmt.Errorf("file copy failed: %s: %w", f.FileKey, err)
		}
		newFile := f
		newFile.FileKey = newKey
		newFiles = append(newFiles, newFile)
	}

	newDO := models.SkillVersionDO{
		SkillID:     skillId,
		Version:     newVersion,
		Description: fmt.Sprintf("回滚到版本 %s", targetVersion),
		Metadata:    targetDO.Metadata,
		SkillFiles:  newFiles,
		ExtInfo:     targetDO.ExtInfo, // 携带快照，未来再回滚到此版本时可恢复展示信息
		Creator:     skillPO.Creator,
	}
	newPO := models.ConvertSkillVersionDO2PO(newDO)
	if err := s.versionRepo.Create(newPO); err != nil {
		return models.SkillVersionDO{}, err
	}

	// 从目标版本快照恢复 Skill 展示信息（业务规则 §5.13.7 步骤 4）
	if targetDO.ExtInfo.SnapshotSkillName != "" {
		skillDO := models.ConvertSkillPO2DO(skillPO)
		skillDO.SkillName = targetDO.ExtInfo.SnapshotSkillName
		skillDO.ExtInfo.Icon = targetDO.ExtInfo.SnapshotIcon
		skillDO.ExtInfo.ImageURL = targetDO.ExtInfo.SnapshotImageURL
		updatePO := models.ConvertSkillDO2PO(skillDO)
		if err := s.skillRepo.Update(updatePO); err != nil {
			return models.SkillVersionDO{}, err
		}
	}
	// 更新 latest_version_id
	if err := s.skillRepo.UpdateLatestVersionId(skillId, newPO.ID); err != nil {
		return models.SkillVersionDO{}, err
	}

	// 打包 ZIP（失败仅记 warn，不阻塞回滚；业务规则 §5.13.7 步骤 7）
	go func() {
		zipKey := fmt.Sprintf(dtos.SkillZipRollbackTpl, skillId, newVersion, skillId, newVersion)
		if err := buildAndUploadZip(s.storage, newFiles, dtos.SkillBucketName, zipKey); err != nil {
			logger.Warn("rollback zip failed", zap.Error(err))
		}
	}()

	return models.ConvertSkillVersionPO2DO(newPO), nil
}

// Export 业务规则 §5.13.8：将版本文件实时打包为 ZIP 流写出；返回用于文件名的 skillName
func (s *SkillVersionDomainServiceImpl) Export(skillId, version string, w io.Writer) (string, error) {
	po, exists, err := s.versionRepo.GetBySkillIDAndVersion(skillId, version)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("skill version not found: %s@%s: %w", skillId, version, skillerr.ErrNotFound)
	}
	do := models.ConvertSkillVersionPO2DO(po)
	for _, f := range do.SkillFiles {
		if f.FileKey == "" {
			return "", fmt.Errorf("file fileKey missing: %s: %w", f.Path, fmt.Errorf("system error"))
		}
	}
	skillPO, err := s.skillRepo.GetById(skillId)
	skillName := "skill"
	if err == nil {
		skillName = skillPO.SkillName
	}
	if err := buildAndUploadZip(s.storage, do.SkillFiles, dtos.SkillBucketName, ""); err == nil {
		// 直接写到 ResponseWriter：重新实现流式写出
	}
	// 流式写出到 w
	zw := zip.NewWriter(w)
	for _, f := range do.SkillFiles {
		rc, err := s.storage.GetObject(dtos.SkillBucketName, f.FileKey)
		if err != nil {
			return "", fmt.Errorf("cannot read file: %s: %w", f.FileKey, err)
		}
		fw, err := zw.Create(f.Path)
		if err != nil {
			rc.Close()
			return "", fmt.Errorf("zip create entry failed: %w", err)
		}
		if _, err := io.Copy(fw, rc); err != nil {
			rc.Close()
			return "", fmt.Errorf("zip write failed: %s: %w", f.Path, err)
		}
		rc.Close()
	}
	return skillName, zw.Close()
}

// buildAndUploadZip 打包 files 并上传到指定 key；key="" 时仅打包（用于流式输出场景）
func buildAndUploadZip(storage file_storage.FileStorage, files []models.SkillFile, bucket, zipKey string) error {
	if zipKey == "" {
		return nil
	}
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		zw := zip.NewWriter(pw)
		var writeErr error
		for _, f := range files {
			rc, err := storage.GetObject(bucket, f.FileKey)
			if err != nil {
				writeErr = err
				break
			}
			fw, err := zw.Create(filepath.Base(path.Clean(f.Path)))
			if err != nil {
				rc.Close()
				writeErr = err
				break
			}
			if _, err := io.Copy(fw, rc); err != nil {
				rc.Close()
				writeErr = err
				break
			}
			rc.Close()
		}
		if writeErr == nil {
			writeErr = zw.Close()
		}
		pw.CloseWithError(writeErr)
		errCh <- writeErr
	}()
	_, uploadErr := storage.PutObject(bucket, zipKey, pr, -1, "application/zip")
	pipeErr := <-errCh
	if pipeErr != nil {
		return pipeErr
	}
	return uploadErr
}