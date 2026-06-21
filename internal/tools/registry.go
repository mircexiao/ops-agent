package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/mircexiao/go-tiny-claw/internal/observatibility"
	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

type Registry interface {
	Register(tool BaseTool)
	Use(mw MiddlewareFunc)
	GetAvailableTools() []schema.ToolDefinition
	Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult
}
type MiddlewareFunc func(ctx context.Context, call schema.ToolCall) (allow bool, rejectReason string)
type BaseTool interface {
	Name() string
	Definition() schema.ToolDefinition
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

type registryImpl struct {
	tools       map[string]BaseTool
	middlewares []MiddlewareFunc
}

func NewRegistry() Registry {
	return &registryImpl{
		tools:       make(map[string]BaseTool),
		middlewares: make([]MiddlewareFunc, 0),
	}
}
func (r *registryImpl) Register(tool BaseTool) {
	name := tool.Name()
	if _, ok := r.tools[name]; ok {
		log.Printf("WARNING 工具%s已注册，将被覆盖\n", name)
	}
	r.tools[name] = tool
	log.Printf("工具%s已注册\n", name)
}
func (r *registryImpl) Use(mw MiddlewareFunc) {
	r.middlewares = append(r.middlewares, mw)
}

func (r *registryImpl) GetAvailableTools() []schema.ToolDefinition {
	defs := make([]schema.ToolDefinition, 0)
	for _, tool := range r.tools {
		defs = append(defs, tool.Definition())
	}
	return defs
}

func (r *registryImpl) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	tool, ok := r.tools[call.Name]
	ctx, span := observatibility.StartSpan(ctx, "Tool.Execute")
	span.AddAttribute("tool_name", call.Name)
	span.AddAttribute("tool_args", string(call.Arguments))
	defer span.EndSpan()
	if !ok {
		errMsg := fmt.Sprintf("工具%s不存在\n", call.Name)
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     errMsg,
			IsError:    true,
		}
	}
	for _, mw := range r.middlewares {
		allow, rejectReason := mw(ctx, call)
		if !allow {
			return schema.ToolResult{
				ToolCallID: call.ID,
				Output:     rejectReason,
				IsError:    true,
			}
		}
	}
	output, err := tool.Execute(ctx, call.Arguments)
	span.AddAttribute("tool_output", truncate(output, 100))
	if err != nil {
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     err.Error(),
			IsError:    true,
		}
	}
	return schema.ToolResult{
		ToolCallID: call.ID,
		Output:     output,
		IsError:    false,
	}
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
