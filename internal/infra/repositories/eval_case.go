package repositories

import (
	"context"
	"mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/models"

	"gorm.io/gorm"
)

type CaseListFilter struct {
	NameLike string
	Tags     []string
}

type EvalCaseRepository interface {
	Create(ctx context.Context, c *evaluation.Case) error
	Get(ctx context.Context, id string) (*evaluation.Case, error)
	List(ctx context.Context, filter CaseListFilter, page, size int) ([]*evaluation.Case, int64, error)
	Update(ctx context.Context, c *evaluation.Case) error
	Delete(ctx context.Context, id string) error
	ExistsRunningReferences(ctx context.Context, caseID string) (bool, error)
}

type evalCaseRepositoryImpl struct {
	db *gorm.DB
}

func NewEvalCaseRepository(db *gorm.DB) EvalCaseRepository {
	return &evalCaseRepositoryImpl{db: db}
}

func (r *evalCaseRepositoryImpl) Create(ctx context.Context, c *evaluation.Case) error {
	po := caseToPO(c)
	return r.db.WithContext(ctx).Create(po).Error
}

func (r *evalCaseRepositoryImpl) Get(ctx context.Context, id string) (*evaluation.Case, error) {
	var po models.EvalCasePO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&po).Error
	if err != nil {
		return nil, err
	}
	return caseToDO(&po), nil
}

func (r *evalCaseRepositoryImpl) List(ctx context.Context, filter CaseListFilter, page, size int) ([]*evaluation.Case, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.EvalCasePO{})

	if filter.NameLike != "" {
		query = query.Where("name LIKE ?", "%"+filter.NameLike+"%")
	}
	if len(filter.Tags) > 0 {
		query = query.Where("tags @> ?", filter.Tags)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var pos []models.EvalCasePO
	offset := (page - 1) * size
	err := query.Offset(offset).Limit(size).Find(&pos).Error
	if err != nil {
		return nil, 0, err
	}

	dos := make([]*evaluation.Case, len(pos))
	for i := range pos {
		dos[i] = caseToDO(&pos[i])
	}
	return dos, total, nil
}

func (r *evalCaseRepositoryImpl) Update(ctx context.Context, c *evaluation.Case) error {
	po := caseToPO(c)
	return r.db.WithContext(ctx).Model(&models.EvalCasePO{}).Where("id = ?", c.ID).Updates(po).Error
}

func (r *evalCaseRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.EvalCasePO{}).Error
}

func (r *evalCaseRepositoryImpl) ExistsRunningReferences(ctx context.Context, caseID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.EvalRunInstancePO{}).
		Joins("JOIN eval_task ON eval_run_instance.task_id = eval_task.id").
		Where("eval_run_instance.case_id = ?", caseID).
		Where("eval_task.status IN ?", []string{"PENDING", "RUNNING"}).
		Count(&count).Error
	return count > 0, err
}
