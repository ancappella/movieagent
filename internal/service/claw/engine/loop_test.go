package engine

import (
	"context"
	"strings"
	"testing"

	"movieagent/internal/service/claw/schema"
	"movieagent/internal/service/claw/tools"
)

type stubProvider struct {
	model string
	calls []schema.ChatRequest
}

func (s *stubProvider) Model() string     { return s.model }
func (s *stubProvider) Configured() bool  { return true }

func (s *stubProvider) Chat(_ context.Context, req schema.ChatRequest) (*schema.ChatResponse, error) {
	s.calls = append(s.calls, req)
	if req.Tools == nil {
		return &schema.ChatResponse{
			Message: schema.Message{Role: schema.RoleAssistant, Content: "thinking notes"},
		}, nil
	}
	return &schema.ChatResponse{
		Message:      schema.Message{Role: schema.RoleAssistant, Content: "final answer"},
		FinishReason: "stop",
	}, nil
}

func TestRunFirstThinkingUsesLeanSystemThenActionEnriches(t *testing.T) {
	reg := tools.NewRegistry()
	tools.RegisterBuiltins(reg)

	p := &stubProvider{model: "stub"}
	eng := New(p, reg, Config{
		MaxTurns:             1,
		EnableThinking:       true,
		ThinkingSystemPrompt: "THINK_ONLY",
		ActionSystemPrompt:   "ACTION_WITH_SKILLS_AND_TOOLS",
	})

	_, err := eng.Run(context.Background(), []schema.Message{
		{Role: schema.RoleSystem, Content: "THINK_ONLY"},
		{Role: schema.RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(p.calls))
	}
	if p.calls[0].Tools != nil {
		t.Fatal("first call should be Thinking (Tools=nil)")
	}
	sys0 := firstSystem(p.calls[0].Messages)
	if sys0 != "THINK_ONLY" {
		t.Fatalf("thinking system = %q", sys0)
	}
	if strings.Contains(sys0, "SKILLS") {
		t.Fatalf("thinking unexpectedly rich: %q", sys0)
	}
	if p.calls[1].Tools == nil {
		t.Fatal("second call should be Action (Tools set)")
	}
	sys1 := firstSystem(p.calls[1].Messages)
	if sys1 != "ACTION_WITH_SKILLS_AND_TOOLS" {
		t.Fatalf("action system = %q", sys1)
	}
}

func firstSystem(msgs []schema.Message) string {
	for _, m := range msgs {
		if m.Role == schema.RoleSystem {
			return m.Content
		}
	}
	return ""
}
