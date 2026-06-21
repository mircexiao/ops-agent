package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	ctxpkg "github.com/mircexiao/go-tiny-claw/internal/context"
	"github.com/mircexiao/go-tiny-claw/internal/engine"
	"github.com/mircexiao/go-tiny-claw/internal/feishu"
	"github.com/mircexiao/go-tiny-claw/internal/observatibility"
	"github.com/mircexiao/go-tiny-claw/internal/provider"
	"github.com/mircexiao/go-tiny-claw/internal/schema"
	"github.com/mircexiao/go-tiny-claw/internal/tools"
)

func main() {
	fmt.Printf("🚗正在启动Agent运维平台服务端……\n")
	if err := godotenv.Load(); err != nil {
		log.Fatal("未找到.env文件")
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		fmt.Println("❗当前未配置模型")
	}
	workDir, _ := os.Getwd()
	workDir += "/workSpace"
	if err := os.MkdirAll(workDir, 0755); err != nil {
		log.Fatalf("❗创建工作区失败,%v", err)
	}
	modelName := "deepseek-chat"
	llmProvider := provider.NewOpenAIProvider(modelName)
	registry := tools.NewRegistry()
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))
	registry.Use(
		func(ctx context.Context, call schema.ToolCall) (bool, string) {
			argsStr := string(call.Arguments)
			if feishu.IsDangerousCommand(call.Name, argsStr) {
				taskID := call.ID
				log.Printf("[Middleware]拦截到危险操作%s，需要飞书审批\n", call.Name)
				currentReporter, _ := feishu.ReporterFromContext(ctx).(*feishu.FeishuReporter)
				allowed, reason := feishu.GlobalApprovalManager.WaitForApproval(taskID, call.Name, argsStr, currentReporter)
				if !allowed {
					return false, reason
				}
				return true, ""
			}
			return true, ""
		},
	)
	log.Print("MiddleWare已挂载")
	engineFactory := func(session *ctxpkg.Session) *engine.AgentEngine {
		trackedProvider := observatibility.NewCostTracker(llmProvider, modelName, session)
		return engine.NewAgentEngine(trackedProvider, registry, false, false)
	}
	bot := feishu.NewFeishuBotFactory(engineFactory, workDir)
	err := bot.StartLongConnection()
	if err != nil {
		fmt.Print("飞书连接失败")
	}
}
