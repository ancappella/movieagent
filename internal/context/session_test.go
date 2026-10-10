package context

import (
	"strings"
	"testing"
)

func TestSessionSystemPatchAndReplay(t *testing.T) {
	s := NewSession("t1", "/tmp/proj")
	s.SetSection(SectionInstructions, "you are helpful")
	s.SetTools([]ToolInfo{{Name: "picturebook_illustrate", Description: "draw"}})
	s.DeleteSection(SectionTools)
	s.SetSection(SectionSkills, "skill body")

	replay := s.Replay()
	if len(replay) < 1 || replay[0].Kind != KindSystem {
		t.Fatalf("replay system missing: %+v", replay)
	}
	if _, ok := replay[0].Sections[SectionTools]; ok {
		t.Fatal("tools section should be deleted")
	}
	if replay[0].Sections[SectionSkills] != "skill body" {
		t.Fatalf("skills = %q", replay[0].Sections[SectionSkills])
	}
}

func TestSessionWireTurn(t *testing.T) {
	s := NewSession("t2", "/proj")
	s.SetSection(SectionInstructions, "sys")
	s.AppendUser("draw a fox")
	s.AppendAssistant("thinking of fox", "plan", ToolCallReq{
		ID: "c1", Name: "picturebook_illustrate", Arguments: `{"prompt":"fox"}`,
	})
	s.AppendToolResult("c1", "picturebook_illustrate", "ok: wrote png", false)
	s.AppendAssistant("here is the art", "")

	wire := s.WireMessages()
	if len(wire) < 5 {
		t.Fatalf("wire len=%d %+v", len(wire), wire)
	}
	if wire[0].Role != "system" || !strings.Contains(wire[0].Content, "sys") {
		t.Fatalf("system = %+v", wire[0])
	}
	if wire[1].Role != "user" || wire[1].Content != "draw a fox" {
		t.Fatalf("user = %+v", wire[1])
	}
	if wire[2].Role != "assistant" || len(wire[2].ToolCalls) != 1 {
		t.Fatalf("assistant = %+v", wire[2])
	}
	if wire[3].Role != "tool" || wire[3].ToolCallID != "c1" {
		t.Fatalf("tool = %+v", wire[3])
	}
}

func TestApplyPhaseThinkingOmitsSkillsTools(t *testing.T) {
	b := NewBuilder(t.TempDir()).WithTools(ToolInfo{Name: "picturebook_illustrate", Description: "draw"})
	s := NewSession("t3", t.TempDir())
	s.ApplyPhase(b, PhaseThinking)
	sys := s.Replay()[0]
	if sys.Sections[SectionSkills] != "" || sys.Sections[SectionTools] != "" {
		t.Fatalf("thinking should omit skills/tools: %+v", sys.Sections)
	}
	if !strings.Contains(sys.Sections[SectionSchedule], "picturebook-illustrate") {
		t.Fatalf("schedule missing: %+v", sys.Sections)
	}
	if !strings.Contains(sys.Sections[SectionSchedule], "picturebook-animate") {
		t.Fatalf("animate schedule missing: %+v", sys.Sections)
	}
	s.ApplyPhase(b, PhaseAction)
	sys = s.Replay()[0]
	if !strings.Contains(sys.Sections[SectionTools], "picturebook_illustrate") {
		t.Fatalf("action tools missing: %+v", sys.Sections)
	}
}
