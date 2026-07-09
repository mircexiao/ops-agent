package eval

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	ctxpkg "github.com/mircexiao/go-tiny-claw/internal/context"
	"github.com/mircexiao/go-tiny-claw/internal/engine"
	"github.com/mircexiao/go-tiny-claw/internal/observatibility"
	"github.com/mircexiao/go-tiny-claw/internal/provider"
	"github.com/mircexiao/go-tiny-claw/internal/schema"
	"github.com/mircexiao/go-tiny-claw/internal/tools"
)

type TestCase struct {
	ID             string
	Name           string
	SetupScript    string
	ValidateScript string
	TaskPrompt     string
	MaxTurns       int
}

type TestResult struct {
	TestCaseID   string
	Passed       bool
	TotalCostCNY float64
	Duration     float64
	ErrMsg       string
}

type BenchmarkRunner struct {
	modelName string
}

func NewBenchmarkRunner(modelName string) *BenchmarkRunner {
	return &BenchmarkRunner{
		modelName: modelName,
	}
}

func (b *BenchmarkRunner) RunSuit(ctx context.Context, testcases []TestCase) {
	log.Printf("============================================")
	log.Printf("🐧启动Harness Benchmark评估|模型:%s\n", b.modelName)
	log.Printf("============================================")
	var results []TestResult
	var passedCount int
	var totalCost float64
	for _, tc := range testcases {
		log.Printf("正在执行用例[%s]:%s\n", tc.ID, tc.Name)
		res := b.runSingleTest(ctx, tc)
		results = append(results, res)
		if res.Passed {
			log.Printf("✅用例[%s]:%s执行通过,耗时:%fms,成本:%f元\n", tc.ID, tc.Name, res.Duration, res.TotalCostCNY)
			passedCount++
		} else {
			log.Printf("❌用例[%s]:%s执行失败,错误信息:%s\n", tc.ID, tc.Name, res.ErrMsg)
		}
		totalCost += res.TotalCostCNY
	}
	log.Printf("=================跑分终极报告=================\n")
	log.Printf("总用例数:%d|通过用例数:%d|成功率:%f%%\n", len(testcases), passedCount, float64(passedCount)/float64(len(testcases))*100)
	log.Printf("总成本:%f元\n", totalCost)
	log.Printf("============================================")
}

func (b *BenchmarkRunner) runSingleTest(ctx context.Context, tc TestCase) TestResult {
	startTime := time.Now()
	workDir, _ := os.Getwd()
	workDir += fmt.Sprintf("/%s_%d", tc.ID, time.Now().Unix())
	_ = os.MkdirAll(workDir, 0755)
	if tc.SetupScript != "" {
		cmd := exec.Command("powershell", "-Command", tc.SetupScript)
		cmd.Dir = workDir
		if err := cmd.Run(); err != nil {
			return TestResult{
				TestCaseID: tc.ID,
				Passed:     false,
				ErrMsg:     fmt.Sprintf("靶机Setup失败:%v", err),
			}
		}
	}
	provider := provider.NewOpenAIProvider(b.modelName)
	session := ctxpkg.GlobalSessionManager.GetOrCreate(fmt.Sprintf("chat_front_%s", tc.ID), workDir)
	trackerProvider := observatibility.NewCostTracker(provider, b.modelName, session)
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))
	eng := engine.NewAgentEngine(trackerProvider, registry, false, false, false)
	session.Append(
		schema.Message{
			Role:    schema.RoleUser,
			Content: tc.TaskPrompt,
		},
	)
	err := eng.Run(ctx, session, nil)
	if err != nil {
		return TestResult{
			TestCaseID: tc.ID,
			Passed:     false,
			ErrMsg:     fmt.Sprintf("Agent崩溃:%v", err),
		}
	}
	cmdValidate := exec.Command("powershell", "-Command", tc.ValidateScript)
	cmdValidate.Dir = workDir
	out, err := cmdValidate.CombinedOutput()
	duration := time.Since(startTime).Seconds()
	if err != nil {
		return TestResult{
			TestCaseID: tc.ID,
			Passed:     false,

			TotalCostCNY: session.TotalCostCNY,

			Duration: duration,
			ErrMsg:   fmt.Sprintf("验证脚本执行失败:%s", string(out)),
		}
	}
	return TestResult{
		TestCaseID:   tc.ID,
		Passed:       true,
		TotalCostCNY: session.TotalCostCNY,
		Duration:     duration,
	}

}
