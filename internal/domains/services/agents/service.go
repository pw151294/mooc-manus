package agents

import (
	"fmt"
	"mooc-manus/internal/domains/events"
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/services"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/llm"
	"mooc-manus/pkg/logger"
	"sync"

	"go.uber.org/zap"
)

type BaseAgentDomainService interface {
	Chat(agents.AgentChatRequest, chan events.AgentEvent)
	CreatePlan(agents.AgentPlanCreateRequest, chan events.AgentEvent)
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

func (s *BaseAgentDomainServiceImpl) Chat(request agents.AgentChatRequest, eventCh chan events.AgentEvent) {
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

func (s *BaseAgentDomainServiceImpl) createBaseAgent(request agents.AgentChatRequest) (*BaseAgent, error) {
	appConfig, err := s.appConfigDomainSvc.GetById(request.AppConfigId)
	if err != nil {
		return nil, err
	}
	logger.Info("get app config", zap.Any("model config", appConfig.ModelConfig), zap.Any("agent config", appConfig.AgentConfig))
	appConfig.ModelConfig.ApiKey = request.ApiKey
	openAiLLM := llm.NewOpenAiLLM(appConfig.ModelConfig)
	chatMemory := memory.FetchMemory(request.ConversationId)

	// 初始化工具tools
	functions, err := s.functionDomainSvc.GetByIds(request.FunctionIds)
	if err != nil {
		return nil, err
	}
	logger.Info("get functions from function ids",
		zap.Strings("function ids", request.FunctionIds), zap.Any("functions", functions))

	providerIds := make([]string, 0, 0)
	providerId2Functions := make(map[string][]models.ToolFunctionDO)
	for _, f := range functions {
		providerId := f.ProviderID
		if funcs, ok := providerId2Functions[providerId]; ok {
			providerId2Functions[providerId] = append(funcs, f)
		} else {
			providerIds = append(providerIds, providerId)
			providerId2Functions[providerId] = []models.ToolFunctionDO{f}
		}
	}
	providers, err := s.providerDomainSvc.GetByIds(providerIds)
	if err != nil {
		return nil, err
	}
	logger.Info("get providers from functions",
		zap.Strings("provider ids", providerIds), zap.Any("providers", providers))

	baseTools := make([]tools.Tool, 0, 0)
	for _, provider := range providers {
		funcs := providerId2Functions[provider.ProviderID]
		var tool tools.Tool
		switch provider.ProviderType {
		case "MCP":
			tool = tools.NewMcpTool(provider, funcs)
		case "CUSTOM":
			tool = tools.NewCustomTool(provider, funcs)
		}
		if tool != nil {
			if err := tool.Init(); err != nil {
				return nil, err
			}
			baseTools = append(baseTools, tool)
		}
	}
	logger.Info("init tools success")

	return NewBaseAgent(appConfig.AgentConfig, openAiLLM, chatMemory, baseTools, request.SystemPrompt), nil
}
