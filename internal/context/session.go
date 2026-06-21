package context

import (
	"sync"
	"time"

	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

type Session struct {
	ID                    string
	WorkDir               string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	history               []schema.Message
	mu                    sync.Mutex
	TotalPromptTokens     int
	TotalCompletionTokens int
	TotalCostCNY          float64
}

func NewSession(id string, workDir string) *Session {
	return &Session{
		ID:        id,
		WorkDir:   workDir,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		history:   make([]schema.Message, 0),
	}
}

func (s *Session) Append(msgs ...schema.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, msgs...)
	s.UpdatedAt = time.Now()
	// 保存至磁盘
	// s.saveToDisk()
}
func (s *Session) GetWorkingMemory(limit int) []schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := len(s.history)
	if total <= limit || limit <= 0 {
		res := make([]schema.Message, total)
		copy(res, s.history)
		return res
	}
	res := make([]schema.Message, limit)
	copy(res, s.history[total-limit:])
	for len(res) > 0 {
		if res[0].Role == schema.RoleUser && res[0].ToolCallID != "" {
			res = res[1:]
		} else {
			break
		}
	}
	return res
}

func (s *Session) RecordUsage(prompt int, complete int, cost float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalPromptTokens += prompt
	s.TotalCompletionTokens += complete
	s.TotalCostCNY += cost
}

type SessionManager struct {
	sessions map[string]*Session
	mu       sync.Mutex
}

var GlobalSessionManager = &SessionManager{
	sessions: make(map[string]*Session),
}

func (mg *SessionManager) GetOrCreate(id string, workDir string) *Session {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	if sess, exist := mg.sessions[id]; exist {
		return sess
	}
	sess := NewSession(id, workDir)
	mg.sessions[id] = sess
	return sess
}
