package engine

import (
	"testing"

	"github.com/mircexiao/go-tiny-claw/internal/schema"
)

func TestReminder_ThreeIdenticalFailsTrigger(t *testing.T) {
	inj := NewReminderInjector()
	call := schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"ls"}`)}
	fail := schema.ToolResult{IsError: true, Output: "err"}

	var got *schema.Message
	for i := 0; i < 3; i++ {
		got = inj.CheckAndInject(call, fail)
	}
	if got == nil {
		t.Fatal("expected reminder after 3 identical failures, got nil")
	}
}

func TestReminder_InterleavedSuccessKeepsOtherStreak(t *testing.T) {
	inj := NewReminderInjector()
	failCall := schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"ls"}`)}
	okCall := schema.ToolCall{Name: "read", Arguments: []byte(`{"path":"x"}`)}
	fail := schema.ToolResult{IsError: true, Output: "err"}
	ok := schema.ToolResult{IsError: false, Output: "ok"}

	inj.CheckAndInject(failCall, fail) // bash 失败计数=1
	inj.CheckAndInject(failCall, fail) // bash 失败计数=2
	inj.CheckAndInject(okCall, ok)     // read 成功：只清 read 自己的指纹，不动 bash
	got := inj.CheckAndInject(failCall, fail) // bash 失败计数=3 → 应触发

	if got == nil {
		t.Fatal("delete-only reset failed: interleaved success should NOT wipe the bash streak")
	}
}

func TestReminder_SameToolSuccessResetsOwnStreak(t *testing.T) {
	inj := NewReminderInjector()
	call := schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"ls"}`)}
	fail := schema.ToolResult{IsError: true, Output: "err"}
	ok := schema.ToolResult{IsError: false, Output: "ok"}

	inj.CheckAndInject(call, fail)
	inj.CheckAndInject(call, fail)
	inj.CheckAndInject(call, ok) // 同一工具同一参数成功 → 自己的计数应清零

	// 再失败两次，不应达到 3（因为成功时已被 delete）
	inj.CheckAndInject(call, fail)
	got := inj.CheckAndInject(call, fail)
	if got != nil {
		t.Fatal("same-tool success should reset its own streak; unexpected reminder fired")
	}
}
