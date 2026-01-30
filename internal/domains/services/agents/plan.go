package agents

import (
	"encoding/json"
	"fmt"
	"mooc-manus/internal/domains/events"
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/prompts"
	"mooc-manus/pkg/logger"
	"strings"
	"sync"

	"github.com/kaptinlin/jsonrepair"
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

func (pa *PlanAgent) CreatePlan(message string, files []agents.File, eventCh chan<- events.AgentEvent) {
	// 根据用户的提问拼装规划智能体的提示词
	attachments := make([]string, 0, len(files))
	for _, file := range files {
		attachments = append(attachments, file.FilePath)
	}
	query := strings.ReplaceAll(pa.planCreatePrompt, messagePlaceHolder, message)
	query = strings.ReplaceAll(query, attachmentsPlanHolder, strings.Join(attachments, "\n"))
	logger.Info("init plan create prompt success", zap.String("prompt", query))

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
			logger.Info("plan created", zap.String("plan", msg))
			plan := agents.Plan{}
			repairedMsg, err := jsonrepair.JSONRepair(msg)
			if err != nil {
				eventCh <- events.OnError(fmt.Sprintf("修复json字符串失败：%s", err.Error()))
			}
			if err := json.Unmarshal([]byte(repairedMsg), &plan); err != nil {
				eventCh <- events.OnError(fmt.Sprintf("大模型创建的计划格式不正确：%s", msg))
			} else {
				eventCh <- events.OnPlanCreateSuccess(plan)
			}
		default:
			logger.Info("get event during plan creating",
				zap.String("type", event.EventType()), zap.Any("data", event))
			eventCh <- event
		}
	}
	wg.Wait()
	close(eventCh)
}
