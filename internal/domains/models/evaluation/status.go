package evaluation

type TaskStatus string
type InstanceStatus string

const (
	TaskStatusPending       TaskStatus = "PENDING"
	TaskStatusRunning       TaskStatus = "RUNNING"
	TaskStatusSucceeded     TaskStatus = "SUCCEEDED"
	TaskStatusPartialFailed TaskStatus = "PARTIAL_FAILED"
)

const (
	InstanceStatusPending      InstanceStatus = "PENDING"
	InstanceStatusQueued       InstanceStatus = "QUEUED"
	InstanceStatusInitializing InstanceStatus = "INITIALIZING"
	InstanceStatusRunning      InstanceStatus = "RUNNING"
	InstanceStatusVerifying    InstanceStatus = "VERIFYING"
	InstanceStatusPassed       InstanceStatus = "PASSED"
	InstanceStatusFailed       InstanceStatus = "FAILED"
	InstanceStatusTimeout      InstanceStatus = "TIMEOUT"
	InstanceStatusCanceled     InstanceStatus = "CANCELED"
)

func (s InstanceStatus) IsTerminal() bool {
	switch s {
	case InstanceStatusPassed, InstanceStatusFailed, InstanceStatusTimeout, InstanceStatusCanceled:
		return true
	}
	return false
}
