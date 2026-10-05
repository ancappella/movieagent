package sandbox_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"movieagent/internal/service/config"
	"movieagent/internal/service/sandbox"
)

func TestBuildSandboxEnvStripsSecrets(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("GEMINI_API_KEY", "gemini-test")
	t.Setenv("PATH", "/usr/bin")
	env := sandbox.BuildSandboxEnv(map[string]string{"SKILL_TOKEN": "req"})
	if _, ok := env["OPENAI_API_KEY"]; ok {
		t.Fatal("platform key leaked into sandbox env")
	}
	if env["GEMINI_API_KEY"] != "gemini-test" {
		t.Fatalf("GEMINI_API_KEY passthrough = %q", env["GEMINI_API_KEY"])
	}
	if env["PATH"] == "" {
		t.Fatal("PATH should be preserved")
	}
	if env["SKILL_TOKEN"] != "req" {
		t.Fatalf("injected secret = %q", env["SKILL_TOKEN"])
	}
}

func TestLocalSandboxWriteReadAndCommand(t *testing.T) {
	base := t.TempDir()
	paths, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	provider := sandbox.NewLocalProvider(paths)
	id, err := provider.Acquire("t1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if id != "local:alice:t1" {
		t.Fatalf("id = %q", id)
	}
	sb, ok := provider.Get(id)
	if !ok {
		t.Fatal("sandbox not cached")
	}

	if err := sb.WriteFile("/mnt/user-data/workspace/hello.txt", "hello sandbox", false); err != nil {
		t.Fatal(err)
	}
	got, err := sb.ReadFile("/mnt/user-data/workspace/hello.txt", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello sandbox" {
		t.Fatalf("read = %q", got)
	}

	hostFile := filepath.Join(base, "users", "alice", "threads", "t1", "user-data", "workspace", "hello.txt")
	if _, err := os.Stat(hostFile); err != nil {
		t.Fatal(err)
	}

	out, err := sb.ExecuteCommand(context.Background(), "cat /mnt/user-data/workspace/hello.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello sandbox") {
		t.Fatalf("command output = %q", out)
	}
	if strings.Contains(out, base) {
		t.Fatalf("host path leaked: %s", out)
	}

	entries, err := sb.ListDir("/mnt/user-data", 1)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(entries, ",")
	if !strings.Contains(joined, "/mnt/user-data/workspace") {
		t.Fatalf("list = %v", entries)
	}
}

func acquireThread(t *testing.T, base, userID, threadID string) sandbox.Sandbox {
	t.Helper()
	paths, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	provider := sandbox.NewLocalProvider(paths)
	id, err := provider.Acquire(threadID, userID)
	if err != nil {
		t.Fatal(err)
	}
	sb, ok := provider.Get(id)
	if !ok {
		t.Fatal("sandbox missing after acquire")
	}
	return sb
}

func TestAgentWritesScriptAndRunsIt(t *testing.T) {
	base := t.TempDir()
	t.Setenv("OPENAI_API_KEY", "sk-platform")
	sb := acquireThread(t, base, "alice", "script")

	script := "#!/bin/sh\nprintf 'token=%s\\n' \"$SKILL_TOKEN\"\nprintf 'key=%s\\n' \"$OPENAI_API_KEY\"\n"
	if err := sb.WriteFile("/mnt/user-data/workspace/run.sh", script, false); err != nil {
		t.Fatal(err)
	}
	out, err := sb.ExecuteCommand(context.Background(), "sh /mnt/user-data/workspace/run.sh", map[string]string{
		"SKILL_TOKEN": "request-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "token=request-secret") {
		t.Fatalf("request secret missing: %q", out)
	}
	if strings.Contains(out, "sk-platform") {
		t.Fatalf("platform key leaked into script output: %q", out)
	}
}

func TestThreadsAreIsolated(t *testing.T) {
	base := t.TempDir()
	alice := acquireThread(t, base, "alice", "t1")
	bob := acquireThread(t, base, "bob", "t1")

	if err := alice.WriteFile("/mnt/user-data/workspace/note.txt", "alice-only", false); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.ReadFile("/mnt/user-data/workspace/note.txt", 0, 0); err == nil {
		t.Fatal("bob should not see alice's workspace file")
	}
	if err := bob.WriteFile("/mnt/user-data/outputs/bob.txt", "bob-only", false); err != nil {
		t.Fatal(err)
	}
	aliceHost := filepath.Join(base, "users", "alice", "threads", "t1", "user-data", "workspace", "note.txt")
	bobHost := filepath.Join(base, "users", "bob", "threads", "t1", "user-data", "outputs", "bob.txt")
	if _, err := os.Stat(aliceHost); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bobHost); err != nil {
		t.Fatal(err)
	}
}

func TestPathEscapeIsRejected(t *testing.T) {
	base := t.TempDir()
	sb := acquireThread(t, base, "alice", "escape")
	err := sb.WriteFile("/mnt/user-data/workspace/../../outside.txt", "nope", false)
	if err == nil {
		t.Fatal("expected path escape to be rejected")
	}
	outside := filepath.Join(base, "outside.txt")
	if _, statErr := os.Stat(outside); !os.IsNotExist(statErr) {
		t.Fatalf("escaped file was created: %v", statErr)
	}
}

