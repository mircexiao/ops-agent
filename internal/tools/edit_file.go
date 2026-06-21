package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

type EditFileTool struct {
	workDir string
}

func NewEditFileTool(workDir string) *EditFileTool {
	return &EditFileTool{
		workDir: workDir,
	}
}

func (e *EditFileTool) Name() string {
	return "edit_file"
}

func (e *EditFileTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        e.Name(),
		Description: "对文件进行局部字符匹配替换，这比重写整个文件更安全、更快速，请提供足够的old_text上下文，已保证唯一性",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "要编辑的文件路径",
				},
				"old_text": map[string]interface{}{
					"type":        "string",
					"description": "文本中原有的旧文本，请提供足够的上下文(建议上下多包含几行)",
				},
				"new_text": map[string]interface{}{
					"type":        "string",
					"description": "要替换成的新文本",
				},
			},
			"required": []string{"path", "old_text", "new_text"},
		},
	}
}

type editFileArgs struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

func fuzzyReplace(originContent, oldText, newText string) (string, error) {
	count := strings.Count(originContent, oldText)
	if count == 0 {
		return "", fmt.Errorf("未找到匹配文本")
	}
	if count == 1 {
		return strings.Replace(originContent, oldText, newText, 1), nil
	}
	if count > 1 {
		return "", fmt.Errorf("匹配到了多个文本，请提供更多上下文")
	}
	normalizedContent := strings.ReplaceAll(originContent, "\r\n", "\n")
	normalizedOld := strings.ReplaceAll(oldText, "\r\n", "\n")
	count = strings.Count(normalizedContent, normalizedOld)
	if count == 1 {
		return strings.Replace(normalizedContent, normalizedOld, newText, 1), nil
	}
	trimmedOld := strings.TrimSpace(normalizedOld)
	if trimmedOld != "" {
		count = strings.Count(normalizedContent, trimmedOld)
		if count == 1 {
			return strings.Replace(normalizedContent, trimmedOld, newText, 1), nil
		}
	}
	return lineByLineReplace(originContent, oldText, newText)
}

func lineByLineReplace(originContent, oldText, newText string) (string, error) {
	contentLines := strings.Split(originContent, "\n")
	oldLines := strings.Split(strings.TrimSpace(oldText), "\n")
	if len(oldLines) == 0 || len(contentLines) < len(oldLines) {
		return "", fmt.Errorf("找不到该代码片段")
	}
	for i := range oldLines {
		oldLines[i] = strings.TrimSpace(oldLines[i])
	}
	matchCount := 0
	matchStartIndex := 0
	matchEndIndex := 0
	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		isMatch := true
		for j := 0; j < len(oldLines); j++ {
			if strings.TrimSpace(contentLines[i+j]) != oldLines[j] {
				isMatch = false
				break
			}
		}
		if isMatch {
			matchCount++
			matchStartIndex = i
			matchEndIndex = i + len(oldLines)
		}
	}
	if matchCount == 0 {
		return "", fmt.Errorf("未找到匹配文本")
	}
	if matchCount > 1 {
		return "", fmt.Errorf("匹配到了多个文本，请提供更多上下文")
	}
	var newContentLines []string
	newContentLines = append(newContentLines, contentLines[:matchStartIndex]...)
	newContentLines = append(newContentLines, strings.Split(newText, "\n")...)
	newContentLines = append(newContentLines, contentLines[matchEndIndex:]...)
	return strings.Join(newContentLines, "\n"), nil
}

func (e *EditFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input editFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败:%w", err)
	}
	fullPath := filepath.Join(e.workDir, input.Path)
	originContent, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败:%w", err)
	}
	newContent, err := fuzzyReplace(string(originContent), input.OldText, input.NewText)
	if err != nil {
		return "", fmt.Errorf("替换文本失败:%w", err)
	}
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败:%w", err)
	}
	return fmt.Sprintf("文件 %s 已成功编辑", input.Path), nil
}
