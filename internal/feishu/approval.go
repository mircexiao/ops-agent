package feishu

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sync"
)

type ApprovalResult struct {
	Allowed bool
	Reason  string
}

type ApprovalManager struct {
	mu           sync.Mutex
	pendingTasks map[string]chan ApprovalResult
}

var GlobalApprovalManager = &ApprovalManager{
	pendingTasks: make(map[string]chan ApprovalResult),
}

func (m *ApprovalManager) WaitForApproval(taskID string, toolName string, args string, reporter *FeishuReporter) (bool, string) {
	ch := make(chan ApprovalResult, 1)
	m.mu.Lock()
	m.pendingTasks[taskID] = ch
	m.mu.Unlock()
	// noticeMsg := fmt.Sprintf(`❗高危请求，Agent试图执行以下请求
	// 工具名称:%s
	// 参数:%s

	// 任务ID:%s
	// 请在此消息下方回复"approve %s"或"reject %s"决定是否执行
	// `, toolName, args, taskID, taskID, taskID)
	cardContent := buildApprovalCard(taskID, toolName, args)
	if reporter != nil {
		reporter.sendCardMessage(cardContent)
	} else {
		fmt.Printf("[需审批任务%s]%s", taskID, cardContent)
	}
	log.Printf("[Approval]任务%s需求审批已发送，协程已挂起等待", taskID)
	result := <-ch
	m.mu.Lock()
	delete(m.pendingTasks, taskID)
	m.mu.Unlock()
	return result.Allowed, result.Reason
}
func (m *ApprovalManager) ResolveApproval(taskID string, allowed bool, reason string) {
	m.mu.Lock()
	ch, exists := m.pendingTasks[taskID]
	m.mu.Unlock()
	if exists {
		log.Printf("[Approval]收到来自飞书的审批结果，任务ID:%s,是否允许:%v,原因:%s", taskID, allowed, reason)
		ch <- ApprovalResult{
			Allowed: allowed,
			Reason:  reason,
		}
	} else {
		log.Printf("[Approval]找不到对应的任务ID:%s,可能已处理或已过期", taskID)
	}

}

func buildApprovalCard(taskID string, toolName string, args string) string {
	card := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "⚠️ 高危操作审批请求",
			},
			"template": "red",
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": fmt.Sprintf("**工具名称:** %s\n**任务ID:** %s\n\n**参数:**\n```\n%s\n```", toolName, taskID, args),
				},
			},
			map[string]interface{}{
				"tag": "hr",
			},
			map[string]interface{}{
				"tag": "action",
				"actions": []interface{}{
					map[string]interface{}{
						"tag": "button",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "✅ 批准",
						},
						"type": "primary",
						"value": map[string]interface{}{
							"action": "approve",
							"taskID": taskID,
						},
					},
					map[string]interface{}{
						"tag": "button",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "❌ 拒绝",
						},
						"type": "danger",
						"value": map[string]interface{}{
							"action": "reject",
							"taskID": taskID,
						},
					},
				},
			},
		},
	}
	cardBytes, _ := json.Marshal(card)
	return string(cardBytes)
}
func IsDangerousCommand(toolName string, args string) bool {
	if toolName == "write_file" || toolName == "edit_file" {
		return true
	}
	if toolName == "bash" {
		dangerPatterns := []string{
			`rm\s+-r`,
			`sudo\s+`,
			`drop\s+`,
			`>.*\.go`,
			`nginx\s+-s`,
			`systemctl\s+`,
			`kill\s+`,
		}
		for _, pattern := range dangerPatterns {
			matched, _ := regexp.MatchString(pattern, args)
			if matched {
				return true
			}
		}
	}
	return false
}
