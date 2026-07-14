package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/interrupt"
	"mooc-manus/internal/domains/models/invoker"
	"mooc-manus/internal/domains/models/llm"
)

const SubagentToolName = "dispatchSubagent"

// SubagentParams 子智能体调用参数
type SubagentParams struct {
	TaskDescription      string   `json:"task_description"`
	Context              string   `json:"context"`
	AllowedTools         []string `json:"allowed_tools"`
	SystemPromptTemplate string   `json:"system_prompt_template"`
}

// SubagentResult 子智能体调用结果
type SubagentResult struct {
	Success   bool     `json:"success"`
	Output    string   `json:"output"`
	ToolCalls []string `json:"tool_calls"`
	Error     string   `json:"error,omitempty"`
}

// AgentRunConfig 子智能体运行配置
type AgentRunConfig struct {
	AgentConfig  models.AgentConfig
	Invoker      invoker.Invoker
	Tools        []Tool
	SystemPrompt string
	Query        string
	PendingSink  interrupt.PendingSink
	MessageId    string
}

// AgentRunner 子智能体执行函数签名。
// 由 application/domain 层注入，内部创建 BaseAgent 并调用 Invoke。
// eventCh 由 AgentRunner 内部 close。
type AgentRunner func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent)

// SubagentTool 子智能体调度工具
type SubagentTool struct {
	agentConfig   models.AgentConfig
	llmInvoker    invoker.Invoker
	baseTools     []Tool
	pendingSink   interrupt.PendingSink
	messageId     string
	parentEventCh chan<- events.AgentEvent
	agentRunner   AgentRunner
	timeout       time.Duration // 子智能体执行超时，默认 3 分钟
}

// NewSubagentTool 构造 SubagentTool
func NewSubagentTool(
	agentConfig models.AgentConfig,
	inv invoker.Invoker,
	baseTools []Tool,
	pendingSink interrupt.PendingSink,
	messageId string,
	parentEventCh chan<- events.AgentEvent,
	agentRunner AgentRunner,
) *SubagentTool {
	return &SubagentTool{
		agentConfig:   agentConfig,
		llmInvoker:    inv,
		baseTools:     baseTools,
		pendingSink:   pendingSink,
		messageId:     messageId,
		parentEventCh: parentEventCh,
		agentRunner:   agentRunner,
		timeout:       3 * time.Minute,
	}
}

// SetParentEventCh 设置父智能体事件通道（用于延迟注入）
// 在 BaseAgent.Invoke/StreamingInvoke 开始时调用，以支持子智能体事件透传
func (t *SubagentTool) SetParentEventCh(ch chan<- events.AgentEvent) {
	t.parentEventCh = ch
}

func (t *SubagentTool) GetTools() []llm.Tool {
	return []llm.Tool{
		{
			Name:        SubagentToolName,
			Description: "派遣一个子智能体执行特定子任务。子智能体拥有独立的对话上下文和受限的工具集。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_description": map[string]any{
						"type":        "string",
						"description": "子智能体需要完成的任务描述",
					},
					"context": map[string]any{
						"type":        "string",
						"description": "提供给子智能体的上下文信息",
					},
					"allowed_tools": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "子智能体可以使用的工具名称列表",
					},
					"system_prompt_template": map[string]any{
						"type":        "string",
						"description": "自定义子智能体的系统提示词模板",
					},
				},
				"required": []string{"task_description", "allowed_tools"},
			},
		},
	}
}

func (t *SubagentTool) HasTool(funcName string) bool {
	return funcName == SubagentToolName
}

func (t *SubagentTool) Init() error {
	return nil
}

func (t *SubagentTool) ProviderName() string {
	return "native"
}

func (t *SubagentTool) SupportsRiskAssessment() bool {
	return false
}

func (t *SubagentTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	return t.InvokeWithContext(context.Background(), funcName, funcArgs)
}

func (t *SubagentTool) InvokeWithContext(ctx context.Context, funcName, funcArgs string) models.ToolCallResult {
	// 参数解析
	var params SubagentParams
	if err := json.Unmarshal([]byte(funcArgs), &params); err != nil {
		return errorResult(fmt.Sprintf("参数解析失败: %v", err))
	}

	// task_description 不能为空
	if params.TaskDescription == "" {
		return errorResult("task_description 不能为空")
	}

	// 递归检测：allowed_tools 不能包含 dispatchSubagent
	for _, name := range params.AllowedTools {
		if name == SubagentToolName {
			return errorResult("allowed_tools 不能包含 dispatchSubagent（禁止递归调用）")
		}
	}

	// 白名单校验：allowed_tools 必须是 baseTools 中已注册工具的子集
	for _, name := range params.AllowedTools {
		found := false
		for _, tool := range t.baseTools {
			if tool.HasTool(name) {
				found = true
				break
			}
		}
		if !found {
			return errorResult(fmt.Sprintf("allowed_tools 包含未注册的工具: %s", name))
		}
	}

	// 子智能体执行逻辑
	return t.executeSubagent(ctx, params)
}

