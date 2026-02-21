package flows

import (
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/services"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/llm"
	"mooc-manus/pkg/logger"
	"sync"

	"go.uber.org/zap"
)

type BaseFlowDomainService interface {
	Run(agents.ChatRequest, chan events.AgentEvent)
}

type BaseFlowDomainServiceImpl struct {
	appConfigDomainSvc services.AppConfigDomainService
	providerDomainSvc  services.ToolProviderDomainService
	functionDomainSvc  services.ToolFunctionDomainService
}

func NewBaseFlowDomainService(appConfigDomainSvc services.AppConfigDomainService, providerDomainSvc services.ToolProviderDomainService, functionDomainSvc services.ToolFunctionDomainService) BaseFlowDomainService {
	return &BaseFlowDomainServiceImpl{
		appConfigDomainSvc: appConfigDomainSvc,
		providerDomainSvc:  providerDomainSvc,
		functionDomainSvc:  functionDomainSvc,
	}
}

func (s *BaseFlowDomainServiceImpl) Run(request agents.ChatRequest, eventCh chan events.AgentEvent) {
	// 创建flow工作流
	flow, err := s.createBaseFlow(request)
	if err != nil {
		logger.Error("create flow failed", zap.Error(err), zap.Any("chat request", request))
		eventCh <- events.OnError("创建工作流失败")
		close(eventCh)
		return
	}

	// 调用工作流运行接口
	var wg sync.WaitGroup
	wg.Add(1)
	flowEventCh := make(chan events.AgentEvent)
	go func() {
		flow.Invoke(request, flowEventCh)
		wg.Done()
	}()
	for event := range flowEventCh {
		eventCh <- event
	}
	wg.Wait()
	close(eventCh)
}

func (s *BaseFlowDomainServiceImpl) createBaseFlow(request agents.ChatRequest) (BaseFlow, error) {
	appConfig, err := s.appConfigDomainSvc.GetById(request.AppConfigId)
	if err != nil {
		return nil, err
	}
	logger.Info("get app config", zap.Any("model config", appConfig.ModelConfig), zap.Any("agent config", appConfig.AgentConfig))
	llm := llm.NewOpenAiLLM(appConfig.ModelConfig)

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
	baseTools, err := tools.InitTools(providers, proId2Funcs, nil)
	if err != nil {
		logger.Error("init tools failed", zap.Error(err))
		return nil, err
	}
	logger.Info("init tools success")

	return NewPlanReActFlow(appConfig.AgentConfig, llm, request.ConversationId, baseTools), nil
}
