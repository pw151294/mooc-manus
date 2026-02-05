package repositories

import (
	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"

	"github.com/google/uuid"

	"gorm.io/gorm"
)

type AppConfigRepository interface {
	Create(models.AppConfigPO) error
	CreateA2AServers(configs []models.A2AServerConfigPO, functions []models.A2AServerFunctionPO) error
	Update(models.AppConfigPO) error
	UpdateA2AServers(configs []models.A2AServerConfigPO, functions []models.A2AServerFunctionPO) error
	GetById(string) (models.AppConfigPO, error)
	GetA2AServerConfigsByAppConfigId(appConfigId string) ([]models.A2AServerConfigPO, error)
	GetA2AServerConfigById(id string) (models.A2AServerConfigPO, error)
	GetA2AServerFunctionsByServerConfigIds(serverConfigIds []string) ([]models.A2AServerFunctionPO, error)
	List() ([]models.AppConfigPO, error)
	DeleteById(string) error
	DeleteA2AServerFunctionsByServerConfigIds(serverConfigIds []string) error
	DeleteA2AServers(serverConfigIds []string) error
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
	return a.client.Transaction(func(tx *gorm.DB) error {
		// 删除 app_config
		if err := tx.Where("id = ?", id).Delete(&models.AppConfigPO{}).Error; err != nil {
			return err
		}

		// 获取与 app_config_id 关联的 a2a_server_config IDs
		var serverConfigIds []string
		if err := tx.Model(&models.A2AServerConfigPO{}).
			Where("app_config_id = ?", id).
			Pluck("id", &serverConfigIds).Error; err != nil {
			return err
		}

		// 删除与 serverConfigIds 关联的 a2a_server_function 记录
		if len(serverConfigIds) > 0 {
			if err := tx.Where("a2a_server_config_id IN ?", serverConfigIds).
				Delete(&models.A2AServerFunctionPO{}).Error; err != nil {
				return err
			}
		}

		// 删除与 app_config_id 关联的 a2a_server_config 记录
		if err := tx.Where("app_config_id = ?", id).
			Delete(&models.A2AServerConfigPO{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func (a *AppConfigRepositoryImpl) List() ([]models.AppConfigPO, error) {
	var pos []models.AppConfigPO
	err := a.client.Find(&pos).Error
	return pos, err
}

func (a *AppConfigRepositoryImpl) UpdateA2AServers(configs []models.A2AServerConfigPO, functions []models.A2AServerFunctionPO) error {
	return a.client.Transaction(func(tx *gorm.DB) error {
		// 更新 a2a_server_config
		for _, config := range configs {
			if err := tx.Model(&models.A2AServerConfigPO{}).
				Where("id = ?", config.ID).
				Updates(config).Error; err != nil {
				return err
			}
		}

		// 获取 a2a_server_config 的 ServerConfigId 列表
		serverConfigIds := make([]string, len(configs))
		for i, config := range configs {
			serverConfigIds[i] = config.ID
		}

		// 删除与 serverConfigIds 关联的 a2a_server_function 记录
		if len(serverConfigIds) > 0 {
			if err := tx.Where("a2a_server_config_id IN ?", serverConfigIds).
				Delete(&models.A2AServerFunctionPO{}).Error; err != nil {
				return err
			}
		}

		// 给 functions 的每条记录设置主键 UUID
		for i := range functions {
			functions[i].ID = uuid.New().String()
		}

		// 批量插入新的 a2a_server_function 记录
		if len(functions) > 0 {
			if err := tx.Create(&functions).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (a *AppConfigRepositoryImpl) GetA2AServerConfigsByAppConfigId(appConfigId string) ([]models.A2AServerConfigPO, error) {
	var configs []models.A2AServerConfigPO
	err := a.client.Where("app_config_id = ?", appConfigId).Find(&configs).Error
	return configs, err
}

func (a *AppConfigRepositoryImpl) GetA2AServerConfigById(id string) (models.A2AServerConfigPO, error) {
	var config models.A2AServerConfigPO
	err := a.client.Where("id = ?", id).First(&config).Error
	return config, err
}

func (a *AppConfigRepositoryImpl) DeleteA2AServerFunctionsByServerConfigIds(serverConfigIds []string) error {
	if len(serverConfigIds) == 0 {
		return nil
	}
	return a.client.Where("a2a_server_config_id IN ?", serverConfigIds).Delete(&models.A2AServerFunctionPO{}).Error
}

func (a *AppConfigRepositoryImpl) CreateA2AServers(configs []models.A2AServerConfigPO, functions []models.A2AServerFunctionPO) error {
	return a.client.Transaction(func(tx *gorm.DB) error {
		// 批量创建 a2a_server_config
		if len(configs) > 0 {
			if err := tx.Create(&configs).Error; err != nil {
				return err
			}
		}

		// 批量创建 a2a_server_function
		if len(functions) > 0 {
			if err := tx.Create(&functions).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (a *AppConfigRepositoryImpl) GetA2AServerFunctionsByServerConfigIds(serverConfigIds []string) ([]models.A2AServerFunctionPO, error) {
	if len(serverConfigIds) == 0 {
		return nil, nil // 如果没有提供 serverConfigIds，直接返回空结果
	}

	var functions []models.A2AServerFunctionPO
	err := a.client.Where("a2a_server_config_id IN ?", serverConfigIds).Find(&functions).Error
	return functions, err
}

func (a *AppConfigRepositoryImpl) DeleteA2AServers(serverConfigIds []string) error {
	if len(serverConfigIds) == 0 {
		return nil // 如果没有提供 serverConfigIds，直接返回
	}

	return a.client.Transaction(func(tx *gorm.DB) error {
		// 删除与 serverConfigIds 关联的 a2a_server_function 记录
		if err := tx.Where("a2a_server_config_id IN ?", serverConfigIds).
			Delete(&models.A2AServerFunctionPO{}).Error; err != nil {
			return err
		}

		// 删除 a2a_server_config 记录
		if err := tx.Where("id IN ?", serverConfigIds).
			Delete(&models.A2AServerConfigPO{}).Error; err != nil {
			return err
		}

		return nil
	})
}
