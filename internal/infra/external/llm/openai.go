package llm

import (
	"context"
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

func (l *OpenAiLLM) Invoke(messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (openai.ChatCompletionMessage, error) {
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
		return openai.ChatCompletionMessage{}, err
	}
	return completion.Choices[0].Message, nil
}

func (l *OpenAiLLM) StreamingInvoke(messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam, eventCh chan<- events.AgentEvent) openai.ChatCompletionMessage {
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
		content := chunk.Choices[0].Delta.Content
		if content != "" {
			logger.Debug("got chunk during streaming chat", zap.String("content", content))
			eventCh <- events.OnMessage(content, nil)
		}
		acc.AddChunk(chunk)
	}
	if stream.Err() != nil {
		eventCh <- events.OnError(stream.Err().Error())
	}
	close(eventCh)
	if len(acc.Choices) == 0 {
		return openai.ChatCompletionMessage{}
	} else {
		return acc.Choices[0].Message
	}
}
