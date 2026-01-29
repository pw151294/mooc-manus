package repositories

import (
	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"

	"gorm.io/gorm"
)

type ToolProviderRepository interface {
	Create(models.ToolProviderPO) error
	Update(models.ToolProviderPO) error
	GetById(string) (models.ToolProviderPO, error)
	GetByIds([]string) ([]models.ToolProviderPO, error)
	DeleteById(string) error
	Exists(string) bool
	List() ([]models.ToolProviderPO, error)
}

type ToolProviderRepositoryImpl struct {
	dbCli *gorm.DB
}

func (t *ToolProviderRepositoryImpl) GetById(id string) (models.ToolProviderPO, error) {
	var po models.ToolProviderPO
	err := t.dbCli.Where("id = ?", id).First(&po).Error
	return po, err
}

func (t *ToolProviderRepositoryImpl) GetByIds(ids []string) ([]models.ToolProviderPO, error) {
	var pos []models.ToolProviderPO
	err := t.dbCli.Where("id IN ?", ids).Find(&pos).Error
	return pos, err
}

func (t *ToolProviderRepositoryImpl) Exists(id string) bool {
	var count int64
	t.dbCli.Model(&models.ToolProviderPO{}).Where("id = ?", id).Count(&count)
	return count > 0
}

func NewToolProviderRepository() ToolProviderRepository {
	return &ToolProviderRepositoryImpl{dbCli: storage.GetPostgresClient()}
}

func (t *ToolProviderRepositoryImpl) Create(po models.ToolProviderPO) error {
	return t.dbCli.Create(&po).Error
}

func (t *ToolProviderRepositoryImpl) Update(po models.ToolProviderPO) error {
	return t.dbCli.Model(&po).Where("id = ?", po.ID).Updates(po).Error
}

func (t *ToolProviderRepositoryImpl) DeleteById(id string) error {
	return t.dbCli.Transaction(func(tx *gorm.DB) error {
		// 1. 删除 tool_function 表中关联的记录
		if err := tx.Where("provider_id = ?", id).Delete(&models.ToolFunctionPO{}).Error; err != nil {
			return err
		}

		// 2. 删除 tool_provider 表中的记录
		if err := tx.Where("id = ?", id).Delete(&models.ToolProviderPO{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func (t *ToolProviderRepositoryImpl) List() ([]models.ToolProviderPO, error) {
	var pos []models.ToolProviderPO
	err := t.dbCli.Find(&pos).Error
	return pos, err
}