func (t *SubagentTool) executeSubagent(ctx context.Context, params SubagentParams) models.ToolCallResult {
	// 生成子智能体 ID
	subagentId := uuid.New().String()

	// 过滤工具集：只保留包含 allowedTools 中函数名的 Tool provider
	subTools := filterTools(t.baseTools, params.AllowedTools)

	// 构造 query（如果有 context，拼接到 task_description 前面）
	query := params.TaskDescription
	if params.Context != "" {
		query = fmt.Sprintf("背景信息:\n%s\n\n任务:\n%s", params.Context, params.TaskDescription)
	}

	// 构造子智能体 messageId
	subMessageId := t.messageId + "-sub-" + subagentId

	// 创建事件桥接器
	bridge := NewSubagentEventBridge(t.parentEventCh, subagentId, params.TaskDescription, params.Context)

	// 设置子智能体超时
	subCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// 创建子智能体事件通道
	subEventCh := make(chan events.AgentEvent, 100)

	// 构造系统提示词
	systemPrompt := params.SystemPromptTemplate
	if systemPrompt == "" {
		systemPrompt = fmt.Sprintf("你是一个专注于执行特定任务的子智能体。\n任务: %s", params.TaskDescription)
	}

	// 构造运行配置
	cfg := AgentRunConfig{
		AgentConfig:  t.agentConfig,
		Invoker:      t.llmInvoker,
		Tools:        subTools,
		SystemPrompt: systemPrompt,
		Query:        query,
		PendingSink:  t.pendingSink,
		MessageId:    subMessageId,
	}

	// 启动子智能体（goroutine，因为 Invoke 是阻塞的且完成时 close eventCh）
	go t.agentRunner(subCtx, cfg, subEventCh)

	// 监听子智能体事件
	var finalText string
	var toolCallsSummary []string
	var lastError string

loop:
	for {
		select {
		case event, ok := <-subEventCh:
			if !ok {
				// 子智能体事件通道关闭，执行完毕
				break loop
			}
			// 通过 bridge 转发事件到父智能体
			bridge.ForwardEvent(event)
			// 根据事件类型记录结果
			switch e := event.(type) {
			case *events.MessageEvent:
				finalText = e.Message
			case *events.ToolEvent:
				if e.Status == events.ToolEventStatusCalling {
					toolCallsSummary = append(toolCallsSummary, e.FunctionName)
				}
			case *events.ErrorEvent:
				lastError = e.Error
			}
		case <-subCtx.Done():
			// 超时或被取消
			lastError = fmt.Sprintf("子智能体执行超时或被取消: %v", subCtx.Err())
			break loop
		}
	}

	// 构造返回结果
	if lastError != "" && finalText == "" {
		result := SubagentResult{
			Success:   false,
			Output:    "",
			ToolCalls: toolCallsSummary,
			Error:     lastError,
		}
		resultJSON, _ := json.Marshal(result)
		return models.ToolCallResult{Success: false, Data: string(resultJSON)}
	}

	result := SubagentResult{
		Success:   true,
		Output:    finalText,
		ToolCalls: toolCallsSummary,
	}
	resultJSON, _ := json.Marshal(result)
	return models.ToolCallResult{Success: true, Data: string(resultJSON)}
}

// filterTools 从 baseTools 中筛选包含 allowedNames 中任一函数名的 Tool provider
func filterTools(baseTools []Tool, allowedNames []string) []Tool {
	if len(allowedNames) == 0 {
		return nil
	}
	allowedSet := make(map[string]struct{}, len(allowedNames))
	for _, name := range allowedNames {
		allowedSet[name] = struct{}{}
	}

	var result []Tool
	for _, tool := range baseTools {
		for _, t := range tool.GetTools() {
			if _, ok := allowedSet[t.Name]; ok {
				result = append(result, tool)
				break
			}
		}
	}
	return result
}

func errorResult(msg string) models.ToolCallResult {
	result := SubagentResult{
		Success: false,
		Error:   msg,
	}
	resultJSON, _ := json.Marshal(result)
	return models.ToolCallResult{Success: false, Message: "Error: " + msg, Data: string(resultJSON)}
}
