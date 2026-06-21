package engine

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

type ReminderInjector struct {
	consecutiveFailures map[string]int
}

func NewReminderInjector() *ReminderInjector {
	return &ReminderInjector{
		consecutiveFailures: make(map[string]int),
	}
}

func generateFingerPrint(toolName string, args []byte) string {
	hasher := md5.New()
	hasher.Write([]byte(toolName))
	hasher.Write(args)
	return hex.EncodeToString(hasher.Sum(nil))
}

func (r *ReminderInjector) CheckAndInject(lastToolCall schema.ToolCall, lastToolResult schema.ToolResult) *schema.Message {
	fingerprint := generateFingerPrint(lastToolCall.Name, lastToolCall.Arguments)
	if !lastToolResult.IsError {
		r.consecutiveFailures = make(map[string]int)
		return nil
	}
	r.consecutiveFailures[fingerprint]++
	failCount := r.consecutiveFailures[fingerprint]
	log.Printf("[Engine]检测到工具%s连续调用失败,该特征参数已失败%d次", lastToolCall.Name, failCount)
	if failCount >= 3 {
		log.Printf("[Engine]工具多次调用失败,强制修正指令")
		nudgeMsg := fmt.Sprintf(`
		[严正警告]
		你似乎陷入了死循环。
		你刚刚连续 %d 次使用相同的参数调用了 '%s' 工具，并且都失败了。
		请立即停止这种无效的重试！你的注意力被当前的报错过度吸引了。你需要：
		1. 停止猜测参数。跳出当前的局部思维。
		2. 彻底改变你的策略。
		3. 如果你确实无法通过系统工具解决当前问题，请直接结束任务并向用户说明你需要什么人工帮助，而不是继续盲目消耗 API 资源尝试。
		`, failCount, lastToolCall.Name)
		return &schema.Message{
			Role:    schema.RoleUser,
			Content: nudgeMsg,
		}
	}
	return nil
}
