package repositories

import (
	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"

	"gorm.io/gorm"
)

type A2AConfigRepository interface {
	Create(config models.A2AConfigPO) error
	Update(config models.A2AConfigPO) error
	DeleteById(id string) error
	GetById(id string) (models.A2AConfigPO, error)
	ListByAppConfigId(appConfigId string) ([]models.A2AConfigPO, error)
}

type A2AConfigRepositoryImpl struct {
	dbCli *gorm.DB
}

func NewA2AConfigRepository() A2AConfigRepository {
	return &A2AConfigRepositoryImpl{
		dbCli: storage.GetPostgresClient(),
	}
}

func (r *A2AConfigRepositoryImpl) Create(config models.A2AConfigPO) error {
	return r.dbCli.Create(&config).Error
}

func (r *A2AConfigRepositoryImpl) Update(config models.A2AConfigPO) error {
	return r.dbCli.Model(&config).Where("id = ?", config.ID).Updates(config).Error
}

func (r *A2AConfigRepositoryImpl) DeleteById(id string) error {
	return r.dbCli.Where("id = ?", id).Delete(&models.A2AConfigPO{}).Error
}

func (r *A2AConfigRepositoryImpl) GetById(id string) (models.A2AConfigPO, error) {
	var config models.A2AConfigPO
	err := r.dbCli.Where("id = ?", id).First(&config).Error
	return config, err
}

func (r *A2AConfigRepositoryImpl) ListByAppConfigId(appConfigId string) ([]models.A2AConfigPO, error) {
	var configs []models.A2AConfigPO
	err := r.dbCli.Where("app_config_id = ?", appConfigId).Find(&configs).Error
	return configs, err
}
