package events

import (
	"time"

	"mooc-manus/internal/domains/models/agents"

	"github.com/google/uuid"
)

// PlanEvent 规划事件类型
type PlanEvent struct {
	BaseEvent
	Plan   agents.Plan     `json:"plan"`   // 规划信息
	Status PlanEventStatus `json:"status"` // 规划事件状态
}

// StepEvent 子任务/步骤事件
type StepEvent struct {
	BaseEvent
	Step   agents.Step     `json:"step"`   // 步骤信息
	Status StepEventStatus `json:"status"` // 步骤执行的状态
}

func OnPlanCreateSuccess(plan agents.Plan) *PlanEvent {
	ev := PlanEvent{}
	ev.ID = uuid.New().String()
	ev.Type = EventTypePlanCreateSuccess
	ev.CreatedAt = time.Now()
	ev.Plan = plan
	ev.Status = PlanCreated
	return &ev
}

func OnPlanUpdateSuccess(plan agents.Plan) *PlanEvent {
	ev := PlanEvent{}
	ev.ID = uuid.New().String()
	ev.Type = EventTypePlanUpdateSuccess
	ev.CreatedAt = time.Now()
	ev.Plan = plan
	ev.Status = PlanUpdated
	return &ev
}

func OnPlanUpdateFailed(plan agents.Plan) *PlanEvent {
	ev := PlanEvent{}
	ev.ID = uuid.New().String()
	ev.Type = EventTypePlanUpdateFailed
	ev.CreatedAt = time.Now()
	ev.Plan = plan
	ev.Status = PlanFailed // 保持原有状态枚举不扩展；失败语义用 Type 区分
	return &ev
}

func OnStepStart(step agents.Step) *StepEvent {
	ev := StepEvent{}
	ev.ID = uuid.New().String()
	ev.Type = EventTypeStepStart
	ev.CreatedAt = time.Now()
	ev.Step = step
	ev.Status = StepStarted
	return &ev
}

func OnStepComplete(step agents.Step) *StepEvent {
	ev := StepEvent{}
	ev.ID = uuid.New().String()
	ev.Type = EventTypeStepComplete
	ev.CreatedAt = time.Now()
	ev.Step = step
	ev.Status = StepCompleted
	return &ev
}

func OnStepFail(step agents.Step) *StepEvent {
	ev := StepEvent{}
	ev.ID = uuid.New().String()
	ev.Type = EventTypeStepFail
	ev.CreatedAt = time.Now()
	ev.Step = step
	ev.Status = StepFailed
	return &ev
}
