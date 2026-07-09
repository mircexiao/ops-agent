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

func (t *CostTracker) GenerateStream(ctx context.Context, msgs []schema.Message, availableTools []schema.ToolDefinition) (<-chan schema.StreamEvent, error) {
	startTime := time.Now()
	upstream, err := t.nextProvider.GenerateStream(ctx, msgs, availableTools)
	if err != nil {
		latency := time.Since(startTime)
		log.Printf("[Tracker] 模型失败,耗时:%v", latency)
		return upstream, err
	}

	ch := make(chan schema.StreamEvent, 10)
	go func() {
		defer close(ch)
		var promptTokens, completionTokens int64
		for event := range upstream {
			if event.Type == schema.EventUsage && event.Usage != nil {
				promptTokens = int64(event.Usage.PromptTokens)
				completionTokens = int64(event.Usage.CompletionTokens)
			}
			ch <- event
		}

		latency := time.Since(startTime)
		if promptTokens > 0 || completionTokens > 0 {
			var cost float64
			if price, exist := PricingModel[t.modelName]; exist {
				cost = (float64(promptTokens)*price.InputPrice + float64(completionTokens)*price.OutputPrice) / 1000000.0
			}
			log.Printf("[Tracker] 模型:%s,耗时:%v,提示token:%d,完成token:%d,成本:%f", t.modelName, latency, promptTokens, completionTokens, cost)
			if t.session != nil {
				t.session.RecordUsage(int(promptTokens), int(completionTokens), cost)
			}
		} else {
			log.Printf("[Tracker]模型未返回费用数据，耗时:%v", latency)
		}
	}()
	return ch, nil
}
