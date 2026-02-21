package agents

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kaptinlin/jsonrepair"
)

type ExecutionStatus string

const (
	ExecuteStatusPending   ExecutionStatus = "PENDING"   // 空闲或者等待中
	ExecuteStatusRunning   ExecutionStatus = "RUNNING"   // 执行中
	ExecuteStatusCompleted ExecutionStatus = "COMPLETED" // 执行完成
	ExecuteStatusFailed    ExecutionStatus = "FAILED"    // 执行失败
)

type Step struct {
	ID          string          `json:"id"`               // 子任务ID
	Description string          `json:"description"`      // 步骤的描述信息
	Status      ExecutionStatus `json:"status"`           // 子任务的执行状态
	Result      string          `json:"result,omitempty"` // 执行结果
	Error       string          `json:"error,omitempty"`  // 错误信息
	Success     bool            `json:"success"`          // 子任务是否执行成功
	Attachments []string        `json:"attachments"`      // 附件列表信息
}

type Plan struct {
	ID       string          `json:"id"`              // 计划ID
	Title    string          `json:"title"`           // 任务标题
	Goal     string          `json:"goal"`            // 任务目标
	Language string          `json:"language"`        // 工作语言
	Steps    []Step          `json:"steps"`           // 步骤/子任务列表
	Message  string          `json:"message"`         // 用户传递的消息
	Status   ExecutionStatus `json:"status"`          // 规划的状态
	Error    string          `json:"error,omitempty"` // 错误信息
}

func ConvertMessage2Plan(message string) (Plan, error) {
	plan := Plan{}
	repairJson, err := jsonrepair.JSONRepair(message)
	if err != nil {
		return plan, err
	}
	if err := json.Unmarshal([]byte(repairJson), &plan); err != nil {
		return Plan{}, err
	}

	plan.ID = uuid.New().String()
	plan.Status = ExecuteStatusPending
	for i := range plan.Steps {
		plan.Steps[i].ID = uuid.New().String()
		plan.Steps[i].Status = ExecuteStatusPending
		plan.Steps[i].Success = false
	}

	return plan, nil
}

func ConvertMessage2UpdatedPlan(message string) (Plan, error) {
	updatedPlan := Plan{}
	repairJson, err := jsonrepair.JSONRepair(message)
	if err != nil {
		return Plan{}, err
	}
	if err := json.Unmarshal([]byte(repairJson), &updatedPlan); err != nil {
		return Plan{}, err
	}
	for i := range updatedPlan.Steps {
		updatedPlan.Steps[i].ID = uuid.New().String()
		updatedPlan.Steps[i].Status = ExecuteStatusPending // 新计划必然是未执行的
		updatedPlan.Steps[i].Success = false
	}
	return updatedPlan, nil
}
