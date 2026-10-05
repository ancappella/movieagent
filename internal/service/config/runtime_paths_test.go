package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withEnv(t *testing.T, key, value string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	if value == "" {
		_ = os.Unsetenv(key)
	} else {
		_ = os.Setenv(key, value)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestProjectRootFromEnv(t *testing.T) {
	dir := t.TempDir()
	withEnv(t, "DEER_FLOW_PROJECT_ROOT", dir)

	got, err := ProjectRoot()
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}
	want, _ := filepath.Abs(dir)
	if got != filepath.Clean(want) {
		t.Fatalf("ProjectRoot = %q, want %q", got, want)
	}
}

func TestProjectRootEnvMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	withEnv(t, "DEER_FLOW_PROJECT_ROOT", missing)

	_, err := ProjectRoot()
	if err == nil {
		t.Fatal("expected error for missing DEER_FLOW_PROJECT_ROOT")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectRootEnvNotDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	withEnv(t, "DEER_FLOW_PROJECT_ROOT", file)

	_, err := ProjectRoot()
	if err == nil {
		t.Fatal("expected error when DEER_FLOW_PROJECT_ROOT is a file")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectRootFallsBackToCWD(t *testing.T) {
	withEnv(t, "DEER_FLOW_PROJECT_ROOT", "")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(cwd)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ProjectRoot()
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}
	if got != want {
		t.Fatalf("ProjectRoot = %q, want %q", got, want)
	}
}

func TestRuntimeHomeFromEnv(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "custom-home")
	withEnv(t, "DEER_FLOW_HOME", home)

	got, err := RuntimeHome()
	if err != nil {
		t.Fatalf("RuntimeHome: %v", err)
	}
	want, _ := filepath.Abs(home)
	if got != filepath.Clean(want) {
		t.Fatalf("RuntimeHome = %q, want %q", got, want)
	}
}

func TestRuntimeHomeDefaultUnderProjectRoot(t *testing.T) {
	root := t.TempDir()
	withEnv(t, "DEER_FLOW_HOME", "")
	withEnv(t, "DEER_FLOW_PROJECT_ROOT", root)

	got, err := RuntimeHome()
	if err != nil {
		t.Fatalf("RuntimeHome: %v", err)
	}
	want := filepath.Join(root, ".deer-flow")
	want, _ = filepath.Abs(want)
	if got != filepath.Clean(want) {
		t.Fatalf("RuntimeHome = %q, want %q", got, want)
	}
}

func TestResolvePathAbsolute(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "abs-file")
	got, err := ResolvePath(abs, "")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	want, _ := filepath.Abs(abs)
	if got != filepath.Clean(want) {
		t.Fatalf("ResolvePath = %q, want %q", got, want)
	}
}

func TestResolvePathRelativeWithBase(t *testing.T) {
	base := t.TempDir()
	got, err := ResolvePath("skills/demo", base)
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	want := filepath.Join(base, "skills", "demo")
	want, _ = filepath.Abs(want)
	if got != filepath.Clean(want) {
		t.Fatalf("ResolvePath = %q, want %q", got, want)
	}
}

func TestResolvePathRelativeUsesProjectRoot(t *testing.T) {
	root := t.TempDir()
	withEnv(t, "DEER_FLOW_PROJECT_ROOT", root)

	got, err := ResolvePath("configs/app.yaml", "")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	want := filepath.Join(root, "configs", "app.yaml")
	want, _ = filepath.Abs(want)
	if got != filepath.Clean(want) {
		t.Fatalf("ResolvePath = %q, want %q", got, want)
	}
}

func TestExistingProjectFileFindsFirstRegularFile(t *testing.T) {
	root := t.TempDir()
	withEnv(t, "DEER_FLOW_PROJECT_ROOT", root)

	if err := os.WriteFile(filepath.Join(root, "b.yaml"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "a.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ExistingProjectFile("missing.yaml", "a.yaml", "b.yaml")
	if err != nil {
		t.Fatalf("ExistingProjectFile: %v", err)
	}
	want := filepath.Join(root, "b.yaml")
	if got != want {
		t.Fatalf("ExistingProjectFile = %q, want %q", got, want)
	}
}

func TestExistingProjectFileNone(t *testing.T) {
	root := t.TempDir()
	withEnv(t, "DEER_FLOW_PROJECT_ROOT", root)

	got, err := ExistingProjectFile("nope1.yaml", "nope2.yaml")
	if err != nil {
		t.Fatalf("ExistingProjectFile: %v", err)
	}
	if got != "" {
		t.Fatalf("ExistingProjectFile = %q, want empty", got)
	}
}
