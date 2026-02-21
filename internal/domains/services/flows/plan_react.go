package flows

import (
	"fmt"
	"mooc-manus/internal/domains/models"
	agent "mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/llm"
	"mooc-manus/pkg/logger"
	"sync"

	"go.uber.org/zap"
)

type PlanReActFlow struct {
	status        FlowStatus
	sessionStatus SessionStatus
	sessionId     string
	plan          agent.Plan
	planner       *agents.PlanAgent
	executor      *agents.ReActAgent
}

func NewPlanReActFlow(agentConfig models.AgentConfig, llm *llm.OpenAiLLM, sessionId string, tools []tools.Tool) BaseFlow {
	flow := &PlanReActFlow{}
	flow.sessionId = sessionId
	flow.status = FlowStatusIdle
	flow.sessionStatus = SessionStatusPending
	plannerBaseAgent := agents.NewBaseAgent(agentConfig, llm, memory.FetchMemory("planner::"+sessionId), tools, "")
	flow.planner = agents.NewPlanAgent(plannerBaseAgent)
	reActBaseAgent := agents.NewBaseAgent(agentConfig, llm, memory.FetchMemory("react::"+sessionId), tools, "")
	flow.executor = agents.NewReActAgent(reActBaseAgent)

	return flow
}

func (f *PlanReActFlow) Invoke(request agent.ChatRequest, eventCh chan events.AgentEvent) {
	// 更新会话状态为执行中
	f.sessionStatus = SessionStatusRunning

	// 获取当前会话中的最新事件
	f.plan = events.GetLatestPlan(f.sessionId)
	logger.Info("get latest plan", zap.Any("latest plan", f.plan))
	var step *agent.Step
	logger.Info("begin flow run", zap.Any("request", request))
LOOP:
	for {
		var wg sync.WaitGroup
		flowEventCh := make(chan events.AgentEvent)
		switch f.status {
		case FlowStatusIdle: // 如果流的状态为空闲 将状态修改为规划中
			f.status = FlowStatusPlanning
			logger.Info(fmt.Sprintf(flowStatusTransitionLogPattern, FlowStatusIdle, FlowStatusPlanning))
		case FlowStatusPlanning: // 流的状态为规划中 调用createPlan开始创建计划
			wg.Add(1)
			logger.Info("begin plan creating......")
			go func() {
				f.planner.CreatePlan(request.Query, request.Files, flowEventCh)
				wg.Done()
			}()
			for event := range flowEventCh {
				if planEvent, ok := event.(*events.PlanEvent); ok && planEvent.Status == events.PlanCreated {
					f.plan = planEvent.Plan
					titleEvent := events.OnTitle(f.plan.Title)
					events.AddEvent(f.sessionId, titleEvent)
					eventCh <- titleEvent
					msgEvent := events.OnMessage(f.plan.Message, nil)
					events.AddEvent(f.sessionId, msgEvent)
					eventCh <- msgEvent
				}
				events.AddEvent(f.sessionId, event)
				eventCh <- event
			}
			wg.Wait()
			f.status = FlowStatusExecuting
			if len(f.plan.Steps) == 0 {
				f.status = FlowStatusCompleted
				logger.Info(fmt.Sprintf(flowStatusTransitionLogPattern, FlowStatusPlanning, FlowStatusCompleted))
			}
			logger.Info("end create plan", zap.Any("plan", f.plan))
			logger.Info(fmt.Sprintf(flowStatusTransitionLogPattern, FlowStatusPlanning, FlowStatusExecuting))
		case FlowStatusExecuting:
			f.plan.Status = agent.ExecuteStatusRunning
			// 查找第一个未执行完成步骤
			var firstPendingIdx = -1
			for idx := range f.plan.Steps {
				if f.plan.Steps[idx].Status == agent.ExecuteStatusPending || f.plan.Steps[idx].Status == agent.ExecuteStatusRunning {
					firstPendingIdx = idx
				}
			}
			if firstPendingIdx != -1 {
				step = &f.plan.Steps[firstPendingIdx]
			}
			if step == nil {
				f.status = FlowStatusSummarizing
				logger.Info(fmt.Sprintf(flowStatusTransitionLogPattern, FlowStatusExecuting, FlowStatusSummarizing))
				continue
			}
			// 调用react执行该步骤
			wg.Add(1)
			go func() {
				logger.Info("begin execute step", zap.Any("step", step))
				f.executor.ExecuteStep(f.plan, step, request, flowEventCh)
				wg.Done()
			}()
			for event := range flowEventCh {
				events.AddEvent(f.sessionId, event)
				eventCh <- event
			}
			wg.Wait()
			f.plan.Steps[firstPendingIdx] = *step
			logger.Info("end execute step", zap.Any("step", step), zap.Any("plan", f.plan))
			f.status = FlowStatusUpdating
			logger.Info(fmt.Sprintf(flowStatusTransitionLogPattern, FlowStatusExecuting, FlowStatusUpdating))
		case FlowStatusUpdating:
			wg.Add(1)
			go func() {
				f.planner.UpdatePlan(f.plan, *step, flowEventCh)
				wg.Done()
			}()
			for event := range flowEventCh {
				if planEvent, ok := event.(*events.PlanEvent); ok && planEvent.Status == events.PlanUpdated {
					f.plan = planEvent.Plan
				}
				events.AddEvent(f.sessionId, event)
				eventCh <- event
			}
			wg.Wait()
			logger.Info("end update plan", zap.Any("plan", f.plan))
			f.status = FlowStatusExecuting
			logger.Info(fmt.Sprintf(flowStatusTransitionLogPattern, FlowStatusUpdating, FlowStatusExecuting))
		case FlowStatusSummarizing:
			wg.Add(1)
			go func() {
				f.executor.Summarize(flowEventCh)
				wg.Done()
			}()
			for event := range flowEventCh {
				events.AddEvent(f.sessionId, event)
				eventCh <- event
			}
			wg.Wait()
			f.status = FlowStatusCompleted
			logger.Info(fmt.Sprintf(flowStatusTransitionLogPattern, FlowStatusSummarizing, FlowStatusCompleted))
		case FlowStatusCompleted:
			f.plan.Status = agent.ExecuteStatusCompleted
			f.status = FlowStatusIdle
			planCompleteEvent := events.OnPlanComplete(f.plan)
			events.AddEvent(f.sessionId, planCompleteEvent)
			eventCh <- planCompleteEvent
			close(flowEventCh)
			logger.Info("flow run completed")
			break LOOP
		}
	}

	doneEvent := events.OnDone()
	events.AddEvent(f.sessionId, doneEvent)
	eventCh <- doneEvent
	close(eventCh)
}

func (f *PlanReActFlow) Done() bool {
	return f.status == FlowStatusCompleted
}
