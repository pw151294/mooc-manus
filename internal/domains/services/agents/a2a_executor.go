package agents

import (
	"context"
	"errors"
	"fmt"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/pkg/logger"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"go.uber.org/zap"
)

type A2AExecutor struct {
	agentCard *a2a.AgentCard
	agent     *BaseAgent
}

func (e *A2AExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, q eventqueue.Queue) error {
	// 解析出提问
	query := reqCtx.Message.Parts[0].(a2a.TextPart).Text
	if query == "" {
		err := errors.New("query is empty")
		logger.Error("failed to execute a2a agent", zap.Error(err), zap.Any("request_context", reqCtx))
		return err
	}
	logger.Info("execute a2a agent", zap.String("query", query), zap.String("agent name", e.agentCard.Name))

	eventCh := make(chan events.AgentEvent)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		e.agent.Invoke(query, eventCh) // 这里使用阻塞模式完成子Agent调用即可
		wg.Done()
	}()

	var message string
	for event := range eventCh {
		switch event.EventType() {
		case events.EventTypeMessage: // message事件 提取agent响应
			messageEvent := event.(*events.MessageEvent)
			message = messageEvent.Message
		case events.EventTypeError: // error事件 提取错误信息
			errorEvent := event.(*events.ErrorEvent)
			message = fmt.Sprintf("调用Agent%s错误：%s", e.agentCard.Name, errorEvent.Error)
		default:
			continue // 子Agent的其他事件不做上报
		}
	}
	wg.Wait()
	if message == "" {
		message = fmt.Sprintf("Agent%s针对提问%s没有做出任何回复", e.agentCard.Name, query)
	}
	response := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: message})
	if err := q.Write(ctx, response); err != nil {
		logger.Error("failed to write response to event queue", zap.Error(err), zap.Any("response", response))
		return err
	}
	logger.Info("execute a2a agent success", zap.String("response", message))
	return nil
}

func (e *A2AExecutor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, q eventqueue.Queue) error {
	return nil
}
