package evaluation

import "time"

type RunInstance struct {
	ID                    string
	TaskID                string
	CaseID                string
	CaseSnapshot          Case
	AgentConfigSnapshotID string
	Status                InstanceStatus
	Attempt               int
	ConversationID        string
	MessageID             string
	TraceID               string
	QueuedAt              *time.Time
	StartedAt             *time.Time
	FinishedAt            *time.Time
	HeartbeatAt           *time.Time
	DeadlineAt            *time.Time
	WorkerID              string
	ErrorMessage          string
}
