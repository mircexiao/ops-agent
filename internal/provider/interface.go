package provider

import (
	"context"

	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

type LLMProvider interface {
	Generate(ctx context.Context, messages []schema.Message, avaliableTiils []schema.ToolDefinition) (*schema.Message, error)
}
