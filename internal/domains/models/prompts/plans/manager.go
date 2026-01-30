package plans

import (
	"mooc-manus/internal/domains/models/agents"
	"sync"
)

var manager *PlanManager

type PlanManager struct {
	sync.Mutex
	id2Plan map[string]agents.Plan
	id2Step map[string]agents.Step
}

func SaveOrUpdate(plan agents.Plan) {
	manager.Lock()
	defer manager.Unlock()
	// 如果是从更新操作，先清理旧的Step映射，防止Step被删除后造成残留
	if oldPlan, ok := manager.id2Plan[plan.ID]; ok {
		for _, step := range oldPlan.Steps {
			delete(manager.id2Step, step.ID)
		}
	}

	manager.id2Plan[plan.ID] = plan
	// 同步更新新的Steps到映射中
	for _, step := range plan.Steps {
		manager.id2Step[step.ID] = step
	}
}

func DeletePlanById(id string) {
	manager.Lock()
	defer manager.Unlock()
	// 删除Plan前先清理关联的Steps
	if plan, ok := manager.id2Plan[id]; ok {
		for _, step := range plan.Steps {
			delete(manager.id2Step, step.ID)
		}
		delete(manager.id2Plan, id)
	}
}

func GetPlanById(id string) (agents.Plan, bool) {
	manager.Lock()
	defer manager.Unlock()
	plan, ok := manager.id2Plan[id]
	return plan, ok
}

func GetStepById(id string) (agents.Step, bool) {
	manager.Lock()
	defer manager.Unlock()
	step, ok := manager.id2Step[id]
	return step, ok
}

func init() {
	manager = &PlanManager{
		id2Plan: make(map[string]agents.Plan),
		id2Step: make(map[string]agents.Step),
	}
}
