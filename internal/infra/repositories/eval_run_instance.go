package repositories

import (
	"context"
	"mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/models"
	"time"

	"gorm.io/gorm"
)

type InstanceListFilter struct {
	TaskID   string
	Status   evaluation.InstanceStatus
	StatusIn []evaluation.InstanceStatus // StatusIn 多状态筛选
}

type EvalRunInstanceRepository interface {
	Create(ctx context.Context, inst *evaluation.RunInstance) error
	GetByID(ctx context.Context, id string) (*evaluation.RunInstance, error)
	GetStatus(ctx context.Context, id string) (evaluation.InstanceStatus, error)
	List(ctx context.Context, filter InstanceListFilter, page, size int) ([]*evaluation.RunInstance, int64, error)
	Update(ctx context.Context, inst *evaluation.RunInstance) error
	Delete(ctx context.Context, id string) error
	UpdateTraceID(ctx context.Context, id, traceID string) error
	ListStaleInstances(ctx context.Context, before time.Time) ([]*evaluation.RunInstance, error)
	UpdateHeartbeat(ctx context.Context, id, workerID string, now time.Time) error
	CASStatus(ctx context.Context, id string, from, to evaluation.InstanceStatus) (bool, error)
}

type evalRunInstanceRepositoryImpl struct {
	db *gorm.DB
}

func NewEvalRunInstanceRepository(db *gorm.DB) EvalRunInstanceRepository {
	return &evalRunInstanceRepositoryImpl{db: db}
}

func (r *evalRunInstanceRepositoryImpl) Create(ctx context.Context, inst *evaluation.RunInstance) error {
	po := instanceToPO(inst)
	return r.db.WithContext(ctx).Create(po).Error
}

func (r *evalRunInstanceRepositoryImpl) GetByID(ctx context.Context, id string) (*evaluation.RunInstance, error) {
	var po models.EvalRunInstancePO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&po).Error
	if err != nil {
		return nil, err
	}
	return instanceToDO(&po), nil
}

func (r *evalRunInstanceRepositoryImpl) GetStatus(ctx context.Context, id string) (evaluation.InstanceStatus, error) {
	var status string
	err := r.db.WithContext(ctx).Model(&models.EvalRunInstancePO{}).
		Where("id = ?", id).Select("status").Scan(&status).Error
	return evaluation.InstanceStatus(status), err
}

func (r *evalRunInstanceRepositoryImpl) List(ctx context.Context, filter InstanceListFilter, page, size int) ([]*evaluation.RunInstance, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.EvalRunInstancePO{})

	if filter.TaskID != "" {
		query = query.Where("task_id = ?", filter.TaskID)
	}
	// StatusIn 优先；否则回落到单值 Status
	if len(filter.StatusIn) > 0 {
		statuses := make([]string, 0, len(filter.StatusIn))
		for _, s := range filter.StatusIn {
			statuses = append(statuses, string(s))
		}
		query = query.Where("status IN ?", statuses)
	} else if filter.Status != "" {
		query = query.Where("status = ?", string(filter.Status))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var pos []models.EvalRunInstancePO
	offset := (page - 1) * size
	err := query.Offset(offset).Limit(size).Order("created_at DESC").Find(&pos).Error
	if err != nil {
		return nil, 0, err
	}

	dos := make([]*evaluation.RunInstance, len(pos))
	for i := range pos {
		dos[i] = instanceToDO(&pos[i])
	}
	return dos, total, nil
}

func (r *evalRunInstanceRepositoryImpl) Update(ctx context.Context, inst *evaluation.RunInstance) error {
	po := instanceToPO(inst)
	return r.db.WithContext(ctx).Model(&models.EvalRunInstancePO{}).Where("id = ?", inst.ID).Updates(po).Error
}

func (r *evalRunInstanceRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.EvalRunInstancePO{}).Error
}

func (r *evalRunInstanceRepositoryImpl) UpdateTraceID(ctx context.Context, id, traceID string) error {
	return r.db.WithContext(ctx).Model(&models.EvalRunInstancePO{}).
		Where("id = ?", id).Update("trace_id", traceID).Error
}

func (r *evalRunInstanceRepositoryImpl) ListStaleInstances(ctx context.Context, before time.Time) ([]*evaluation.RunInstance, error) {
	var pos []models.EvalRunInstancePO
	err := r.db.WithContext(ctx).
		Where("status NOT IN ?", []string{"PASSED", "FAILED", "TIMEOUT", "CANCELED"}).
		Where("(heartbeat_at IS NULL OR heartbeat_at < ?) OR (deadline_at IS NOT NULL AND deadline_at < ?)", before, time.Now()).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}

	dos := make([]*evaluation.RunInstance, len(pos))
	for i := range pos {
		dos[i] = instanceToDO(&pos[i])
	}
	return dos, nil
}

func (r *evalRunInstanceRepositoryImpl) UpdateHeartbeat(ctx context.Context, id, workerID string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&models.EvalRunInstancePO{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"heartbeat_at": now,
			"worker_id":    workerID,
		}).Error
}

func (r *evalRunInstanceRepositoryImpl) CASStatus(ctx context.Context, id string, from, to evaluation.InstanceStatus) (bool, error) {
	result := r.db.WithContext(ctx).Model(&models.EvalRunInstancePO{}).
		Where("id = ? AND status = ?", id, string(from)).
		Update("status", string(to))
	return result.RowsAffected > 0, result.Error
}
