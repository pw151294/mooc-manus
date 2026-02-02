package repositories

import (
	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"

	"gorm.io/gorm"
)

type A2AServerConfigRepository interface {
	Create(config models.A2AServerConfigPO) error
	Update(config models.A2AServerConfigPO) error
	DeleteById(id string) error
	GetById(id string) (models.A2AServerConfigPO, error)
	ListByA2AConfigId(a2aConfigId string) ([]models.A2AServerConfigPO, error)
	ListByAppConfigId(appConfigId string) ([]models.A2AServerConfigPO, error)
}

type A2AServerConfigRepositoryImpl struct {
	dbCli *gorm.DB
}

func NewA2AServerConfigRepository() A2AServerConfigRepository {
	return &A2AServerConfigRepositoryImpl{
		dbCli: storage.GetPostgresClient(),
	}
}

func (r *A2AServerConfigRepositoryImpl) Create(config models.A2AServerConfigPO) error {
	return r.dbCli.Create(&config).Error
}

func (r *A2AServerConfigRepositoryImpl) Update(config models.A2AServerConfigPO) error {
	return r.dbCli.Model(&config).Where("id = ?", config.ID).Updates(config).Error
}

func (r *A2AServerConfigRepositoryImpl) DeleteById(id string) error {
	return r.dbCli.Where("id = ?", id).Delete(&models.A2AServerConfigPO{}).Error
}

func (r *A2AServerConfigRepositoryImpl) GetById(id string) (models.A2AServerConfigPO, error) {
	var config models.A2AServerConfigPO
	err := r.dbCli.Where("id = ?", id).First(&config).Error
	return config, err
}

func (r *A2AServerConfigRepositoryImpl) ListByA2AConfigId(a2aConfigId string) ([]models.A2AServerConfigPO, error) {
	var configs []models.A2AServerConfigPO
	err := r.dbCli.Where("a2a_config_id = ?", a2aConfigId).Find(&configs).Error
	return configs, err
}

func (r *A2AServerConfigRepositoryImpl) ListByAppConfigId(appConfigId string) ([]models.A2AServerConfigPO, error) {
	var configs []models.A2AServerConfigPO
	err := r.dbCli.Joins("JOIN a2a_config ON a2a_server_config.a2a_config_id = a2a_config.id").
		Where("a2a_config.app_config_id = ?", appConfigId).
		Find(&configs).Error
	return configs, err
}
