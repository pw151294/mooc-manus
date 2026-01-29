package tests

import (
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
