package context

import (
	"fmt"
	"strings"
)

type RecoveryManager struct{}

func NewRecoveryManager() *RecoveryManager {
	return &RecoveryManager{}
}

func (r *RecoveryManager) AnalyzeAndInject(toolName string, rawError string) string {
	var hint string
	lowerError := strings.ToLower(rawError)
	switch toolName {
	case "edit_file":
		if strings.Contains(rawError, "在文件中未找到 old_text") || strings.Contains(rawError, "找到匹配文本") {
			hint = "你提供的 old_text 与文件当前内容不一致，或者缺少必要的缩进。请先使用 `read_file` 工具重新读取该文件，获取最新、准确的内容后，再重新发起编辑。"
		} else if strings.Contains(rawError, "匹配到了多处") || strings.Contains(rawError, "提供更多上下文") {
			hint = "你的 old_text 不够具体，命中了多个相同代码块。请在 old_text 中增加上下相邻的几行代码，以确保替换的唯一性。"
		}
	case "read_file", "write_file":
		if strings.Contains(lowerError, "no such file or directory") {
			hint = "路径似乎不正确。请不要凭空猜测，先使用 `bash` 执行 `ls -la` 或 `find . -name` 命令查找正确的目录结构和文件名。"
		} else if strings.Contains(lowerError, "permission denied") {
			hint = "你没有权限操作该文件。请检查工作区限制，或者思考是否需要修改其他文件。"
		}
	case "bash":
		if strings.Contains(lowerError, "command not found") {
			hint = "系统中未安装该命令。请先思考：是否有替代命令？或者你需要先编写脚本进行安装？"
		} else if strings.Contains(rawError, "超时") || strings.Contains(rawError, "DeadlineExceeded") {
			// 匹配我们手写的 30s context.WithTimeout 报错
			hint = "该命令执行被超时强杀。如果它是一个常驻服务（如 server 或 watch），请将其转入后台执行（例如使用 `nohup ... &`），不要阻塞主线程。"
		} else if strings.Contains(lowerError, "syntax error") {
			hint = "Bash 语法错误。请检查引号转义或特殊字符，确保命令在终端中可直接运行。"
		}
	}
	if hint == "" {
		return rawError
	}
	return fmt.Sprintf("%s\n\n[系统救援指南]%s", rawError, hint)
}
