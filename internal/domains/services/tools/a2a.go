package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mooc-manus/internal/domains/models"
	"mooc-manus/pkg/logger"
	"net/http"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
	"go.uber.org/zap"
)

type a2aToolFunction string

const (
	getRemoteAgentCards a2aToolFunction = "get_remote_agent_cards"
	callRemoteAgent     a2aToolFunction = "call_remote_agent"
)

const (
	AgentCardPattern        = "%sAgent的卡片信息如下：\n"
	AgentIdPattern          = "AgentID：%s\n"
	AgentDescriptionPattern = "Agent描述：%s\n"
	skillIdxPattern         = "skill %d；"
	skillNamePattern        = "名称：%s；"
	skillDescriptionPattern = "描述：%s；"
	skillTagsPattern        = "工具调用参数：%s\n"
)

type A2ATool struct {
	BaseTool
	srvCfgs      []models.A2AServerConfigDO
	id2AgentCard map[string]*a2a.AgentCard
	id2A2AClient map[string]*a2aclient.Client
}

func NewA2ATool(provider models.ToolProviderDO, functions []models.ToolFunctionDO, srvCfgs []models.A2AServerConfigDO) Tool {
	a2aTool := &A2ATool{}
	a2aTool.providerId = provider.ProviderID
	a2aTool.providerName = provider.ProviderName
	a2aTool.providerType = "A2A"
	a2aTool.functions = functions
	a2aTool.srvCfgs = srvCfgs
	a2aTool.id2AgentCard = make(map[string]*a2a.AgentCard)
	a2aTool.id2A2AClient = make(map[string]*a2aclient.Client)

	return a2aTool
}

func (a *A2ATool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	switch a2aToolFunction(funcName) {
	case getRemoteAgentCards:
		return a.getRemoteAgentCards()
	case callRemoteAgent:
		args := struct {
			Id    string `json:"id"`
			Query string `json:"query"`
		}{}
		if err := json.Unmarshal([]byte(funcArgs), &args); err != nil {
			logger.Error("unmarshal call_remote_agent args failed", zap.Error(err), zap.String("func_args", funcArgs))
			return models.ToolCallResult{
				Success: false,
				Message: fmt.Sprintf("call_remote_agent工具调用参数%s不符合规范：%s", funcArgs, err.Error()),
			}
		} else {
			return a.callRemoteAgent(args.Id, args.Query)
		}
	default:
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("A2A工具不支持%s工具调用", funcName),
		}
	}
}

// Init todo 这里a2a服务还没启动 肯定无法resolve出agentcard 目前采取的方式是在getRemoteAgentCards的时候懒加载
func (a *A2ATool) Init() error {
	return nil
}

func (t *A2ATool) getRemoteAgentCards() models.ToolCallResult {
	timeoutCtx, cancelFunc := context.WithTimeout(context.Background(), time.Second*30)
	defer cancelFunc()

	withInsecureJSONRPC := a2aclient.WithJSONRPCTransport(&http.Client{})
	for _, cfg := range t.srvCfgs {
		if _, ok := t.id2AgentCard[cfg.ID]; ok { // 如果已经存在那就不再加载 默认在程序运行期间子Agent的卡片信息不会发生变化
			continue
		}
		card, err := agentcard.DefaultResolver.Resolve(timeoutCtx, cfg.BaseURL)
		if err != nil {
			logger.Error("resolve agent card failed", zap.Error(err), zap.Any("a2a server config", cfg))
			continue
		}
		client, err := a2aclient.NewFromCard(timeoutCtx, card, withInsecureJSONRPC)
		if err != nil {
			logger.Error("create  client from agent card failed", zap.Error(err), zap.Any("agent card", card))
			continue
		}
		client.Destroy()
		t.id2AgentCard[cfg.ID] = card
		t.id2A2AClient[cfg.ID] = client
	}
	result := models.ToolCallResult{}
	result.Success = true
	result.Data = ConvertAgentCards2Message(t.id2AgentCard)
	logger.Info("invoke get_remote_agent_cards tool success", zap.Any("agent cards", result.Data))
	return result
}

func (t *A2ATool) callRemoteAgent(id, query string) models.ToolCallResult {
	result := models.ToolCallResult{}
	client := t.id2A2AClient[id]
	if client == nil {
		result.Success = false
		result.Message = fmt.Sprintf("远程Agent%s不存在", id)
		return result
	}

	// Send a message and log the response.
	timeoutCtx, cancelFunc := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelFunc()
	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: query})
	if resp, err := client.SendMessage(timeoutCtx, &a2a.MessageSendParams{Message: message}); err != nil {
		logger.Error("send message to remote agent failed", zap.Error(err), zap.String("agent_id", id), zap.String("query", query))
		result.Success = false
		result.Message = fmt.Sprintf("Agent调用失败：%s", err.Error())
	} else {
		result.Success = true
		result.Data = resp.(*a2a.Message).Parts[0].(a2a.TextPart).Text
		logger.Info("invoke call_remote_agent tool success", zap.String("agent_id", id), zap.String("query", query),
			zap.Any("response", result.Data))
	}
	return result
}

func ConvertAgentCard2Message(id string, card *a2a.AgentCard) string {
	var messageBuffer bytes.Buffer
	messageBuffer.WriteString(fmt.Sprintf(AgentIdPattern, id))
	messageBuffer.WriteString(fmt.Sprintf(AgentDescriptionPattern, card.Description))

	skills := card.Skills
	if len(skills) > 0 {
		messageBuffer.WriteString("Agent具备的skills如下：\n")
		for idx, skill := range skills {
			messageBuffer.WriteString(fmt.Sprintf(skillIdxPattern, idx+1))
			messageBuffer.WriteString(fmt.Sprintf(skillNamePattern, skill.Name))
			messageBuffer.WriteString(fmt.Sprintf(skillDescriptionPattern, skill.Description))
			messageBuffer.WriteString(fmt.Sprintf(skillTagsPattern, strings.Join(skill.Tags, ",")))
		}

	}
	return messageBuffer.String()
}

func ConvertAgentCards2Message(id2Cards map[string]*a2a.AgentCard) string {
	var messageBuffer bytes.Buffer
	if len(id2Cards) == 0 {
		messageBuffer.WriteString("未查询到任务可远程调用的Agent卡片信息")
	} else {
		messageBuffer.WriteString("可远程调用的Agent卡片信息如下：\n")
		for id, card := range id2Cards {
			messageBuffer.WriteString(fmt.Sprintf(AgentCardPattern, card.Name))
			messageBuffer.WriteString(ConvertAgentCard2Message(id, card))
			messageBuffer.WriteString("------------------------------\n")
		}
	}

	return messageBuffer.String()
}
