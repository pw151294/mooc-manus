package evaluation

import "time"

type Task struct {
	ID             string
	Name           string
	CaseIDs        []string
	AgentConfigIDs []string
	Status         TaskStatus
	TotalCount     int
	SucceededCount int
	FailedCount    int
	RunningCount   int
	CreatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
}
