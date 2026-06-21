package engine

import (
	"context"
	"fmt"
	"log"
	"sync"

	ctxpkg "github.com/mircexiao/go-tiny-claw/internal/context"
	"github.com/mircexiao/go-tiny-claw/internal/observatibility"
	"github.com/mircexiao/go-tiny-claw/internal/provider"
	"github.com/mircexiao/go-tiny-claw/internal/schema"
	"github.com/mircexiao/go-tiny-claw/internal/tools"
)

type AgentEngine struct {
	provider        provider.LLMProvider
	registry        tools.Registry
	WorkDir         string
	EnableThinking  bool
	PlanMode        bool
	composer        *ctxpkg.PromptComposer
	compactor       *ctxpkg.Compactor
	recoveryManager *ctxpkg.RecoveryManager
	injector        *ReminderInjector
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, enableThinking bool, planMode bool) *AgentEngine {
	return &AgentEngine{
		provider:        p,
		registry:        r,
		EnableThinking:  enableThinking,
		PlanMode:        planMode,
		compactor:       ctxpkg.NewCompactor(3000, 20),
		recoveryManager: ctxpkg.NewRecoveryManager(),
		injector:        NewReminderInjector(),
	}
}
func (e *AgentEngine) Run(ctx context.Context, session *ctxpkg.Session, report Reporter) error {
	log.Printf("Engine 已启动，锁定工作区%s\n", e.WorkDir)
	ctx, rootSpan := observatibility.StartSpan(ctx, "Agent.Run")
	defer func() {
		rootSpan.EndSpan()
		observatibility.ExportTraceToFile(rootSpan, e.WorkDir, session.ID)
		log.Printf("🧮[Tracing]本次任务执行回放链路已保存到工作区.claw/traces目录下")
	}()
	composer := ctxpkg.NewPromptComposer(session.WorkDir, e.PlanMode)
	systemMessage := composer.Build()
	turnCount := 0
	for {
		turnCount++
		turnCtx, turnSpan := observatibility.StartSpan(ctx, fmt.Sprintf("Turn %d", turnCount))
		defer turnSpan.EndSpan()
		log.Printf("===============================[Turn %d]=========================开始", turnCount)
		avaliableTools := e.registry.GetAvailableTools()
		log.Printf("[Engine]可用工具数量: %d", len(avaliableTools))
		workingMemory := session.GetWorkingMemory(20)
		var contextHistory []schema.Message
		contextHistory = append(contextHistory, systemMessage)
		contextHistory = append(contextHistory, workingMemory...)
		compactedContext := e.compactor.Compact(contextHistory)
		turnSpan.AddAttribute("context_message_count", len(compactedContext))
		log.Printf("[Engine]正在执行，请稍等(Resoning)...")
		if e.EnableThinking {
			_, thinkSpan := observatibility.StartSpan(turnCtx, "LLM.Thinking")
			defer thinkSpan.EndSpan()
			log.Printf("[Engine]正在慢思考(Thinking)...")
			thinkRes, err := e.provider.Generate(ctx, compactedContext, nil)
			if err != nil {
				log.Printf("[Engine]思考失败")
			}
			if thinkRes.Content != "" {
				fmt.Printf("[Engine]思考结果:%s\n", thinkRes.Content)
				// session.Append(*thinkRes)
				compactedContext = append(compactedContext, *thinkRes)

			}
		}
		_, actSpan := observatibility.StartSpan(turnCtx, "LLM.Action")
		response, err := e.provider.Generate(ctx, compactedContext, avaliableTools)
		actSpan.EndSpan()
		if err != nil {
			log.Printf("[Engine]执行失败:%v", err)
			return err
		}
		session.Append(*response)
		// compactedContext = append(compactedContext, *response)
		if response.Content != "" && report != nil {
			report.OnMessage(ctx, response.Content)
		}
		if len(response.ToolCalls) == 0 {
			actSpan.EndSpan()
			log.Print("[Engine]任务已完成，退出循环...\n")
			break
		}
		observationMsgs := make([]schema.Message, len(response.ToolCalls))
		var lastToolCall schema.ToolCall
		var lastToolResult schema.ToolResult
		var wg sync.WaitGroup
		for i, toolCall := range response.ToolCalls {
			wg.Add(1)
			go func(idx int, call schema.ToolCall) {
				defer wg.Done()
				if report != nil {
					report.OnToolCall(ctx, toolCall.Name, string(toolCall.Arguments))
				}
				result := e.registry.Execute(turnCtx, toolCall)
				finalOutput := result.Output
				// fmt.Printf("工具执行错误:%s,%v\n", finalOutput, result.IsError)
				if result.IsError || err != nil {
					finalOutput = e.recoveryManager.AnalyzeAndInject(toolCall.Name, result.Output)
					// log.Printf("-> ✖️工具执行报错：%s\n", finalOutput)
				} else {
					// log.Printf("->✔️工具执行成功，（返回%d字节）", len(result.Output))
				}
				if report != nil {
					displayOutput := finalOutput
					if len(displayOutput) > 200 {
						displayOutput = displayOutput[:200] + "...已截断"
					}
					report.OnToolResult(ctx, toolCall.Name, finalOutput, result.IsError)
				}

				msg := schema.Message{
					Role:       schema.RoleUser,
					Content:    finalOutput,
					ToolCallID: toolCall.ID,
				}
				observationMsgs[i] = msg
				if idx == 0 {
					lastToolCall = call
					lastToolResult = result
				}
			}(i, toolCall)

		}
		wg.Wait()
		session.Append(observationMsgs...)
		turnSpan.EndSpan()
		reminderMsg := e.injector.CheckAndInject(lastToolCall, lastToolResult)
		if reminderMsg != nil {
			session.Append(*reminderMsg)
		}
	}
	return nil
}

