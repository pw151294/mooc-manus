package repositories

import (
	"errors"

	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"

	"gorm.io/gorm"
)

type TaskExecutionRepository interface {
	Create(po models.TaskExecutionPO) error
	Update(po models.TaskExecutionPO) error
	GetById(taskId string) (models.TaskExecutionPO, bool, error)
	ListByAppType(appId, appType string) ([]models.TaskExecutionPO, error)
	DeleteByIds(taskIds []string) error
}

type TaskExecutionRepositoryImpl struct {
	client *gorm.DB
}

func NewTaskExecutionRepository() TaskExecutionRepository {
	return &TaskExecutionRepositoryImpl{client: storage.GetPostgresClient()}
}

func (r *TaskExecutionRepositoryImpl) Create(po models.TaskExecutionPO) error {
	return r.client.Create(&po).Error
}

// Update 全字段更新；Service 层先 GetById 再修改字段后调用
func (r *TaskExecutionRepositoryImpl) Update(po models.TaskExecutionPO) error {
	return r.client.Model(&po).
		Where("task_id = ?", po.TaskID).
		// 显式 Select * 以允许写入零值（如 archived_at 仍为 NULL 时不需要写）
		Updates(map[string]interface{}{
			"status":      po.Status,
			"stage":       po.Stage,
			"progress":    po.Progress,
			"ext_info":    po.ExtInfo,
			"archived_at": po.ArchivedAt,
		}).Error
}

func (r *TaskExecutionRepositoryImpl) GetById(taskId string) (models.TaskExecutionPO, bool, error) {
	var po models.TaskExecutionPO
	err := r.client.Where("task_id = ?", taskId).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return po, false, nil
		}
		return po, false, err
	}
	return po, true, nil
}

// ListByAppType 业务规则 §5.12.8：当前主体 + app_type=SKILL_IMPORT 维度返回任务集合
// mooc-manus 不引入多租户隔离，仅按 (app_id, app_type) 过滤
func (r *TaskExecutionRepositoryImpl) ListByAppType(appId, appType string) ([]models.TaskExecutionPO, error) {
	var pos []models.TaskExecutionPO
	tx := r.client.Model(&models.TaskExecutionPO{})
	if appId != "" {
		tx = tx.Where("app_id = ?", appId)
	}
	if appType != "" {
		tx = tx.Where("app_type = ?", appType)
	}
	err := tx.Order("created_at DESC").Find(&pos).Error
	return pos, err
}

func (r *TaskExecutionRepositoryImpl) DeleteByIds(taskIds []string) error {
	if len(taskIds) == 0 {
		return nil
	}
	return r.client.Where("task_id IN ?", taskIds).Delete(&models.TaskExecutionPO{}).Error
}
