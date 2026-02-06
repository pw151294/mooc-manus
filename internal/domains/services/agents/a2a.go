package agents

import (
	"errors"
	"fmt"
	"mooc-manus/internal/domains/events"
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/models/prompts"
	"mooc-manus/internal/domains/services"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/llm"
	"mooc-manus/pkg/logger"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/a2asrv"
	"go.uber.org/zap"
)

type A2ADomainService interface {
	A2AChat(agents.ChatRequest, chan events.AgentEvent)
}

type A2ADomainServiceImpl struct {
	id2A2AExecutor     map[string]*A2AExecutor
	agentDomainSvc     BaseAgentDomainService
	appConfigDomainSvc services.AppConfigDomainService
	providerDomainSvc  services.ToolProviderDomainService
	functionDomainSvc  services.ToolFunctionDomainService
}

func NewA2ADomainService(agentDomainSvc BaseAgentDomainService, appConfigDomainSvc services.AppConfigDomainService,
	providerDomainSvc services.ToolProviderDomainService, functionDomainSvc services.ToolFunctionDomainService) A2ADomainService {
	return &A2ADomainServiceImpl{
		id2A2AExecutor:     make(map[string]*A2AExecutor),
		agentDomainSvc:     agentDomainSvc,
		appConfigDomainSvc: appConfigDomainSvc,
		providerDomainSvc:  providerDomainSvc,
		functionDomainSvc:  functionDomainSvc,
	}
}

func (s *A2ADomainServiceImpl) A2AChat(request agents.ChatRequest, eventCh chan events.AgentEvent) {
	// 先创建入口Agent智能体
	request.SystemPrompt = prompts.GetA2ASystemPrompt()
	agentDomainSvcImpl := s.agentDomainSvc.(*BaseAgentDomainServiceImpl)
	agent, err := agentDomainSvcImpl.createBaseAgent(request)
	if err != nil {
		eventCh <- events.OnError(fmt.Sprintf("初始化Agent智能体失败：%s", err.Error()))
		close(eventCh)
		return
	}
	logger.Info("create chat agent success", zap.Any("chat request", request))

	appConfig, err := s.appConfigDomainSvc.GetById(request.AppConfigId)
	if err != nil {
		eventCh <- events.OnError(fmt.Sprintf("查询app配置失败：%s", err.Error()))
		close(eventCh)
		return
	}

	// 启动所有a2a服务
	if err := s.startA2AServers(request.ConversationId, appConfig); err != nil {
		eventCh <- events.OnError(fmt.Sprintf("启动a2a服务失败：%s", err.Error()))
		close(eventCh)
		return
	}

	// 调用智能体的Invoke接口
	agentEventCh := make(chan events.AgentEvent)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		agent.StreamingInvoke(request.Query, agentEventCh)
		wg.Done()
	}()
	go func() {
		for event := range agentEventCh {
			eventCh <- event
		}
		wg.Wait()
		close(eventCh)
	}()
}

func (s *A2ADomainServiceImpl) createA2AExecutor(conversationId string, appConfig models.AppConfigDO, srvCfg models.A2AServerConfigDO) (*A2AExecutor, error) {
	// 初始化工具
	funcIds := make([]string, 0, len(srvCfg.Skills))
	for _, skill := range srvCfg.Skills {
		funcIds = append(funcIds, skill.ID)
	}
	providers, err := s.providerDomainSvc.GetByFunctionAndProviderIds(funcIds, nil)
	if err != nil {
		logger.Error("failed to get tool providers by function ids", zap.Error(err), zap.Strings("function_ids", funcIds))
		return nil, err
	}
	proId2Funcs, err := s.functionDomainSvc.GroupFuncsByProviderId(funcIds, nil)
	if err != nil {
		logger.Error("failed to group functions by provider id", zap.Error(err), zap.Strings("function_ids", funcIds))
		return nil, err
	}
	// 根据app配置id查询出a2a服务配置
	srvCfgs, err := s.appConfigDomainSvc.GetA2AServers(appConfig.AppConfigID)
	if err != nil {
		logger.Error("failed to get a2a servers", zap.Error(err), zap.String("app_config_id", appConfig.AppConfigID))
		return nil, err
	}
	baseTools, err := tools.InitTools(providers, proId2Funcs, srvCfgs)
	if err != nil {
		logger.Error("failed to init tools", zap.Error(err), zap.Any("providers", providers), zap.Any("proId2Funcs", proId2Funcs))
		return nil, err
	}

	// 初始化baseAgent
	openAiLLM := llm.NewOpenAiLLM(appConfig.ModelConfig)
	agentConversationId := fmt.Sprintf("%s::%s", srvCfg.ID, conversationId)
	chatMemory := memory.FetchMemory(agentConversationId)
	baseAgent := NewBaseAgent(appConfig.AgentConfig, openAiLLM, chatMemory, baseTools, "")
	return &A2AExecutor{
		agentCard: agents.ConvertA2AServerConfig2AgentCard(srvCfg),
		agent:     baseAgent,
	}, nil
}

func (s *A2ADomainServiceImpl) startA2AServer(conversationId string, appConfig models.AppConfigDO, srvCfg models.A2AServerConfigDO) error {
	if _, ok := s.id2A2AExecutor[srvCfg.ID]; ok {
		return nil
	}
	executor, err := s.createA2AExecutor(conversationId, appConfig, srvCfg)
	if err != nil {
		logger.Error("failed to create a2a executor", zap.Error(err), zap.String("conversation_id", conversationId), zap.Any("app_config", appConfig), zap.Any("server_config", srvCfg))
		return err
	}
	logger.Info("create executor success", zap.Any("agent card", executor.agentCard))
	parsedURL, err := url.Parse(srvCfg.BaseURL)
	if err != nil {
		logger.Error("failed to parse base url", zap.Error(err), zap.String("base_url", srvCfg.BaseURL))
		return err
	}
	listen, err := net.Listen("tcp", parsedURL.Host)
	if err != nil {
		logger.Error("failed to listen on tcp address", zap.Error(err), zap.String("address", parsedURL.Host))
		return err
	}
	requestHandler := a2asrv.NewHandler(executor)
	mux := http.NewServeMux()
	mux.Handle("/invoke", a2asrv.NewJSONRPCHandler(requestHandler))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(executor.agentCard))

	// http.Server执行成功之后会阻塞 必须异步执行
	errCh := make(chan error, 1)
	go func() {
		if err := http.Serve(listen, mux); err != nil {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		logger.Error("failed to start a2a server", zap.Error(err), zap.Any("server_config", srvCfg))
		return err
	case <-time.After(100 * time.Millisecond):
		s.id2A2AExecutor[srvCfg.ID] = executor
		return nil
	}
}

func (s *A2ADomainServiceImpl) startA2AServers(conversationId string, appConfig models.AppConfigDO) error {
	srvCfgs, err := s.appConfigDomainSvc.GetA2AServers(appConfig.AppConfigID)
	if err != nil {
		logger.Error("failed to get a2a servers when starting servers", zap.Error(err), zap.String("app_config_id", appConfig.AppConfigID))
		return err
	}
	errs := make([]error, 0, 0)
	for _, cfg := range srvCfgs {
		if err := s.startA2AServer(conversationId, appConfig, cfg); err != nil {
			errs = append(errs, fmt.Errorf("a2a服务%s启动失败：%v", cfg.ID, err))
			continue
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	} else {
		return nil
	}
}
