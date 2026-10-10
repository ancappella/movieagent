package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSkillMD(t *testing.T) {
	raw := "---\nname: picturebook-illustrate\ndescription: kids watercolor art\nlicense: MIT\n---\n\n# Body\n\nDo the thing.\n"
	got := parseSkillMD(raw)
	if got.Name != "picturebook-illustrate" {
		t.Fatalf("name = %q", got.Name)
	}
	if got.Description != "kids watercolor art" {
		t.Fatalf("description = %q", got.Description)
	}
	if !strings.Contains(got.Body, "Do the thing.") {
		t.Fatalf("body = %q", got.Body)
	}
}

func TestSkillLoaderLoadAll(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".claw", "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: demo\ndescription: when user asks for demo\ntool: demo_tool\nscript: scripts/run.py\n---\n\nFollow these steps carefully for the demo skill.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "run.py"), []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewSkillLoader(root)
	out := loader.LoadAll()
	if out == "" {
		t.Fatal("expected non-empty skill context")
	}
	for _, want := range []string{"demo", "Follow these steps", "demo_tool", "scripts/run.py"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in: %s", want, out)
		}
	}
	sk, ok := loader.Find("demo")
	if !ok {
		t.Fatal("Find demo failed")
	}
	if sk.ScriptPath() == "" {
		t.Fatal("expected ScriptPath")
	}
}

func TestSkillLoaderMissingDir(t *testing.T) {
	if got := NewSkillLoader(t.TempDir()).LoadAll(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
