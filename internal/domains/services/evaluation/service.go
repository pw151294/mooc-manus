package evaluation

import (
	"context"

	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/repositories"
)

type EvaluationDomainService interface {
	// 用例
	CreateCase(ctx context.Context, c *ev.Case) (*ev.Case, error)
	UpdateCase(ctx context.Context, c *ev.Case) (*ev.Case, error)
	DeleteCase(ctx context.Context, id string) error
	ListCases(ctx context.Context, filter repositories.CaseListFilter, page, size int) ([]*ev.Case, int64, error)
	GetCase(ctx context.Context, id string) (*ev.Case, error)

	// 任务
	CreateTask(ctx context.Context, name string, caseIDs, agentConfigIDs []string) (*ev.Task, error)
	ListTasks(ctx context.Context, filter repositories.TaskListFilter, page, size int) ([]*ev.Task, int64, error)
	GetTask(ctx context.Context, id string) (*ev.Task, error)
	RetryTaskFailedInstances(ctx context.Context, id string) (int, error)
	DeleteTask(ctx context.Context, id string) error

	// 实例
	ListInstances(ctx context.Context, taskID string, filter repositories.InstanceListFilter, page, size int) ([]*ev.RunInstance, int64, error)
	GetInstance(ctx context.Context, id string) (*ev.RunInstance, error)
	RetryInstance(ctx context.Context, id string) error
	DeleteInstance(ctx context.Context, id string) error

	// Worker 入口
	ExecuteInstance(ctx context.Context, instanceID string, workerID string) error

	// 巡检
	SweepStaleInstances(ctx context.Context) (int, error)
	ReconcileTaskStatuses(ctx context.Context) (int, error)
	ArchiveDeadTasks(ctx context.Context) (int, error)
}
