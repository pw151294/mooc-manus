package services

import (
	"errors"
	"fmt"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/infra/repositories"
	"mooc-manus/pkg/skillerr"

	"gorm.io/gorm"
)

type SkillProviderDomainService interface {
	Create(do models.SkillProviderDO) (string, error)
	Update(do models.SkillProviderDO) error
	GetById(id string) (models.SkillProviderDO, error)
	GetByIds(ids []string) (map[string]models.SkillProviderDO, error)
	List(providerType, status string) ([]models.SkillProviderDO, error)
	DeleteById(id string) error
	Sync(id string) (models.SkillProviderDO, error)
	CountSkillsByProviderId(providerId string) (int64, error)
}

type SkillProviderDomainServiceImpl struct {
	providerRepo repositories.SkillProviderRepository
	skillRepo    repositories.SkillRepository
}

func NewSkillProviderDomainService(
	providerRepo repositories.SkillProviderRepository,
	skillRepo repositories.SkillRepository,
) SkillProviderDomainService {
	return &SkillProviderDomainServiceImpl{
		providerRepo: providerRepo,
		skillRepo:    skillRepo,
	}
}

// Create 业务规则：provider_name 全局唯一
func (s *SkillProviderDomainServiceImpl) Create(do models.SkillProviderDO) (string, error) {
	if do.ProviderName == "" {
		return "", fmt.Errorf("provider name is required: %w", skillerr.ErrInvalidInput)
	}
	if do.ProviderType == models.ProviderTypeGit && do.RepoURL == "" {
		return "", fmt.Errorf("repo url is required for GIT provider: %w", skillerr.ErrInvalidInput)
	}
	_, exists, err := s.providerRepo.GetByName(do.ProviderName)
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("provider name duplicate: %s: %w", do.ProviderName, skillerr.ErrDuplicate)
	}
	po := models.ConvertSkillProviderDO2PO(do)
	if err := s.providerRepo.Create(po); err != nil {
		return "", err
	}
	return po.ID, nil
}

func (s *SkillProviderDomainServiceImpl) Update(do models.SkillProviderDO) error {
	po := models.ConvertSkillProviderDO2PO(do)
	return s.providerRepo.Update(po)
}

func (s *SkillProviderDomainServiceImpl) GetById(id string) (models.SkillProviderDO, error) {
	po, err := s.providerRepo.GetById(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.SkillProviderDO{}, fmt.Errorf("skill provider not found: %s: %w", id, skillerr.ErrNotFound)
		}
		return models.SkillProviderDO{}, err
	}
	return models.ConvertSkillProviderPO2DO(po), nil
}

// GetByIds 批量查询，返回 id → DO 的 map（用于列表组装 ProviderName）
func (s *SkillProviderDomainServiceImpl) GetByIds(ids []string) (map[string]models.SkillProviderDO, error) {
	result := make(map[string]models.SkillProviderDO)
	for _, id := range ids {
		po, err := s.providerRepo.GetById(id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, err
		}
		result[id] = models.ConvertSkillProviderPO2DO(po)
	}
	return result, nil
}

func (s *SkillProviderDomainServiceImpl) List(providerType, status string) ([]models.SkillProviderDO, error) {
	pos, err := s.providerRepo.List(providerType, status)
	if err != nil {
		return nil, err
	}
	dos := make([]models.SkillProviderDO, 0, len(pos))
	for _, po := range pos {
		dos = append(dos, models.ConvertSkillProviderPO2DO(po))
	}
	return dos, nil
}

// DeleteById 物理删除 Provider；删除前校验其下无 Skill，否则拒绝
func (s *SkillProviderDomainServiceImpl) DeleteById(id string) error {
	_, err := s.GetById(id) // 复用存在性校验
	if err != nil {
		return err
	}
	count, err := s.skillRepo.CountByProviderId(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("provider has %d active skills, cannot delete: %w", count, skillerr.ErrInvalidInput)
	}
	return s.providerRepo.DeleteById(id)
}

// Sync 当前实现仅校验存在性与状态；真实仓库扫描由 GIT importer 实现
func (s *SkillProviderDomainServiceImpl) Sync(id string) (models.SkillProviderDO, error) {
	do, err := s.GetById(id)
	if err != nil {
		return models.SkillProviderDO{}, err
	}
	if !do.IsActive() {
		return models.SkillProviderDO{}, fmt.Errorf("provider is not active: %s: %w", id, skillerr.ErrInvalidInput)
	}
	return do, nil
}

func (s *SkillProviderDomainServiceImpl) CountSkillsByProviderId(providerId string) (int64, error) {
	return s.skillRepo.CountByProviderId(providerId)
}
