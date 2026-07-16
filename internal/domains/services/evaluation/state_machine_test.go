package evaluation

import (
	"testing"

	ev "mooc-manus/internal/domains/models/evaluation"
)

func TestTransitInstance(t *testing.T) {
	cases := []struct {
		from, to ev.InstanceStatus
		wantErr  bool
	}{
		// 合法
		{ev.InstanceStatusPending, ev.InstanceStatusQueued, false},
		{ev.InstanceStatusQueued, ev.InstanceStatusInitializing, false},
		{ev.InstanceStatusInitializing, ev.InstanceStatusRunning, false},
		{ev.InstanceStatusInitializing, ev.InstanceStatusFailed, false},
		{ev.InstanceStatusRunning, ev.InstanceStatusVerifying, false},
		{ev.InstanceStatusRunning, ev.InstanceStatusFailed, false},
		{ev.InstanceStatusRunning, ev.InstanceStatusTimeout, false},
		{ev.InstanceStatusVerifying, ev.InstanceStatusPassed, false},
		{ev.InstanceStatusVerifying, ev.InstanceStatusFailed, false},
		{ev.InstanceStatusInitializing, ev.InstanceStatusTimeout, false}, // 巡检器
		{ev.InstanceStatusVerifying, ev.InstanceStatusTimeout, false},
		{ev.InstanceStatusFailed, ev.InstanceStatusPending, false},  // 重试
		{ev.InstanceStatusTimeout, ev.InstanceStatusPending, false}, // 重试
		// 非法
		{ev.InstanceStatusPending, ev.InstanceStatusRunning, true},
		{ev.InstanceStatusPassed, ev.InstanceStatusFailed, true},
		{ev.InstanceStatusPassed, ev.InstanceStatusPending, true}, // Passed 不允许重试
	}
	for _, c := range cases {
		err := TransitInstance(c.from, c.to)
		if (err != nil) != c.wantErr {
			t.Errorf("Transit(%s→%s) got err=%v want err=%v", c.from, c.to, err, c.wantErr)
		}
	}
}

func TestTransitTask(t *testing.T) {
	cases := []struct {
		from, to ev.TaskStatus
		wantErr  bool
	}{
		{ev.TaskStatusPending, ev.TaskStatusRunning, false},
		{ev.TaskStatusPending, ev.TaskStatusSucceeded, false},         // edge case: 全部瞬间终态
		{ev.TaskStatusPending, ev.TaskStatusPartialFailed, false},
		{ev.TaskStatusRunning, ev.TaskStatusSucceeded, false},
		{ev.TaskStatusRunning, ev.TaskStatusPartialFailed, false},
		{ev.TaskStatusSucceeded, ev.TaskStatusRunning, false},     // 重试
		{ev.TaskStatusPartialFailed, ev.TaskStatusRunning, false}, // 重试
		{ev.TaskStatusPending, ev.TaskStatusPending, true},        // 自环非法
		{ev.TaskStatusSucceeded, ev.TaskStatusPending, true},
	}
	for _, c := range cases {
		err := TransitTask(c.from, c.to)
		if (err != nil) != c.wantErr {
			t.Errorf("TransitTask(%s→%s) got err=%v want err=%v", c.from, c.to, err, c.wantErr)
		}
	}
}
