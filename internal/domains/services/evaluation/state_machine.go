package evaluation

import (
	"fmt"

	ev "mooc-manus/internal/domains/models/evaluation"
)

var instanceWhitelist = map[ev.InstanceStatus]map[ev.InstanceStatus]bool{
	ev.InstanceStatusPending:      {ev.InstanceStatusQueued: true},
	ev.InstanceStatusQueued:       {ev.InstanceStatusInitializing: true},
	ev.InstanceStatusInitializing: {
		ev.InstanceStatusRunning: true, ev.InstanceStatusFailed: true, ev.InstanceStatusTimeout: true,
	},
	ev.InstanceStatusRunning: {
		ev.InstanceStatusVerifying: true, ev.InstanceStatusFailed: true, ev.InstanceStatusTimeout: true,
	},
	ev.InstanceStatusVerifying: {
		ev.InstanceStatusPassed: true, ev.InstanceStatusFailed: true, ev.InstanceStatusTimeout: true,
	},
	ev.InstanceStatusFailed:  {ev.InstanceStatusPending: true},
	ev.InstanceStatusTimeout: {ev.InstanceStatusPending: true},
}

func TransitInstance(from, to ev.InstanceStatus) error {
	if allowed, ok := instanceWhitelist[from]; ok && allowed[to] {
		return nil
	}
	return fmt.Errorf("非法实例流转: %s → %s", from, to)
}

var taskWhitelist = map[ev.TaskStatus]map[ev.TaskStatus]bool{
	ev.TaskStatusPending: {
		ev.TaskStatusRunning: true, ev.TaskStatusSucceeded: true, ev.TaskStatusPartialFailed: true,
	},
	ev.TaskStatusRunning: {
		ev.TaskStatusSucceeded: true, ev.TaskStatusPartialFailed: true,
	},
	ev.TaskStatusSucceeded:     {ev.TaskStatusRunning: true},
	ev.TaskStatusPartialFailed: {ev.TaskStatusRunning: true},
}

func TransitTask(from, to ev.TaskStatus) error {
	if allowed, ok := taskWhitelist[from]; ok && allowed[to] {
		return nil
	}
	return fmt.Errorf("非法任务流转: %s → %s", from, to)
}
