package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

type ReadFileTool struct {
	workDir string
}

func NewReadFileTool(workDir string) *ReadFileTool {
	return &ReadFileTool{
		workDir: workDir,
	}
}

func (r *ReadFileTool) Name() string {
	return "read_file"
}
func (r *ReadFileTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        r.Name(),
		Description: "读取指定文件内容，请提供文件路径",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "文件路径",
				},
			},
			"required": []string{"path"},
		},
	}
}

type readFileArgs struct {
	Path string `json:"path"`
}

func (r *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input readFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败:%w", err)
	}
	fullPath := filepath.Join(r.workDir, input.Path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败%w:", err)
	}
	const maxLength = 1000000000000
	if len(content) > maxLength {
		fmt.Printf("文件内容超过最大长度%d，已截断", maxLength)
		content = content[:maxLength]
	}
	return string(content), nil
}
