package observatibility

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type traceKey struct{}

type Span struct {
	Name            string                 `json:"name"`
	StartTime       time.Time              `json:"startTime"`
	EndTime         time.Time              `json:"endTime"`
	Duration_Second float64                `json:"durationSecond"`
	Attributes      map[string]interface{} `json:"attributes"`
	Children        []*Span                `json:"children,omitempty"`

	mu sync.Mutex
}

func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	span := &Span{
		Name:       name,
		StartTime:  time.Now(),
		Attributes: make(map[string]interface{}),
	}
	if parentSpan, exist := ctx.Value(traceKey{}).(*Span); exist {
		parentSpan.mu.Lock()
		parentSpan.Children = append(parentSpan.Children, span)
		parentSpan.mu.Unlock()
	}
	newCtx := context.WithValue(ctx, traceKey{}, span)
	return newCtx, span
}

func (s *Span) EndSpan() {
	s.EndTime = time.Now()
	s.Duration_Second = s.EndTime.Sub(s.StartTime).Seconds()
}

func (s *Span) AddAttribute(key string, value interface{}) {
	s.mu.Lock()
	s.Attributes[key] = value
	s.mu.Unlock()
}

func ExportTraceToFile(rootSpan *Span, workDir string, sessionId string) error {
	traceDir := filepath.Join(workDir, ".claw", "traces")
	os.MkdirAll(traceDir, 0755)
	fileName := filepath.Join(traceDir, fmt.Sprintf("trace_%s_%d.json", sessionId, time.Now().Unix()))
	data, err := json.MarshalIndent(rootSpan, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(fileName, data, 0644)
}
