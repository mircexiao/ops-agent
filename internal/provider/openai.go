package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mircexiao/go-tiny-claw/internal/schema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

type OpenAIProvider struct {
	client openai.Client
	model  string
}

func NewOpenAIProvider(model string) *OpenAIProvider {
	api_key := "sk-55ebd276a52a408fae7a1d2af94f24ed"
	if api_key == "" {
		panic("OPENAI_API_KEY is not set")
	}
	base_url := "https://api.deepseek.com/v1"
	return &OpenAIProvider{
		client: openai.NewClient(option.WithAPIKey(api_key), option.WithBaseURL(base_url)),
		model:  model,
	}
}
func (p *OpenAIProvider) converMessages(msgs []schema.Message) []openai.ChatCompletionMessageParamUnion {
	var openaiMsgs []openai.ChatCompletionMessageParamUnion
	for _, msg := range msgs {
		switch msg.Role {
		case schema.RoleSystem:
			openaiMsgs = append(openaiMsgs, openai.SystemMessage(msg.Content))
		case schema.RoleUser:
			if msg.ToolCallID != "" {
				openaiMsgs = append(openaiMsgs, openai.ToolMessage(msg.Content, msg.ToolCallID))
			} else {
				openaiMsgs = append(openaiMsgs, openai.UserMessage(msg.Content))
			}
		case schema.RoleAssistant:
			astParam := openai.ChatCompletionAssistantMessageParam{}
			if msg.Content != "" {
				astParam.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				}
			}
			if len(msg.ToolCalls) > 0 {
				var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID:   tc.ID,
							Type: "function",
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: string(tc.Arguments),
							},
						},
					})
				}
				astParam.ToolCalls = toolCalls
			}
			openaiMsgs = append(openaiMsgs, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &astParam,
			})
		}
	}
	return openaiMsgs
}

func (p *OpenAIProvider) convertTools(tools []schema.ToolDefinition) []openai.ChatCompletionToolUnionParam {
	var openaiTools []openai.ChatCompletionToolUnionParam
	for _, toolDef := range tools {
		var params shared.FunctionParameters
		if m, ok := toolDef.InputSchema.(map[string]interface{}); ok {
			params = shared.FunctionParameters(m)
		} else {
			b, _ := json.Marshal(toolDef.InputSchema)
			_ = json.Unmarshal(b, &params)
		}
		openaiTools = append(openaiTools, openai.ChatCompletionFunctionTool(
			shared.FunctionDefinitionParam{
				Name:        toolDef.Name,
				Description: openai.String(toolDef.Description),
				Parameters:  params,
			},
		))
	}
	return openaiTools
}
func (p *OpenAIProvider) Generate(ctx context.Context, msgs []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	var openaiMsgs []openai.ChatCompletionMessageParamUnion = p.converMessages(msgs)
	var openaiTools []openai.ChatCompletionToolUnionParam = p.convertTools(availableTools)
	params := openai.ChatCompletionNewParams{
		Model:    p.model,
		Messages: openaiMsgs,
	}
	if len(openaiTools) > 0 {
		params.Tools = openaiTools
	}
	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("Model 请求失败：%w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("API返回了空的Choices")
	}
	choice := resp.Choices[0].Message
	resultMsg := &schema.Message{
		Role:    schema.RoleAssistant,
		Content: choice.Content,
	}
	if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		resultMsg.Usage = &schema.Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
		}
	}
	for _, tc := range choice.ToolCalls {
		if tc.Type == "function" {
			resultMsg.ToolCalls = append(resultMsg.ToolCalls, schema.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: []byte(tc.Function.Arguments),
			})
		}
	}
	return resultMsg, nil
}

func (p *OpenAIProvider) GenerateStream(ctx context.Context, msgs []schema.Message, availableTools []schema.ToolDefinition) (<-chan schema.StreamEvent, error) {
	var openaiMsgs []openai.ChatCompletionMessageParamUnion = p.converMessages(msgs)
	var openaiTools []openai.ChatCompletionToolUnionParam = p.convertTools(availableTools)
	params := openai.ChatCompletionNewParams{
		Model:    p.model,
		Messages: openaiMsgs,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
	}
	if len(openaiTools) > 0 {
		params.Tools = openaiTools
	}
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	ch := make(chan schema.StreamEvent, 10)
	go func() {
		defer close(ch)
		toolCallArgs := make(map[int]string)
		toolCallIDs := make(map[int]string)
		var usage *schema.Usage
		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) == 0 {
				continue
			}
			delta := chunk.Choices[0].Delta
			if delta.Content != "" {
				ch <- schema.StreamEvent{
					Type: schema.EventTextDelta,
					Text: delta.Content,
				}
			}
			for _, tc := range delta.ToolCalls {
				idx := int(tc.Index)
				if tc.ID != "" {
					toolCallIDs[idx] = tc.ID
					ch <- schema.StreamEvent{
						Type:       schema.EventToolCallStart,
						ToolCallID: tc.ID,
						ToolName:   tc.Function.Name,
					}
				}
				if tc.Function.Arguments != "" {
					toolCallArgs[idx] += tc.Function.Arguments
					callID := tc.ID
					if callID == "" {
						callID = toolCallIDs[idx]
					}
					ch <- schema.StreamEvent{
						Type:          schema.EventToolCallDelta,
						ToolCallID:    callID,
						ToolArgsDelta: tc.Function.Arguments,
					}
				}
			}
			if chunk.Usage.TotalTokens > 0 {
				usage = &schema.Usage{
					PromptTokens:     int(chunk.Usage.PromptTokens),
					CompletionTokens: int(chunk.Usage.CompletionTokens),
				}
			}
		}
		if usage != nil {
			ch <- schema.StreamEvent{
				Type:  schema.EventUsage,
				Usage: usage,
				Error: nil,
			}
		}
		if err := stream.Err(); err != nil {
			ch <- schema.StreamEvent{
				Type:  schema.EventError,
				Error: err,
			}
			return
		}
		ch <- schema.StreamEvent{
			Type: schema.EventDone,
		}
	}()
	return ch, nil
}
