package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuilderAssemblesSystemSkillsTools(t *testing.T) {
	root := t.TempDir()

	promptPath := filepath.Join(root, "internal", "context", "systemprompt.md")
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(promptPath, []byte("# Sys\n\n你是创意助手。\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(root, "skills", "public", "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: demo\ndescription: when user wants a demo\n---\n\nDo the demo carefully.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	out := NewBuilder(root).
		WithTools(ToolInfo{Name: "picturebook_illustrate", Description: "绘本插画"}).
		WithExtra("### 额外\n补充说明").
		Build()

	for _, want := range []string{
		"你是创意助手",
		"demo",
		"Do the demo carefully",
		"picturebook_illustrate",
		"绘本插画",
		"补充说明",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestBuilderFallsBackToEmbeddedPrompt(t *testing.T) {
	out := NewBuilder(t.TempDir()).Build()
	if !strings.Contains(out, "movieagent") {
		t.Fatalf("expected embedded system prompt, got %q", out)
	}
}

func TestBuilderThinkingOmitsSkillsAndTools(t *testing.T) {
	root := t.TempDir()

	promptPath := filepath.Join(root, "internal", "context", "systemprompt.md")
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(promptPath, []byte("# Sys\n\n你是创意助手。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, "skills", "public", "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: demo\ndescription: when user wants a demo\n---\n\nDo the demo carefully.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	b := NewBuilder(root).WithTools(ToolInfo{Name: "picturebook_illustrate", Description: "绘本插画"})
	thinking := b.BuildThinking()
	action := b.BuildAction()

	if !strings.Contains(thinking, "你是创意助手") {
		t.Fatalf("thinking missing system: %s", thinking)
	}
	for _, banned := range []string{"demo", "picturebook_illustrate", "可用专业技能", "可用工具"} {
		if strings.Contains(thinking, banned) {
			t.Fatalf("thinking should omit %q, got:\n%s", banned, thinking)
		}
	}
	for _, want := range []string{"demo", "picturebook_illustrate", "可用专业技能", "可用工具"} {
		if !strings.Contains(action, want) {
			t.Fatalf("action missing %q:\n%s", want, action)
		}
	}
}
