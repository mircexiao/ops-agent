package engine

import (
	"context"
	"fmt"
)

type TerminalReporter struct{}

func NewTerminalReporter() *TerminalReporter {
	return &TerminalReporter{}
}

func (r *TerminalReporter) OnThinking(ctx context.Context) {
	fmt.Printf("😀 [Engine] 正在慢思考(Thinking)...")
}
func (r *TerminalReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	fmt.Printf("🔨 正在执行工具%s,参数%s\n", toolName, args)
}

func (r *TerminalReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	if isError {
		fmt.Printf("❌工具%s执行失败:%s\n", toolName, result)
	} else {
		fmt.Printf("✅工具%s执行成功\n", toolName)
	}
}

func (r *TerminalReporter) OnMessage(ctx context.Context, content string) {
	fmt.Println(content)
}
