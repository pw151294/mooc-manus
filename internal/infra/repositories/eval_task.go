package repositories

import (
	"context"
	"mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/models"
	"time"

	"gorm.io/gorm"
)

type TaskListFilter struct {
	Status   evaluation.TaskStatus
	StatusIn []evaluation.TaskStatus // StatusIn 多状态筛选
}

type EvalTaskRepository interface {
	Create(ctx context.Context, t *evaluation.Task) error
	Get(ctx context.Context, id string) (*evaluation.Task, error)
	List(ctx context.Context, filter TaskListFilter, page, size int) ([]*evaluation.Task, int64, error)
	Update(ctx context.Context, t *evaluation.Task) error
	Delete(ctx context.Context, id string) error
	RecountAndTransit(ctx context.Context, taskID string) error
}

type evalTaskRepositoryImpl struct {
	db *gorm.DB
}

func NewEvalTaskRepository(db *gorm.DB) EvalTaskRepository {
	return &evalTaskRepositoryImpl{db: db}
}

func (r *evalTaskRepositoryImpl) Create(ctx context.Context, t *evaluation.Task) error {
	po := taskToPO(t)
	return r.db.WithContext(ctx).Create(po).Error
}

func (r *evalTaskRepositoryImpl) Get(ctx context.Context, id string) (*evaluation.Task, error) {
	var po models.EvalTaskPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&po).Error
	if err != nil {
		return nil, err
	}
	return taskToDO(&po), nil
}

func (r *evalTaskRepositoryImpl) List(ctx context.Context, filter TaskListFilter, page, size int) ([]*evaluation.Task, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.EvalTaskPO{})

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

	var pos []models.EvalTaskPO
	offset := (page - 1) * size
	err := query.Offset(offset).Limit(size).Order("created_at DESC").Find(&pos).Error
	if err != nil {
		return nil, 0, err
	}

	dos := make([]*evaluation.Task, len(pos))
	for i := range pos {
		dos[i] = taskToDO(&pos[i])
	}
	return dos, total, nil
}

func (r *evalTaskRepositoryImpl) Update(ctx context.Context, t *evaluation.Task) error {
	po := taskToPO(t)
	return r.db.WithContext(ctx).Model(&models.EvalTaskPO{}).Where("id = ?", t.ID).Updates(po).Error
}

func (r *evalTaskRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.EvalTaskPO{}).Error
}

func (r *evalTaskRepositoryImpl) RecountAndTransit(ctx context.Context, taskID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var terminal, passed, total int64
		err := tx.Raw(`
			SELECT
			  COUNT(*) FILTER (WHERE status IN ('PASSED','FAILED','TIMEOUT','CANCELED')) AS terminal,
			  COUNT(*) FILTER (WHERE status='PASSED') AS passed,
			  COUNT(*) AS total
			FROM eval_run_instance WHERE task_id = ?
		`, taskID).Row().Scan(&terminal, &passed, &total)
		if err != nil {
			return err
		}

		var newStatus string
		switch {
		case terminal < total:
			newStatus = "RUNNING"
		case terminal == total && passed == total:
			newStatus = "SUCCEEDED"
		default:
			newStatus = "PARTIAL_FAILED"
		}

		now := time.Now()
		upd := map[string]any{
			"status":          newStatus,
			"succeeded_count": passed,
			"failed_count":    terminal - passed,
			"running_count":   total - terminal,
		}
		if newStatus == "SUCCEEDED" || newStatus == "PARTIAL_FAILED" {
			upd["finished_at"] = &now
		}
		return tx.Model(&models.EvalTaskPO{}).Where("id = ?", taskID).Updates(upd).Error
	})
}
