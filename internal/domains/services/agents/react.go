package agents

import (
	"encoding/json"
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/prompts"
	"mooc-manus/pkg/logger"
	"strings"
	"sync"

	"github.com/kaptinlin/jsonrepair"
	"go.uber.org/zap"
)

type ReActAgent struct {
	BaseAgent
}

func NewReActAgent(baseAgent *BaseAgent) *ReActAgent {
	agent := &ReActAgent{}
	agent.agentConfig = baseAgent.agentConfig
	agent.llm = baseAgent.llm
	agent.memory = baseAgent.memory
	agent.tools = baseAgent.tools
	agent.retryInterval = 5
	agent.systemPrompt = prompts.GetReActSystemPrompt()

	return agent
}

func (ra *ReActAgent) ExecuteStep(plan agents.Plan, step *agents.Step, request agents.ChatRequest, eventCh chan<- events.AgentEvent) {
	// 1. 构建执行智能体提示词
	query := request.Query
	attachments := make([]string, 0, len(request.Files))
	for _, file := range request.Files {
		attachments = append(attachments, file.FilePath)
	}
	query = strings.ReplaceAll(prompts.GetExecutionPrompt(), messagePlaceHolder, query)
	query = strings.ReplaceAll(query, attachmentsPlanHolder, strings.Join(attachments, "\n"))
	query = strings.ReplaceAll(query, languagePlaceHolder, plan.Language)
	query = strings.ReplaceAll(query, stepPlaceHolder, step.Description)

	// 2. 更新步骤的执行状态并返回Step事件
	step.Status = agents.ExecuteStatusRunning
	eventCh <- events.OnStepStart(*step)

	// 3.调用invoke获取agent返回的事件内容
	var wg sync.WaitGroup
	wg.Add(1)
	execCh := make(chan events.AgentEvent)
	go func() {
		ra.Invoke(query, execCh) // 这里使用阻塞调用react智能体即可
		wg.Done()
	}()

	for event := range execCh {
		switch event.EventType() {
		case events.EventTypeMessage:
			messageEvent := event.(*events.MessageEvent)
			repairJson, err := jsonrepair.JSONRepair(messageEvent.Message)
			if err != nil {
				logger.Error("repair json for react step execution message error", zap.Error(err), zap.String("message", messageEvent.Message))
				eventCh <- events.OnError("react智能体执行步骤的返回结果不满足格式要求")
				close(eventCh)
				return
			}
			newStep := agents.Step{}
			if err := json.Unmarshal([]byte(repairJson), &newStep); err != nil {
				logger.Error("json unmarshal react step execution result error", zap.Error(err), zap.String("result", repairJson))
				eventCh <- events.OnError("react智能体执行步骤的返回结果不满足格式要求")
				close(eventCh)
				return
			}
			// 更新子步骤的数据
			step.Success = newStep.Success
			step.Result = newStep.Result
			step.Attachments = newStep.Attachments
			eventCh <- events.OnStepComplete(*step)
			if step.Result != "" {
				eventCh <- events.OnMessage(step.Result, nil)
			}
		case events.EventTypeError:
			errorEvent := event.(*events.ErrorEvent)
			step.Status = agents.ExecuteStatusFailed
			step.Error = errorEvent.Error
			eventCh <- events.OnStepFail(*step)
		default:
			eventCh <- event
		}
	}
	wg.Wait()

	// 循环表示子步骤已经执行完毕 需要更新状态
	step.Status = agents.ExecuteStatusCompleted
	close(eventCh)
}

func (ra *ReActAgent) Summarize(eventCh chan<- events.AgentEvent) {
	query := prompts.GetSummarizePrompt()

	// 调用invoke方法完成结果的summarize
	summaryCh := make(chan events.AgentEvent)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		ra.Invoke(query, summaryCh)
		wg.Done()
	}()
	for event := range summaryCh {
		if event.EventType() == events.EventTypeMessage {
			messageEvent := event.(*events.MessageEvent)
			logger.Info("summarize end", zap.String("summary", messageEvent.Message))
		}
		eventCh <- event
	}
	wg.Wait()
	close(eventCh)
}
