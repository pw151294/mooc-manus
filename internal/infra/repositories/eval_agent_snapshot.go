package repositories

import (
	"context"
	"mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/models"

	"gorm.io/gorm"
)

type EvalAgentSnapshotRepository interface {
	Create(ctx context.Context, s *evaluation.AgentSnapshot) error
	Get(ctx context.Context, id string) (*evaluation.AgentSnapshot, error)
	Delete(ctx context.Context, id string) error
	BatchCreate(ctx context.Context, snapshots []*evaluation.AgentSnapshot) error
}

type evalAgentSnapshotRepositoryImpl struct {
	db *gorm.DB
}

func NewEvalAgentSnapshotRepository(db *gorm.DB) EvalAgentSnapshotRepository {
	return &evalAgentSnapshotRepositoryImpl{db: db}
}

func (r *evalAgentSnapshotRepositoryImpl) Create(ctx context.Context, s *evaluation.AgentSnapshot) error {
	po := snapshotToPO(s)
	return r.db.WithContext(ctx).Create(po).Error
}

func (r *evalAgentSnapshotRepositoryImpl) Get(ctx context.Context, id string) (*evaluation.AgentSnapshot, error) {
	var po models.EvalAgentSnapshotPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&po).Error
	if err != nil {
		return nil, err
	}
	return snapshotToDO(&po), nil
}

func (r *evalAgentSnapshotRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.EvalAgentSnapshotPO{}).Error
}

func (r *evalAgentSnapshotRepositoryImpl) BatchCreate(ctx context.Context, snapshots []*evaluation.AgentSnapshot) error {
	pos := make([]*models.EvalAgentSnapshotPO, len(snapshots))
	for i, s := range snapshots {
		pos[i] = snapshotToPO(s)
	}
	return r.db.WithContext(ctx).Create(&pos).Error
}
