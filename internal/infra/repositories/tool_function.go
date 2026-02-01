package repositories

import (
	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"

	"gorm.io/gorm"
)

type ToolFunctionRepository interface {
	Create(models.ToolFunctionPO) error
	BatchCreate([]models.ToolFunctionPO) error
	Update(models.ToolFunctionPO) error
	DeleteById(string) error
	DeleteByIds([]string) error
	DeleteByProviderId(string) error // Added interface
	GetByIds([]string) ([]models.ToolFunctionPO, error)
	ListBy(functionIds []string, providerIds []string) ([]models.ToolFunctionPO, error)
	ListByProviderId(string) ([]models.ToolFunctionPO, error)
	List() ([]models.ToolFunctionPO, error)
	Transaction(func(txFuncRepo ToolFunctionRepository, txProviderRepo ToolProviderRepository) error) error
}

type ToolFunctionRepositoryImpl struct {
	dbCli *gorm.DB
}

func NewToolFunctionRepository() ToolFunctionRepository {
	return &ToolFunctionRepositoryImpl{
		dbCli: storage.GetPostgresClient(),
	}
}

func (t *ToolFunctionRepositoryImpl) Create(po models.ToolFunctionPO) error {
	return t.dbCli.Create(&po).Error
}

func (t *ToolFunctionRepositoryImpl) BatchCreate(pos []models.ToolFunctionPO) error {
	return t.dbCli.Create(&pos).Error
}

func (t *ToolFunctionRepositoryImpl) Update(po models.ToolFunctionPO) error {
	return t.dbCli.Model(&po).Where("id = ?", po.ID).Updates(po).Error
}

func (t *ToolFunctionRepositoryImpl) DeleteById(id string) error {
	return t.dbCli.Where("id = ?", id).Delete(&models.ToolFunctionPO{}).Error
}

func (t *ToolFunctionRepositoryImpl) DeleteByIds(ids []string) error {
	return t.dbCli.Where("id IN ?", ids).Delete(&models.ToolFunctionPO{}).Error
}

func (t *ToolFunctionRepositoryImpl) DeleteByProviderId(providerId string) error {
	return t.dbCli.Where("provider_id = ?", providerId).Delete(&models.ToolFunctionPO{}).Error
}

func (t *ToolFunctionRepositoryImpl) GetByIds(ids []string) ([]models.ToolFunctionPO, error) {
	var pos []models.ToolFunctionPO
	err := t.dbCli.Where("id IN ?", ids).Find(&pos).Error
	return pos, err
}

func (t *ToolFunctionRepositoryImpl) ListBy(functionIds []string, providerIds []string) ([]models.ToolFunctionPO, error) {
	var pos []models.ToolFunctionPO
	if len(functionIds) == 0 && len(providerIds) == 0 {
		return pos, nil
	}

	tx := t.dbCli
	if len(functionIds) > 0 && len(providerIds) > 0 {
		tx = tx.Where("id IN ? OR provider_id IN ?", functionIds, providerIds)
	} else if len(functionIds) > 0 {
		tx = tx.Where("id IN ?", functionIds)
	} else {
		tx = tx.Where("provider_id IN ?", providerIds)
	}

	err := tx.Find(&pos).Error
	return pos, err
}

func (t *ToolFunctionRepositoryImpl) ListByProviderId(providerId string) ([]models.ToolFunctionPO, error) {
	var pos []models.ToolFunctionPO
	err := t.dbCli.Where("provider_id = ?", providerId).Find(&pos).Error
	return pos, err
}

func (t *ToolFunctionRepositoryImpl) List() ([]models.ToolFunctionPO, error) {
	var pos []models.ToolFunctionPO
	err := t.dbCli.Find(&pos).Error
	return pos, err
}

func (t *ToolFunctionRepositoryImpl) Transaction(fn func(txFuncRepo ToolFunctionRepository, txProviderRepo ToolProviderRepository) error) error {
	return t.dbCli.Transaction(func(tx *gorm.DB) error {
		txFuncRepo := &ToolFunctionRepositoryImpl{dbCli: tx}
		txProviderRepo := &ToolProviderRepositoryImpl{dbCli: tx}
		return fn(txFuncRepo, txProviderRepo)
	})
}
