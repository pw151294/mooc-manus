package tests

import (
	"context"
	"log"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

func MockOpenAiMessage() {
	messageParamUnion := openai.ChatCompletionMessageParamUnion{
		OfDeveloper: nil,
		OfSystem:    nil,
		OfUser:      nil,
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			Refusal: param.Opt[string]{},
			Name:    param.Opt[string]{},
			Audio: openai.ChatCompletionAssistantMessageParamAudio{
				ID: "",
			},
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: param.Opt[string]{},
				OfArrayOfContentParts: []openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
					{
						OfText: &openai.ChatCompletionContentPartTextParam{
							Text: "",
							Type: "",
						},
						OfRefusal: &openai.ChatCompletionContentPartRefusalParam{
							Refusal: "",
							Type:    "",
						},
					},
				},
			},
			ToolCalls: []openai.ChatCompletionMessageToolCallParam{
				{
					ID: "",
					Function: openai.ChatCompletionMessageToolCallFunctionParam{
						Arguments: "",
						Name:      "",
					},
					Type: "",
				},
			},
			Role: "",
		},
		OfTool: &openai.ChatCompletionToolMessageParam{
			Content: openai.ChatCompletionToolMessageParamContentUnion{
				OfString: param.Opt[string]{},
				OfArrayOfContentParts: []openai.ChatCompletionContentPartTextParam{
					{
						Text: "",
						Type: "",
					},
				},
			},
			ToolCallID: "",
			Role:       "",
		},
	}
	println(messageParamUnion)

	params := openai.ChatCompletionNewParams{
		Messages: nil,
		Model:    "",
		Tools: []openai.ChatCompletionToolParam{
			{
				Function: shared.FunctionDefinitionParam{
					Name:        "",
					Strict:      param.Opt[bool]{},
					Description: param.Opt[string]{},
					Parameters:  make(map[string]any),
				},
				Type: "function",
			},
		},
	}
	println(params)

	toolCallParam := openai.ChatCompletionMessageToolCallParam{
		ID: "",
		Function: openai.ChatCompletionMessageToolCallFunctionParam{
			Arguments: "",
			Name:      "",
		},
		Type: "",
	}
	println(toolCallParam)

	completionMessage := openai.ChatCompletionMessage{
		Content: "",
		Refusal: "",
		Role:    "",
		Annotations: []openai.ChatCompletionMessageAnnotation{
			{
				Type: "",
				URLCitation: openai.ChatCompletionMessageAnnotationURLCitation{
					EndIndex:   0,
					StartIndex: 0,
					Title:      "",
					URL:        "",
				},
			},
		},
		Audio: openai.ChatCompletionAudio{
			ID:         "",
			Data:       "",
			ExpiresAt:  0,
			Transcript: "",
		},
		ToolCalls: []openai.ChatCompletionMessageToolCall{
			{
				ID: "",
				Function: openai.ChatCompletionMessageToolCallFunction{
					Arguments: "",
					Name:      "",
				},
				Type: "",
			},
		},
	}
	println(completionMessage)
}

func TestMockA2ARequestContext() {
	reqCtx := a2asrv.RequestContext{
		Message: &a2a.Message{
			ID:         "",
			ContextID:  "",
			Extensions: nil,
			Metadata:   nil,
			Parts: []a2a.Part{
				a2a.TextPart{
					Text:     "",
					Metadata: nil,
				},
				a2a.FilePart{
					File:     nil,
					Metadata: nil,
				},
				a2a.DataPart{
					Data:     nil,
					Metadata: nil,
				},
			},
			ReferenceTasks: nil,
			Role:           "",
			TaskID:         "",
		},
		TaskID: "",
		StoredTask: &a2a.Task{
			ID: "",
			Artifacts: []*a2a.Artifact{
				&a2a.Artifact{
					ID:          "",
					Description: "",
					Extensions:  nil,
					Metadata:    nil,
					Name:        "",
					Parts:       nil,
				},
			},
			ContextID: "",
			History:   nil,
			Metadata:  nil,
			Status:    a2a.TaskStatus{},
		},
		RelatedTasks: []*a2a.Task{},
		ContextID:    "",
		Metadata:     nil,
	}
	println(reqCtx.Message.Parts[0].(*a2a.TextPart).Text) // 查询文本

	queue := eventqueue.NewInMemoryManager()
	q, ok := queue.Get(context.Background(), reqCtx.TaskID)
	if !ok {
		log.Printf("Failed to get task queue")
	}
	log.Printf("Got task queue: %s", q)
	if err := q.Write(context.Background(), a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: "agent"})); err != nil {
		log.Printf("Failed to write agent message")
	}

	card := a2a.AgentCard{
		AdditionalInterfaces: nil,
		Capabilities:         a2a.AgentCapabilities{},
		DefaultInputModes:    nil,
		DefaultOutputModes:   nil,
		Description:          "",
		DocumentationURL:     "",
		IconURL:              "",
		Name:                 "",
		PreferredTransport:   "",
		ProtocolVersion:      "",
		Provider:             nil,
		Security:             nil,
		SecuritySchemes:      nil,
		Signatures:           nil,
		Skills: []a2a.AgentSkill{
			{
				Description: "",
				Examples:    nil,
				ID:          "",
				InputModes:  nil,
				Name:        "",
				OutputModes: nil,
				Security:    nil,
				Tags:        nil,
			},
		},
		SupportsAuthenticatedExtendedCard: false,
		URL:                               "",
		Version:                           "",
	}
	log.Printf("Got card: %+v", card)

	messageResult := a2a.Message{
		ID:        "",
		ContextID: "",
		Parts: []a2a.Part{
			a2a.TextPart{
				Text: "",
			},
		},
		TaskID: "",
	}
	log.Printf("Got messageResult: %+v", messageResult)

	taskResult := a2a.Task{
		ID: "",
		Artifacts: []*a2a.Artifact{{
			ID:          "",
			Description: "",
			Name:        "",
			Parts:       []a2a.Part{a2a.TextPart{Text: ""}}}},
		ContextID: "",
		History: []*a2a.Message{{
			ID:             "",
			ContextID:      "",
			Parts:          []a2a.Part{a2a.TextPart{Text: ""}},
			ReferenceTasks: []a2a.TaskID{},
			Role:           "",
			TaskID:         "",
		}},
		Status: a2a.TaskStatus{
			Message: &a2a.Message{
				ID:             "",
				ContextID:      "",
				Parts:          []a2a.Part{a2a.TextPart{Text: ""}},
				ReferenceTasks: []a2a.TaskID{},
				Role:           "",
				TaskID:         "",
			},
			State:     "",
			Timestamp: nil,
		},
	}
	log.Printf("Got task result: %+v", taskResult)
}