func TestAppendAndReadLineRange(t *testing.T) {
	base := t.TempDir()
	sb := acquireThread(t, base, "alice", "edit")
	if err := sb.WriteFile("/mnt/user-data/workspace/story.txt", "one\n", false); err != nil {
		t.Fatal(err)
	}
	if err := sb.WriteFile("/mnt/user-data/workspace/story.txt", "two\nthree\n", true); err != nil {
		t.Fatal(err)
	}
	line, err := sb.ReadFile("/mnt/user-data/workspace/story.txt", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if line != "two" {
		t.Fatalf("line 2 = %q", line)
	}
}

func TestCommandFailureKeepsExitCode(t *testing.T) {
	base := t.TempDir()
	sb := acquireThread(t, base, "alice", "fail")
	out, err := sb.ExecuteCommand(context.Background(), "ls /mnt/user-data/workspace/missing-file", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Exit Code:") {
		t.Fatalf("expected exit code in output, got %q", out)
	}
}

func TestReleaseDropsCachedSandbox(t *testing.T) {
	base := t.TempDir()
	paths, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	provider := sandbox.NewLocalProvider(paths)
	id, err := provider.Acquire("t9", "alice")
	if err != nil {
		t.Fatal(err)
	}
	provider.Release(id)
	if _, ok := provider.Get(id); ok {
		t.Fatal("released sandbox should not stay cached")
	}
}

func TestReadOnlySkillMount(t *testing.T) {
	base := t.TempDir()
	public := filepath.Join(base, "skills_view", "public")
	if err := os.MkdirAll(public, 0o755); err != nil {
		t.Fatal(err)
	}
	paths, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	provider := sandbox.NewLocalProvider(paths)
	id, err := provider.Acquire("t1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	sb, _ := provider.Get(id)
	err = sb.WriteFile("/mnt/skills/public/x.txt", "nope", false)
	if err == nil {
		t.Fatal("expected read-only error")
	}
}

// agentState is the slice of agent state a sandbox tool reads and writes.
type agentState map[string]string

// sandboxTool is the tool-call path: authorize, then reuse or acquire.
type sandboxTool struct {
	provider *sandbox.LocalProvider
	role     string
	userID   string
	threadID string
	state    agentState
	allow    func(role string) bool
}

func (t *sandboxTool) call() (sandbox.Sandbox, error) {
	// 授权检查 sandbox:execute。角色不允许就返回错误，不申请。
	if t.allow != nil && !t.allow(t.role) {
		return nil, sandbox.ErrAuthorization(t.role)
	}

	if id := t.state["sandbox_id"]; id != "" {
		sb, ok := t.provider.Get(id)
		if !ok {
			return nil, sandbox.ErrNotFound(id)
		}
		return sb, nil
	}

	id, err := t.provider.Acquire(t.threadID, t.userID)
	if err != nil {
		return nil, err
	}
	t.state["sandbox_id"] = id
	sb, ok := t.provider.Get(id)
	if !ok {
		return nil, sandbox.ErrNotFound(id)
	}
	return sb, nil
}

func TestToolCallDeniedDoesNotAcquire(t *testing.T) {
	base := t.TempDir()
	paths, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	tool := &sandboxTool{
		provider: sandbox.NewLocalProvider(paths),
		role:     "viewer",
		userID:   "alice",
		threadID: "t1",
		state:    agentState{},
		allow:    func(role string) bool { return role == "admin" || role == "agent" },
	}

	sb, err := tool.call()
	if sb != nil {
		t.Fatal("denied role must not receive a sandbox")
	}
	var denied *sandbox.AuthorizationError
	if !errors.As(err, &denied) {
		t.Fatalf("err = %v, want AuthorizationError", err)
	}
	if tool.state["sandbox_id"] != "" {
		t.Fatalf("state was written on deny: %#v", tool.state)
	}
	if _, ok := tool.provider.Get("local:alice:t1"); ok {
		t.Fatal("provider acquired a sandbox after authorization denial")
	}
}

func TestToolCallAcquireThenReuseSameSandbox(t *testing.T) {
	base := t.TempDir()
	paths, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	tool := &sandboxTool{
		provider: sandbox.NewLocalProvider(paths),
		role:     "agent",
		userID:   "alice",
		threadID: "t1",
		state:    agentState{},
		allow:    func(role string) bool { return role == "agent" },
	}

	first, err := tool.call()
	if err != nil {
		t.Fatal(err)
	}
	if tool.state["sandbox_id"] != "local:alice:t1" {
		t.Fatalf("state = %#v", tool.state)
	}
	if err := first.WriteFile("/mnt/user-data/workspace/draft.txt", "scene-1", false); err != nil {
		t.Fatal(err)
	}

	// 下一次工具调用已经有 sandbox_id，必须拿到 LRU 里的同一个实例。
	second, err := tool.call()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("expected the cached LocalSandbox, not a new acquire")
	}
	got, err := second.ReadFile("/mnt/user-data/workspace/draft.txt", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "scene-1" {
		t.Fatalf("read = %q", got)
	}
	out, err := second.ExecuteCommand(context.Background(), "cat /mnt/user-data/workspace/draft.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "scene-1") {
		t.Fatalf("command output = %q", out)
	}
}
