package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

type WriteFileTool struct {
	workDir string
}

func NewWriteFileTool(workDir string) *WriteFileTool {
	return &WriteFileTool{
		workDir: workDir,
	}
}

func (w *WriteFileTool) Name() string {
	return "write_file"
}

func (w *WriteFileTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        w.Name(),
		Description: "创建或覆盖写入一个文件，若文件不存在则创建文件，请提供相对于工作区的相对文件路径",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "文件路径",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "文件内容",
				},
			},
			"required": []string{"path", "content"},
		},
	}
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (w *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input writeFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败:%w", err)
	}
	fullPath := filepath.Join(w.workDir, input.Path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("创建目录失败:%w", err)
	}
	err := os.WriteFile(fullPath, []byte(input.Content), 0644)
	if err != nil {
		return "", fmt.Errorf("写入文件失败:%w", err)
	}
	return fmt.Sprintf("成功将内容写入到文件:%s", fullPath), nil
}
