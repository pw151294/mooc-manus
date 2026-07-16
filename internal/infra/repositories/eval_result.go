package repositories

import (
	"context"
	"mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EvalResultRepository interface {
	Create(ctx context.Context, r *evaluation.Result) error
	Get(ctx context.Context, instanceID string) (*evaluation.Result, error)
	Upsert(ctx context.Context, r *evaluation.Result) error
}

type evalResultRepositoryImpl struct {
	db *gorm.DB
}

func NewEvalResultRepository(db *gorm.DB) EvalResultRepository {
	return &evalResultRepositoryImpl{db: db}
}

func (r *evalResultRepositoryImpl) Create(ctx context.Context, res *evaluation.Result) error {
	po := resultToPO(res)
	return r.db.WithContext(ctx).Create(po).Error
}

func (r *evalResultRepositoryImpl) Get(ctx context.Context, instanceID string) (*evaluation.Result, error) {
	var po models.EvalResultPO
	err := r.db.WithContext(ctx).Where("instance_id = ?", instanceID).First(&po).Error
	if err != nil {
		return nil, err
	}
	return resultToDO(&po), nil
}

func (r *evalResultRepositoryImpl) Upsert(ctx context.Context, res *evaluation.Result) error {
	po := resultToPO(res)
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "instance_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"passed", "verify_exit_code", "verify_stdout", "verify_stderr", "prompt_tokens", "completion_tokens", "total_tokens", "agent_latency_ms", "error_log", "finished_at"}),
	}).Create(po).Error
}
