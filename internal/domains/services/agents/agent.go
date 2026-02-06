package agents

import (
	"fmt"
	"mooc-manus/internal/domains/events"
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/models/prompts/plans"
	"mooc-manus/internal/domains/services"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/llm"
	"mooc-manus/pkg/logger"
	"sync"

	"go.uber.org/zap"
)

type BaseAgentDomainService interface {
	Chat(agents.ChatRequest, chan events.AgentEvent)
	CreatePlan(agents.AgentPlanCreateRequest, chan events.AgentEvent)
	UpdatePlan(agents.AgentPlanUpdateRequest, chan events.AgentEvent)
}

type BaseAgentDomainServiceImpl struct {
	appConfigDomainSvc services.AppConfigDomainService
	providerDomainSvc  services.ToolProviderDomainService
	functionDomainSvc  services.ToolFunctionDomainService
}

func NewBaseAgentDomainService(appConfigDomainSvc services.AppConfigDomainService, providerDomainSvc services.ToolProviderDomainService, functionDomainSvc services.ToolFunctionDomainService) BaseAgentDomainService {
	return &BaseAgentDomainServiceImpl{
		appConfigDomainSvc: appConfigDomainSvc,
		providerDomainSvc:  providerDomainSvc,
		functionDomainSvc:  functionDomainSvc,
	}
}

func (s *BaseAgentDomainServiceImpl) Chat(request agents.ChatRequest, eventCh chan events.AgentEvent) {
	// 先调用createAgent创建Agent智能体
	agent, err := s.createBaseAgent(request)
	if err != nil {
		eventCh <- events.OnError(fmt.Sprintf("初始化智能体失败：%s", err.Error()))
		close(eventCh)
		return
	}
	logger.Info("init base agent success")

	// 调用智能体的Invoke接口
	agentEventCh := make(chan events.AgentEvent)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if request.Streaming {
			logger.Info("begin invoke agent streaming", zap.String("query", request.Query))
			agent.StreamingInvoke(request.Query, agentEventCh)
		} else {
			logger.Info("begin invoke agent", zap.String("query", request.Query))
			agent.Invoke(request.Query, agentEventCh)
		}
	}()
	go func() {
		for event := range agentEventCh {
			logger.Debug("get event from agent", zap.String("type", event.EventType()), zap.Any("data", event))
			eventCh <- event
		}
		wg.Wait()
		logger.Info("agent invoke end")
		close(eventCh)
	}()
}

func (s *BaseAgentDomainServiceImpl) CreatePlan(request agents.AgentPlanCreateRequest, eventCh chan events.AgentEvent) {
	chatReq := agents.ConvertPlanCreateRequest2ChatRequest(request)
	// 先创建规划智能体
	baseAgent, err := s.createBaseAgent(chatReq)
	if err != nil {
		eventCh <- events.OnError(fmt.Sprintf("初始化规划智能体失败：%s", err.Error()))
		close(eventCh)
		return
	}
	planAgent := NewPlanAgent(baseAgent)
	logger.Info("init plan agent success")

	// 调用规划智能体创建计划
	planEventCh := make(chan events.AgentEvent)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		planAgent.CreatePlan(request.Query, request.Files, planEventCh)
		wg.Done()
	}()
	for event := range planEventCh {
		eventCh <- event
	}
	wg.Wait()
	close(eventCh)
}

func (s *BaseAgentDomainServiceImpl) UpdatePlan(request agents.AgentPlanUpdateRequest, eventCh chan events.AgentEvent) {
	chatRequest := agents.ConvertPlanUpdateRequest2ChatRequest(request)
	// 创建规划智能体
	baseAgent, err := s.createBaseAgent(chatRequest)
	if err != nil {
		eventCh <- events.OnError(fmt.Sprintf("初始化规划智能体失败：%s", err.Error()))
		close(eventCh)
		return
	}
	planAgent := NewPlanAgent(baseAgent)
	logger.Info("init plan agent success")

	// 查询Plan和Step
	plan, ok := plans.GetPlanById(request.PlanId)
	if !ok {
		eventCh <- events.OnError(fmt.Sprintf("计划不存在：%s", request.PlanId))
		close(eventCh)
		return
	}
	step, ok := plans.GetStepById(request.StepId)
	if !ok {
		eventCh <- events.OnError(fmt.Sprintf("步骤不存在：%s", request.StepId))
		close(eventCh)
		return
	}

	// 调用规划智能体更新计划
	planEventCh := make(chan events.AgentEvent)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		planAgent.UpdatePlan(plan, step, planEventCh)
		wg.Done()
	}()
	for event := range planEventCh {
		eventCh <- event
	}
	wg.Wait()
	close(eventCh)
}

func (s *BaseAgentDomainServiceImpl) createBaseAgent(request agents.ChatRequest) (*BaseAgent, error) {
	appConfig, err := s.appConfigDomainSvc.GetById(request.AppConfigId)
	if err != nil {
		return nil, err
	}
	logger.Info("get app config", zap.Any("model config", appConfig.ModelConfig), zap.Any("agent config", appConfig.AgentConfig))
	openAiLLM := llm.NewOpenAiLLM(appConfig.ModelConfig)
	chatMemory := memory.FetchMemory(request.ConversationId)

	// 初始化工具tools
	providers, err := s.providerDomainSvc.GetByFunctionAndProviderIds(request.FunctionIds, request.ProviderIds)
	if err != nil {
		logger.Error("get providers failed", zap.Strings("function ids", request.ProviderIds), zap.Strings("provider ids", request.ProviderIds))
		return nil, err
	}
	proId2Funcs, err := s.functionDomainSvc.GroupFuncsByProviderId(request.FunctionIds, request.ProviderIds)
	if err != nil {
		logger.Error("group functions by provider id failed", zap.Error(err),
			zap.Strings("function ids", request.FunctionIds), zap.Strings("provider ids", request.ProviderIds))
		return nil, err
	}
	// 初始化a2a工具
	srvCfgs, err := s.appConfigDomainSvc.GetA2AServers(request.AppConfigId)
	if err != nil {
		logger.Error("get a2a servers failed", zap.Error(err), zap.String("app config id", request.AppConfigId))
	}
	baseTools, err := tools.InitTools(providers, proId2Funcs, srvCfgs)
	if err != nil {
		logger.Error("init tools failed", zap.Error(err))
		return nil, err
	}
	logger.Info("init tools success")

	return NewBaseAgent(appConfig.AgentConfig, openAiLLM, chatMemory, baseTools, request.SystemPrompt), nil
}