func (e *AgentEngine) RunSub(ctx context.Context, taskPrompt string, readOnlyRegistry tools.Registry, reporter interface{}) (string, error) {
	contextHistory := []schema.Message{
		{
			Role: schema.RoleSystem,
			Content: `
			你是一个专门负责深度探索的探路者 (Explorer Subagent)。
			你的任务是根据主架构师的指令，在当前工作区内仔细阅读代码、查阅日志，搜集足够的信息。
			【核心纪律】
			1. 你必须、且只能依靠内置工具（如 bash 的 find/grep，或 read_file）去寻找答案。绝对不允许凭空捏造或猜测！
			2. 如果你没有找到确切的答案，你必须继续使用工具深入搜索。
			3. 当且仅当你找到了确切的线索后，停止调用工具，直接输出一段纯文本作为你的终极汇报。主架构师会根据你的汇报来做下一步决策。`,
		},
	}
	const maxSubTurns = 10
	turnCount := 0
	for {
		turnCount++
		if turnCount > maxSubTurns {
			return "", fmt.Errorf("子智能体探索过于深入，已超过最大轮次，请主Agent给与更明确指令")
		}
		availableTools := readOnlyRegistry.GetAvailableTools()
		compactedContext := e.compactor.Compact(contextHistory)
		response, err := e.provider.Generate(ctx, compactedContext, availableTools)
		if err != nil {
			return "", fmt.Errorf("子智能体执行失败:%v", err)
		}
		contextHistory = append(contextHistory, *response)
		if len(response.ToolCalls) == 0 {
			return response.Content, nil
		}
		observationMsgs := make([]schema.Message, len(response.ToolCalls))
		var wg sync.WaitGroup
		for i, toolCall := range response.ToolCalls {
			wg.Add(1)
			go func(idx int, call schema.ToolCall) {
				defer wg.Done()
				var r Reporter
				if reporter != nil {
					r = reporter.(Reporter)
					r.OnToolCall(ctx, fmt.Sprintf("[Subagent]执行工具%s\n", call.Name), string(call.Arguments))
				}
				result := readOnlyRegistry.Execute(ctx, call)
				finalOutput := result.Output

				if result.IsError {
					finalOutput = e.recoveryManager.AnalyzeAndInject(call.Name, result.Output)
					r.OnToolResult(ctx, call.Name, finalOutput, result.IsError)
				}
				if reporter != nil {
					displayOutput := finalOutput
					if len(displayOutput) > 200 {
						displayOutput = displayOutput[:200] + "...已截断"
					}
					r.OnToolResult(ctx, call.Name, fmt.Sprintf("[Subagent]%s\n", displayOutput), result.IsError)
				}
				observationMsgs[idx] = schema.Message{
					Role:       schema.RoleUser,
					Content:    finalOutput,
					ToolCallID: toolCall.ID,
				}
			}(i, toolCall)
		}
		wg.Wait()
		contextHistory = append(contextHistory, observationMsgs...)
	}
}
