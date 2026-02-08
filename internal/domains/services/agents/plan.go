package agents

import (
	"encoding/json"
	"fmt"
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/prompts"
	"mooc-manus/internal/domains/models/prompts/plans"
	"mooc-manus/pkg/logger"
	"slices"
	"strings"
	"sync"

	"go.uber.org/zap"
)

type PlanAgent struct {
	BaseAgent
	systemPrompt     string
	planSystemPrompt string
	planCreatePrompt string
	planUpdatePrompt string
}

func NewPlanAgent(baseAgent *BaseAgent) *PlanAgent {
	agent := &PlanAgent{}
	agent.agentConfig = baseAgent.agentConfig
	agent.llm = baseAgent.llm
	agent.memory = baseAgent.memory
	agent.tools = baseAgent.tools
	agent.retryInterval = 5
	agent.systemPrompt = prompts.GetSystemPrompt()
	agent.planSystemPrompt = prompts.GetPlanSystemPrompt()
	agent.planCreatePrompt = prompts.GetPlanCreatePrompt()
	agent.planUpdatePrompt = prompts.GetPlanUpdatePrompt()

	return agent
}

func (pa *PlanAgent) CreatePlan(message string, files []models.File, eventCh chan<- events.AgentEvent) {
	// 根据用户的提问拼装规划智能体的提示词
	attachments := make([]string, 0, len(files))
	for _, file := range files {
		attachments = append(attachments, file.FilePath)
	}
	query := strings.ReplaceAll(pa.planCreatePrompt, messagePlaceHolder, message)
	query = strings.ReplaceAll(query, attachmentsPlanHolder, strings.Join(attachments, "\n"))
	logger.Info("init plans create prompt success", zap.String("prompt", query))

	agentEventCh := make(chan events.AgentEvent)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		pa.Invoke(query, agentEventCh) // 这一步直接阻塞调用让大模型直接输出计划即可 需要监听agentEventCh上报的event
		wg.Done()
	}()
	for event := range agentEventCh {
		switch event.EventType() {
		case events.EventTypeMessage:
			messageEvent := event.(*events.MessageEvent)
			msg := messageEvent.Message
			logger.Info("plans created", zap.String("plans", msg))
			plan, err := agents.ConvertMessage2Plan(msg)
			if err != nil {
				eventCh <- events.OnError(fmt.Sprintf("Agent创建的计划格式不正确：%s", err.Error()))
				continue
			}
			plans.SaveOrUpdate(plan)
			eventCh <- events.OnPlanCreateSuccess(plan)
		default:
			logger.Info("get event during plans creating",
				zap.String("type", event.EventType()), zap.Any("data", event))
			eventCh <- event
		}
	}
	wg.Wait()
	close(eventCh)
}

func (pa *PlanAgent) UpdatePlan(plan agents.Plan, step agents.Step, eventCh chan<- events.AgentEvent) {
	planBytes, err := json.Marshal(plan)
	if err != nil {
		eventCh <- events.OnPlanUpdateFailed(plan)
		close(eventCh)
		return
	}
	stepBytes, err := json.Marshal(step)
	if err != nil {
		eventCh <- events.OnPlanUpdateFailed(plan)
		close(eventCh)
		return
	}
	query := strings.ReplaceAll(prompts.GetPlanUpdatePrompt(), planPlaceHolder, string(planBytes))
	query = strings.ReplaceAll(query, stepPlaceHolder, string(stepBytes))

	agentEventCh := make(chan events.AgentEvent)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		pa.Invoke(query, agentEventCh)
		wg.Done()
	}()

	for event := range agentEventCh {
		switch event.EventType() {
		case events.EventTypeMessage:
			messageEvent := event.(*events.MessageEvent)
			updatedPlan, err := agents.ConvertMessage2UpdatedPlan(messageEvent.Message)
			if err != nil {
				eventCh <- events.OnPlanUpdateFailed(plan)
				continue
			}
			newSteps := make([]agents.Step, len(updatedPlan.Steps))
			copy(newSteps, updatedPlan.Steps)

			// 查询旧计划中第一个未完成的计划
			pendingStatus := []agents.ExecutionStatus{agents.Pending, agents.Running}
			firstPendingIdx := -1
			for idx, stp := range plan.Steps {
				if slices.Contains(pendingStatus, stp.Status) {
					firstPendingIdx = idx
					break
				}
			}
			// 更新所有未完成的计划
			if firstPendingIdx != -1 {
				plan.Steps = append(plan.Steps[:firstPendingIdx], newSteps...)
				plans.SaveOrUpdate(updatedPlan)
			}
			eventCh <- events.OnPlanUpdateSuccess(plan)
		default:
			eventCh <- event
		}
	}
	wg.Wait()
	close(eventCh)
}
