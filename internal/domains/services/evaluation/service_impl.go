package evaluation

import (
	"context"
	"errors"

	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/repositories"
)

type serviceImpl struct {
	caseRepo     repositories.EvalCaseRepository
	taskRepo     repositories.EvalTaskRepository
	instanceRepo repositories.EvalRunInstanceRepository
	resultRepo   repositories.EvalResultRepository
	snapshotRepo repositories.EvalAgentSnapshotRepository
	// M4 后期注入：executor, aggregator, verifier, chatRunner
	// M5 后期注入：mqClient
}

func NewEvaluationDomainService(
	caseRepo repositories.EvalCaseRepository,
	taskRepo repositories.EvalTaskRepository,
	instanceRepo repositories.EvalRunInstanceRepository,
	resultRepo repositories.EvalResultRepository,
	snapshotRepo repositories.EvalAgentSnapshotRepository,
) EvaluationDomainService {
	return &serviceImpl{
		caseRepo:     caseRepo,
		taskRepo:     taskRepo,
		instanceRepo: instanceRepo,
		resultRepo:   resultRepo,
		snapshotRepo: snapshotRepo,
	}
}

// 用例
func (s *serviceImpl) CreateCase(ctx context.Context, c *ev.Case) (*ev.Case, error) {
	return nil, errors.New("not implemented in M2")
}

func (s *serviceImpl) UpdateCase(ctx context.Context, c *ev.Case) (*ev.Case, error) {
	return nil, errors.New("not implemented in M2")
}

func (s *serviceImpl) DeleteCase(ctx context.Context, id string) error {
	return errors.New("not implemented in M2")
}

func (s *serviceImpl) ListCases(ctx context.Context, filter repositories.CaseListFilter, page, size int) ([]*ev.Case, int64, error) {
	return nil, 0, errors.New("not implemented in M2")
}

func (s *serviceImpl) GetCase(ctx context.Context, id string) (*ev.Case, error) {
	return nil, errors.New("not implemented in M2")
}

// 任务
func (s *serviceImpl) CreateTask(ctx context.Context, name string, caseIDs, agentConfigIDs []string) (*ev.Task, error) {
	return nil, errors.New("not implemented in M2")
}

func (s *serviceImpl) ListTasks(ctx context.Context, filter repositories.TaskListFilter, page, size int) ([]*ev.Task, int64, error) {
	return nil, 0, errors.New("not implemented in M2")
}

func (s *serviceImpl) GetTask(ctx context.Context, id string) (*ev.Task, error) {
	return nil, errors.New("not implemented in M2")
}

func (s *serviceImpl) RetryTaskFailedInstances(ctx context.Context, id string) (int, error) {
	return 0, errors.New("not implemented in M2")
}

func (s *serviceImpl) DeleteTask(ctx context.Context, id string) error {
	return errors.New("not implemented in M2")
}

// 实例
func (s *serviceImpl) ListInstances(ctx context.Context, taskID string, filter repositories.InstanceListFilter, page, size int) ([]*ev.RunInstance, int64, error) {
	return nil, 0, errors.New("not implemented in M2")
}

func (s *serviceImpl) GetInstance(ctx context.Context, id string) (*ev.RunInstance, error) {
	return nil, errors.New("not implemented in M2")
}

func (s *serviceImpl) RetryInstance(ctx context.Context, id string) error {
	return errors.New("not implemented in M2")
}

func (s *serviceImpl) DeleteInstance(ctx context.Context, id string) error {
	return errors.New("not implemented in M2")
}

// Worker 入口
func (s *serviceImpl) ExecuteInstance(ctx context.Context, instanceID string, workerID string) error {
	return errors.New("not implemented in M2")
}

// 巡检
func (s *serviceImpl) SweepStaleInstances(ctx context.Context) (int, error) {
	return 0, errors.New("not implemented in M2")
}

func (s *serviceImpl) ReconcileTaskStatuses(ctx context.Context) (int, error) {
	return 0, errors.New("not implemented in M2")
}

func (s *serviceImpl) ArchiveDeadTasks(ctx context.Context) (int, error) {
	return 0, errors.New("not implemented in M2")
}
