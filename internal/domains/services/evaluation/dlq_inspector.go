package evaluation

import (
	"context"

	appconfig "mooc-manus/internal/domains/models"
)

// DLQInspector 屏蔽 asynq 依赖：让 domain 层通过抽象接口访问死信队列。
// 由 Application 层实现（背后 asynq.Inspector），domain 不感知 asynq 存在。
// 若注入为 nil，则 ArchiveDeadTasks 返回 (0, nil) 并降级为 stub。
type DLQInspector interface {
	// ListArchivedRunInstanceIDs 列出 DLQ 中已归档的评测实例 ID。
	ListArchivedRunInstanceIDs(ctx context.Context) ([]string, error)
}

// AppConfigLoader 屏蔽父 package 依赖：让 domain 层通过抽象接口按 ID 加载 AppConfig。
// 目的：evaluation 包无法直接 import services 包（会形成循环依赖），因此通过抽象接口反转依赖。
// 由 Application 层（或 InitRouter）在装配时把 services.AppConfigDomainService 适配到此接口。
type AppConfigLoader interface {
	GetById(id string) (appconfig.AppConfigDO, error)
}
