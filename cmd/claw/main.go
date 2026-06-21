package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	ctxpkg "github.com/mircexiao/go-tiny-claw/internal/context"
	"github.com/mircexiao/go-tiny-claw/internal/observatibility"
	"github.com/mircexiao/go-tiny-claw/internal/schema"

	"github.com/joho/godotenv"
	"github.com/mircexiao/go-tiny-claw/internal/engine"
	"github.com/mircexiao/go-tiny-claw/internal/provider"
	"github.com/mircexiao/go-tiny-claw/internal/tools"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("未找到.env文件")
	}

	promptPtr := flag.String("prompt", "", "要交给Agent执行的任务描述")
	workDirPtr := flag.String("dir", ".", "Agent工作目录,默认为当前目录")
	sessionPtr := flag.String("session", "cli_default_session", "指定会话ID,支持断点续传")

	flag.Parse()
	if *promptPtr == "" {
		fmt.Printf("用法: go run /cmd/claw/main.go -prompt \"要交给Agent执行的任务描述\"")
		os.Exit(1)
	}
	workDir, err := filepath.Abs(*workDirPtr)
	if err != nil {
		log.Fatalf("解析工作目录失败:%v", err)
	}
	fmt.Println("=======================================")
	fmt.Printf("🚗启动go-tiny-claw CLI引擎...\n")
	fmt.Printf("📁锁定工作区:%s\n", workDir)
	fmt.Println("=======================================")
	var realProvider provider.LLMProvider
	modelName := "deepseek-chat"
	realProvider = provider.NewOpenAIProvider(modelName)
	session := ctxpkg.GlobalSessionManager.GetOrCreate(*sessionPtr, workDir)
	trackerProvider := observatibility.NewCostTracker(realProvider, modelName, session)
	readFileTool := tools.NewReadFileTool(workDir)
	writeFileTool := tools.NewWriteFileTool(workDir)
	bashTool := tools.NewBashTool(workDir)
	editFileTool := tools.NewEditFileTool(workDir)
	registry := tools.NewRegistry()
	registry.Register(readFileTool)
	registry.Register(writeFileTool)
	registry.Register(bashTool)
	registry.Register(editFileTool)
	eng := engine.NewAgentEngine(trackerProvider, registry, true, true)
	ctx, rootSpan := observatibility.StartSpan(context.Background(), "CLI.TaskRun")
	rootSpan.AddAttribute("prompt", *promptPtr)
	defer func() {
		rootSpan.EndSpan()
		observatibility.ExportTraceToFile(rootSpan, workDir, *sessionPtr)
	}()
	reporter := engine.NewTerminalReporter()
	session.Append(
		schema.Message{
			Role:    schema.RoleUser,
			Content: *promptPtr,
		},
	)
	errAgent := eng.Run(ctx, session, reporter)
	if errAgent != nil {
		log.Fatalf("Agent启动失败:%v", errAgent)
	}
	fmt.Println("\n=================================================")
	fmt.Printf("🎊任务圆满结束，总计耗时:%v\n", time.Since(rootSpan.StartTime))
	fmt.Printf("💰任务累计费用%.6f,其中Input Tokens:%d,Output Tokens:%d\n", session.TotalCostCNY, session.TotalPromptTokens, session.TotalCompletionTokens)
	fmt.Println("=====================================================")
}
