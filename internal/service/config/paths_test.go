package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathsLayout(t *testing.T) {
	base := t.TempDir()
	p, err := NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}

	memory, err := p.MemoryFile()
	if err != nil || memory != filepath.Join(base, "memory.json") {
		t.Fatalf("memory = %q err=%v", memory, err)
	}
	userMD, err := p.UserMDFile()
	if err != nil || userMD != filepath.Join(base, "USER.md") {
		t.Fatalf("USER.md = %q", userMD)
	}
	sub, err := p.ManagedSubagentFile("Research")
	if err != nil || sub != filepath.Join(base, "managed-subagents", "research.json") {
		t.Fatalf("subagent = %q", sub)
	}
	pub, err := p.PublicSkillsViewDir()
	if err != nil || pub != filepath.Join(base, "skills_view", "public") {
		t.Fatalf("public skills = %q", pub)
	}
	pkg, err := p.IntegrationSkillDir("github", "issues")
	if err != nil || pkg != filepath.Join(base, "integrations", "skills", "github", "issues") {
		t.Fatalf("integration skill = %q", pkg)
	}
}

func TestUserThreadAndProjectPaths(t *testing.T) {
	base := t.TempDir()
	p, err := NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	user := "alice"
	thread := "t1"

	custom, err := p.UserCustomSkillsDir(user)
	if err != nil || !strings.HasSuffix(custom, filepath.Join("users", "alice", "skills", "custom")) {
		t.Fatalf("custom skills = %q err=%v", custom, err)
	}
	view, err := p.UserLegacySkillsViewDir(user)
	if err != nil || !strings.HasSuffix(view, filepath.Join("skills_view", "legacy")) {
		t.Fatalf("legacy view = %q", view)
	}

	ws, err := p.SandboxWorkDir(thread, &user)
	if err != nil {
		t.Fatal(err)
	}
	wantWS := filepath.Join(base, "users", "alice", "threads", "t1", "user-data", "workspace")
	if ws != wantWS {
		t.Fatalf("workspace = %q want %q", ws, wantWS)
	}
	acp, err := p.ACPWorkspaceDir(thread, &user)
	if err != nil || !strings.HasSuffix(acp, "acp-workspace") {
		t.Fatalf("acp = %q", acp)
	}
	tsv, err := p.ThreadSkillsViewDir(thread, user)
	if err != nil || !strings.HasSuffix(tsv, filepath.Join("t1", "skills_view")) {
		t.Fatalf("thread skills_view = %q", tsv)
	}

	sha := "ab" + strings.Repeat("c", 62)
	orig, err := p.ProjectDocumentOriginal(user, "proj1", sha, "doc1", "a.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(orig, filepath.Join("projects", "proj1", "documents", "ab", sha, "doc1", "original", "a.pdf")) {
		t.Fatalf("original = %q", orig)
	}

	cfg, err := p.UserIntegrationConfigDir(user, "slack")
	if err != nil || !strings.HasSuffix(cfg, filepath.Join("integrations", "slack", "config")) {
		t.Fatalf("integration config = %q", cfg)
	}

	legacy, err := p.ThreadDir(thread, nil)
	if err != nil || legacy != filepath.Join(base, "threads", "t1") {
		t.Fatalf("legacy thread = %q", legacy)
	}
}

func TestResolveVirtualPath(t *testing.T) {
	base := t.TempDir()
	p, err := NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	user := "alice"
	got, err := p.ResolveVirtualPath("t1", "/mnt/user-data/outputs/report.pdf", &user)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "users", "alice", "threads", "t1", "user-data", "outputs", "report.pdf")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := p.ResolveVirtualPath("t1", "/etc/passwd", &user); err == nil {
		t.Fatal("expected reject non-virtual path")
	}
}

func TestEnsureAndDeleteThreadDirs(t *testing.T) {
	base := t.TempDir()
	p, err := NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	user := "alice"
	if err := p.EnsureThreadDirs("t1", &user); err != nil {
		t.Fatal(err)
	}
	ws, _ := p.SandboxWorkDir("t1", &user)
	if _, err := os.Stat(ws); err != nil {
		t.Fatal(err)
	}
	if err := p.DeleteThreadDir("t1", &user); err != nil {
		t.Fatal(err)
	}
	dir, _ := p.ThreadDir("t1", &user)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("thread dir still exists: %v", err)
	}
}
