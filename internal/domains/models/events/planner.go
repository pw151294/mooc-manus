package events

import (
	"time"

	"mooc-manus/internal/domains/models/agents"

	"github.com/google/uuid"
)

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
