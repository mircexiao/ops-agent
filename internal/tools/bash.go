package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

type BashTool struct {
	workDir string
}

func NewBashTool(workDir string) *BashTool {
	return &BashTool{
		workDir: workDir,
	}
}

func (b *BashTool) Name() string {
	return "bash"
}

func (b *BashTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        b.Name(),
		Description: "在当前工作区执行bash命令，执行链式命令，返回标准输出",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "bash执行的命令",
				},
			},
			"required": []string{"command"},
		},
	}
}

type bashArgs struct {
	Command string `json:"command"`
}

func (b *BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input bashArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败:%w", err)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	// 根据操作系统选择不同的 shell
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(timeoutCtx, "cmd", "/c", input.Command)
	} else {
		cmd = exec.CommandContext(timeoutCtx, "bash", "-c", input.Command)
	}
	cmd.Dir = b.workDir
	output, err := cmd.CombinedOutput()
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("命令执行超时(30秒),请重试")
	}
	if err != nil {
		// 即使命令失败，也返回输出内容（可能包含有用的错误信息）
		if len(output) > 0 {
			return fmt.Sprintf("命令执行失败（退出码非零）:\n%s", string(output)), nil
		}
		return "", fmt.Errorf("命令执行失败:%w", err)
	}
	if string(output) == "" {
		return "命令执行成功，终端无输出", nil
	}
	const maxLength = 8000
	if len(output) > maxLength {
		return fmt.Sprintf("%s\n\n[输出过长，已截断前%d字节]", string(output[:maxLength]), maxLength), nil
	}
	return string(output), nil
}
