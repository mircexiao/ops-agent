package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

type ClaudeProvider struct {
	client anthropic.Client
	model  string
}

func NewClaudeProvider(model string) *ClaudeProvider {
	api_key := "sk-55ebd276a52a408fae7a1d2af94f24ed"
	if api_key == "" {
		panic("CLAUDE_API_KEY is not set")
	}
	base_url := "https://api.deepseek.com/anthropic"
	return &ClaudeProvider{
		client: anthropic.NewClient(option.WithAPIKey(api_key), option.WithBaseURL(base_url)),
		model:  model,
	}
}

func (p *ClaudeProvider) Generate(ctx context.Context, msgs []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	var anthropicMsgs []anthropic.MessageParam
	var systemPrompt string
	for _, msg := range msgs {
		switch msg.Role {
		case schema.RoleSystem:
			systemPrompt = msg.Content
		case schema.RoleUser:
			if msg.ToolCallID != "" {
				anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false)))
			} else {
				anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
			}
		case schema.RoleAssistant:
			var blocks []anthropic.ContentBlockParamUnion
			if msg.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				var inputMap map[string]interface{}
				_ = json.Unmarshal(tc.Arguments, &inputMap)
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    tc.ID,
						Name:  tc.Name,
						Input: inputMap,
					},
				})
			}
			if len(blocks) > 0 {
				anthropicMsgs = append(anthropicMsgs, anthropic.NewAssistantMessage(blocks...))
			}

		}
	}
	//把消息的工具调用翻译成anthropic格式
	var anthropicTools []anthropic.ToolUnionParam
	for _, toolDef := range availableTools {
		var properties map[string]interface{}
		var required []string
		if m, ok := toolDef.InputSchema.(map[string]interface{}); ok {
			if p, ok := m["properties"].(map[string]interface{}); ok {
				properties = p
			}
			if r, ok := m["required"].([]string); ok {
				required = r
			}
		}
		tp := anthropic.ToolParam{
			Name:        toolDef.Name,
			Description: anthropic.String(toolDef.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: properties,
				Required:   required,
			},
		}
		anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{OfTool: &tp})
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		Messages:  anthropicMsgs,
		MaxTokens: 4096,
	}
	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemPrompt},
		}
	}
	if len(anthropicTools) > 0 {
		params.Tools = anthropicTools
	}
	res, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("Model 请求失败，%w", err)
	}
	returnMsg := &schema.Message{
		Role: schema.RoleAssistant,
	}
	for _, block := range res.Content {
		switch block.Type {
		case "text":
			returnMsg.Content += block.Text
		case "tool_use":
			argsBytes, _ := json.Marshal(block.Input)
			returnMsg.ToolCalls = append(returnMsg.ToolCalls, schema.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: argsBytes,
			})
		}
	}
	return returnMsg, nil
}

func (p *ClaudeProvider) GenerateStream(ctx context.Context, msgs []schema.Message, availableTools []schema.ToolDefinition) (<-chan schema.EventType, error) {
	return nil, nil
}
