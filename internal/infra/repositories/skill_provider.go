package repositories

import (
	"errors"

	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"

	"gorm.io/gorm"
)

type SkillProviderRepository interface {
	Create(po models.SkillProviderPO) error
	Update(po models.SkillProviderPO) error
	DeleteById(id string) error
	GetById(id string) (models.SkillProviderPO, error)
	GetByName(name string) (models.SkillProviderPO, bool, error)
	List(providerType, status string) ([]models.SkillProviderPO, error)
}

type SkillProviderRepositoryImpl struct {
	client *gorm.DB
}

func NewSkillProviderRepository() SkillProviderRepository {
	return &SkillProviderRepositoryImpl{client: storage.GetPostgresClient()}
}

func (r *SkillProviderRepositoryImpl) Create(po models.SkillProviderPO) error {
	return r.client.Create(&po).Error
}

func (r *SkillProviderRepositoryImpl) Update(po models.SkillProviderPO) error {
	return r.client.Model(&po).Where("skill_provider_id = ?", po.ID).Updates(po).Error
}

func (r *SkillProviderRepositoryImpl) DeleteById(id string) error {
	return r.client.Where("skill_provider_id = ?", id).Delete(&models.SkillProviderPO{}).Error
}

func (r *SkillProviderRepositoryImpl) GetById(id string) (models.SkillProviderPO, error) {
	var po models.SkillProviderPO
	err := r.client.Where("skill_provider_id = ?", id).First(&po).Error
	return po, err
}

// GetByName 按 provider_name 全局唯一查询；不存在返回 (zero, false, nil)
func (r *SkillProviderRepositoryImpl) GetByName(name string) (models.SkillProviderPO, bool, error) {
	var po models.SkillProviderPO
	err := r.client.Where("provider_name = ?", name).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return po, false, nil
		}
		return po, false, err
	}
	return po, true, nil
}

func (r *SkillProviderRepositoryImpl) List(providerType, status string) ([]models.SkillProviderPO, error) {
	var pos []models.SkillProviderPO
	tx := r.client.Model(&models.SkillProviderPO{})
	if providerType != "" {
		tx = tx.Where("provider_type = ?", providerType)
	}
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	err := tx.Order("created_at DESC").Find(&pos).Error
	return pos, err
}
