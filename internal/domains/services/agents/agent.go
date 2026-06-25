package agents

import (
	"fmt"
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/models/prompts/plans"
	"mooc-manus/internal/domains/services"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/file_storage"
	"mooc-manus/internal/infra/external/llm"
	"mooc-manus/internal/infra/repositories"
	"mooc-manus/pkg/logger"
	"strings"
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
	skillRepo          repositories.SkillRepository
	versionRepo        repositories.SkillVersionRepository
	storage            file_storage.FileStorage
}

func NewBaseAgentDomainService(
	appConfigDomainSvc services.AppConfigDomainService,
	providerDomainSvc services.ToolProviderDomainService,
	functionDomainSvc services.ToolFunctionDomainService,
	skillRepo repositories.SkillRepository,
	versionRepo repositories.SkillVersionRepository,
	storage file_storage.FileStorage,
) BaseAgentDomainService {
	return &BaseAgentDomainServiceImpl{
		appConfigDomainSvc: appConfigDomainSvc,
		providerDomainSvc:  providerDomainSvc,
		functionDomainSvc:  functionDomainSvc,
		skillRepo:          skillRepo,
		versionRepo:        versionRepo,
		storage:            storage,
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

	// 追加 Skill 内置工具（仅在 SkillRefs 非空时）
	if len(request.SkillRefs) > 0 {
		skillTools, err := tools.SkillTools(s.skillRepo, s.versionRepo, s.storage, request.SkillRefs)
		if err != nil {
			logger.Error("init skill tools failed", zap.Error(err))
			return nil, err
		}
		baseTools = append(baseTools, skillTools...)
		logger.Info("init skill tools success", zap.Int("skill_count", len(request.SkillRefs)))
	}

	// 构建系统提示词（拼接 Skill 元信息）
	systemPrompt := request.SystemPrompt
	if len(request.SkillRefs) > 0 {
		skillsPrompt, err := s.buildSkillsSystemPrompt(request.SkillRefs)
		if err != nil {
			logger.Warn("build skills system prompt failed", zap.Error(err))
			// 失败时仍可继续，但记录警告
		} else if skillsPrompt != "" {
			// 拼接顺序：原 systemPrompt + "\n\n" + skillsPrompt（对齐 Beedance BaseAgent.java:863-864）
			systemPrompt = systemPrompt + "\n\n" + skillsPrompt
			logger.Info("skills system prompt injected", zap.Int("skill_count", len(request.SkillRefs)))
		}
	}

	return NewBaseAgent(appConfig.AgentConfig, openAiLLM, chatMemory, baseTools, systemPrompt), nil
}

// skillUsageRules Skill 使用规则常量（对齐 Beedance BaseAgent.java:163-179）
const skillUsageRules = `
### Skill Usage Rules (MUST follow)

**Step 1 (Required):** Call ` + "`loadSkill(skillName)`" + ` to read the Skill documentation.

**Step 2:** Follow the Skill documentation's instructions to determine your next action:
- If the documentation defines an **output format/syntax** (e.g., Markdown blocks, custom tags): output that syntax directly in your response. Do NOT call ` + "`executeSkill`" + `.
- If the documentation requires **script execution**: call ` + "`executeSkill(skillName, bash)`" + `. Write output files ONLY to ` + "`/workspace/outputs/`" + `.

**Constraints:**
- Always call ` + "`loadSkill`" + ` before ` + "`executeSkill`" + `. Never skip ` + "`loadSkill`" + `.
- ` + "`skillName`" + ` MUST match one of the names listed above exactly. Do NOT invent skill names.
- After a successful ` + "`executeSkill`" + ` that generates a file, do not repeat the file content or add download links in your response.
`

// buildSkillsSystemPrompt 构建 Skill 相关系统提示词段落
// 参考 Beedance BaseAgent.buildSkillsPromptSection (BaseAgent.java:881-912)
func (s *BaseAgentDomainServiceImpl) buildSkillsSystemPrompt(skillRefs []agents.SkillRef) (string, error) {
	if len(skillRefs) == 0 {
		return "", nil
	}

	// 从 SkillRef 中提取 skillName
	skillNames := make([]string, 0, len(skillRefs))
	for _, ref := range skillRefs {
		if ref.SkillName != "" {
			skillNames = append(skillNames, ref.SkillName)
		}
	}

	if len(skillNames) == 0 {
		return "", nil
	}

	// 批量查询 Skill 信息
	skillPOs, err := s.skillRepo.GetByNames(skillNames)
	if err != nil {
		return "", fmt.Errorf("query skills by names failed: %w", err)
	}

	if len(skillPOs) == 0 {
		logger.Warn("no skills found for provided skillNames", zap.Strings("skill_names", skillNames))
		return "", nil
	}

	// 拼接 Skill 列表（格式：- **{skillName}**: {description}）
	var skillListBuilder strings.Builder
	for _, po := range skillPOs {
		skillListBuilder.WriteString(fmt.Sprintf("- **%s**", po.SkillName))
		if po.Description != "" {
			skillListBuilder.WriteString(fmt.Sprintf(": %s", po.Description))
		}
		skillListBuilder.WriteString("\n")
	}

	// 拼接最终段落（对齐 Beedance BaseAgent.java:906-911）
	return fmt.Sprintf(`## Available Skills
You have access to the following skills.

%s
%s`, skillListBuilder.String(), skillUsageRules), nil
}
