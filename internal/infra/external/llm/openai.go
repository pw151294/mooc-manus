package llm

import (
	"context"
	"errors"
	"fmt"
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/pkg/logger"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"go.uber.org/zap"
)

type OpenAiLLM struct {
	client      openai.Client
	modelName   string
	temperature float64
	maxTokens   int64
	timeout     int
}

func NewOpenAiLLM(config models.ModelConfig) *OpenAiLLM {
	llm := &OpenAiLLM{}
	llm.modelName = config.ModelName
	llm.temperature = config.Temperature
	llm.maxTokens = config.MaxTokens
	llm.timeout = 60
	llm.client = openai.NewClient(
		option.WithBaseURL(config.BaseUrl),
		option.WithAPIKey(config.ApiKey),
	)
	return llm
}

func (l *OpenAiLLM) Invoke(messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (openai.ChatCompletionMessage, openai.CompletionUsage, error) {
	params := openai.ChatCompletionNewParams{}
	params.Model = l.modelName
	params.Messages = messages
	if len(tools) > 0 {
		params.Tools = tools
	}
	timeoutCtx, cancelFunc := context.WithTimeout(context.Background(), time.Duration(l.timeout)*time.Second)
	defer cancelFunc()
	logger.Info("begin chat with llm", zap.String("model", l.modelName))
	completion, err := l.client.Chat.Completions.New(timeoutCtx, params)
	if err != nil {
		return openai.ChatCompletionMessage{}, openai.CompletionUsage{}, err
	}
	if len(completion.Choices) == 0 {
		return openai.ChatCompletionMessage{}, openai.CompletionUsage{}, fmt.Errorf("llm返回空响应")
	}
	return completion.Choices[0].Message, completion.Usage, nil
}

func (l *OpenAiLLM) StreamingInvoke(messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam, eventCh chan<- events.AgentEvent) (openai.ChatCompletionMessage, openai.CompletionUsage) {
	params := openai.ChatCompletionNewParams{}
	params.Model = l.modelName
	params.Messages = messages
	if len(tools) > 0 {
		params.Tools = tools
	}
	timeoutCtx, cancelFunc := context.WithTimeout(context.Background(), time.Duration(l.timeout)*time.Second)
	defer cancelFunc()

	logger.Info("begin streaming chat with llm", zap.String("model", l.modelName))
	stream := l.client.Chat.Completions.NewStreaming(timeoutCtx, params)
	acc := openai.ChatCompletionAccumulator{}
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)
		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			if content != "" {
				logger.Debug("got chunk during streaming chat", zap.String("content", content))
				eventCh <- events.OnMessage(content, nil)
			}
		}
	}
	if streamErr := stream.Err(); streamErr != nil {
		errMsg := streamErr.Error()
		if errors.Is(streamErr, context.DeadlineExceeded) {
			errMsg = fmt.Sprintf("llm streaming timeout after %ds: %s", l.timeout, errMsg)
			logger.Warn("llm streaming timeout", zap.Int("timeout_seconds", l.timeout), zap.String("model", l.modelName))
		} else {
			logger.Warn("llm streaming error", zap.Error(streamErr), zap.String("model", l.modelName))
		}
		eventCh <- events.OnError(errMsg)
	}
	close(eventCh)
	if len(acc.Choices) == 0 {
		return openai.ChatCompletionMessage{}, openai.CompletionUsage{}
	}
	return acc.Choices[0].Message, acc.Usage
}
