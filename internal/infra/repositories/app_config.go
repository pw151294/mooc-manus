package repositories

import (
	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"

	"gorm.io/gorm"
)

type AppConfigRepository interface {
	Create(models.AppConfigPO) error
	Update(models.AppConfigPO) error
	GetById(string) (models.AppConfigPO, error)
	DeleteById(string) error
	List() ([]models.AppConfigPO, error)
}

func NewAppConfigRepository() AppConfigRepository {
	return &AppConfigRepositoryImpl{client: storage.GetPostgresClient()}
}

type AppConfigRepositoryImpl struct {
	client *gorm.DB
}

func (a *AppConfigRepositoryImpl) Create(po models.AppConfigPO) error {
	return a.client.Create(&po).Error
}

func (a *AppConfigRepositoryImpl) Update(po models.AppConfigPO) error {
	return a.client.Model(&po).Where("id = ?", po.ID).Updates(po).Error
}

func (a *AppConfigRepositoryImpl) GetById(id string) (models.AppConfigPO, error) {
	var po models.AppConfigPO
	err := a.client.Where("id = ?", id).First(&po).Error
	return po, err
}

func (a *AppConfigRepositoryImpl) DeleteById(id string) error {
	return a.client.Where("id = ?", id).Delete(&models.AppConfigPO{}).Error
}

func (a *AppConfigRepositoryImpl) List() ([]models.AppConfigPO, error) {
	var pos []models.AppConfigPO
	err := a.client.Find(&pos).Error
	return pos, err
}
