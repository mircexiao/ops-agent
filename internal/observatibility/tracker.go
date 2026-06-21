package observatibility

import (
	"context"
	"log"
	"time"

	ctxpkg "github.com/mircexiao/go-tiny-claw/internal/context"
	"github.com/mircexiao/go-tiny-claw/internal/provider"
	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

var PricingModel = map[string]struct {
	InputPrice  float64
	OutputPrice float64
}{
	"deepseek-chat": {
		InputPrice:  0.15,
		OutputPrice: 0.15,
	},
}

type CostTracker struct {
	nextProvider provider.LLMProvider
	modelName    string
	session      *ctxpkg.Session
}

func NewCostTracker(nextProvider provider.LLMProvider, modelName string, session *ctxpkg.Session) *CostTracker {
	return &CostTracker{
		nextProvider: nextProvider,
		modelName:    modelName,
		session:      session,
	}
}

func (t *CostTracker) Generate(ctx context.Context, msgs []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	startTime := time.Now()
	resp, err := t.nextProvider.Generate(ctx, msgs, availableTools)

	latency := time.Since(startTime)

	if err != nil {
		log.Printf("[Tracker] 模型失败,耗时:%v", latency)
		return resp, err
	}
	if resp.Usage != nil {
		promptTokens := int(resp.Usage.PromptTokens)
		completionTokens := int(resp.Usage.CompletionTokens)
		var cost float64
		if price, exist := PricingModel[t.modelName]; exist {
			cost = (float64(promptTokens)*price.InputPrice + float64(completionTokens)*price.OutputPrice) / 1000000.0
		}
		log.Printf("[Tracker] 模型:%s,耗时:%v,提示token:%d,完成token:%d,成本:%f", t.modelName, latency, promptTokens, completionTokens, cost)
		if t.session != nil {
			t.session.RecordUsage(promptTokens, completionTokens, cost)
		}
	} else {
		log.Printf("[Tracker]模型未返回费用数据，耗时:%v", latency)
	}
	return resp, nil
}
