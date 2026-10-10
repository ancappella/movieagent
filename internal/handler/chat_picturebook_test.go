package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"movieagent/internal/service/agents/brandagent"
	svcconfig "movieagent/internal/service/config"
	svctools "movieagent/internal/service/tools"
)

type fakeOSS struct {
	url string
}

func (f fakeOSS) Enabled() bool { return true }
func (f fakeOSS) Put(_ context.Context, key string, _ []byte, _ string) (string, error) {
	if f.url != "" {
		return f.url, nil
	}
	return "https://cdn.example.com/" + key, nil
}

type failingOSS struct{}

func (failingOSS) Enabled() bool { return true }
func (failingOSS) Put(context.Context, string, []byte, string) (string, error) {
	return "", context.Canceled
}

func TestPublishPicturebookCopiesToMedia(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEER_FLOW_HOME", home)

	paths, err := svcconfig.NewPaths(nil)
	if err != nil {
		t.Fatal(err)
	}
	userID := brandagent.DefaultUserID
	if err := paths.EnsureThreadDirs("chat", &userID); err != nil {
		t.Fatal(err)
	}
	hostOut, err := paths.ResolveVirtualPath("chat", svctools.PictureBookDefaultOut, &userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(hostOut), 0o755); err != nil {
		t.Fatal(err)
	}
	// Minimal valid-ish PNG header + payload bytes for the copy path.
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	}
	if err := os.WriteFile(hostOut, png, 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	h := &ChatHandler{Paths: paths, ProjectRoot: root}
	url, title := h.publishPicturebook(map[string]any{
		"path":   svctools.PictureBookDefaultOut,
		"width":  512,
		"height": 768,
	})
	if !strings.HasPrefix(url, "/media/generated/chat-") || !strings.HasSuffix(url, ".png") {
		t.Fatalf("url=%q", url)
	}
	if title != "插画 512x768" {
		t.Fatalf("title=%q", title)
	}
	name := strings.TrimPrefix(url, "/media/generated/")
	got, err := os.ReadFile(filepath.Join(root, "tmp", "generated", name))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(png) {
		t.Fatalf("copied %d bytes, want %d", len(got), len(png))
	}
}

func TestPublishPicturebookVideoCopiesToMedia(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEER_FLOW_HOME", home)

	paths, err := svcconfig.NewPaths(nil)
	if err != nil {
		t.Fatal(err)
	}
	userID := brandagent.DefaultUserID
	if err := paths.EnsureThreadDirs("chat", &userID); err != nil {
		t.Fatal(err)
	}
	hostOut, err := paths.ResolveVirtualPath("chat", svctools.PictureBookAnimateDefaultOut, &userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(hostOut), 0o755); err != nil {
		t.Fatal(err)
	}
	mp4 := []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6f, 0x6d}
	if err := os.WriteFile(hostOut, mp4, 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	h := &ChatHandler{Paths: paths, ProjectRoot: root}
	url, title := h.publishPicturebookVideo(map[string]any{
		"path":     svctools.PictureBookAnimateDefaultOut,
		"template": "character_spin",
		"width":    512,
		"height":   768,
	})
	if !strings.HasPrefix(url, "/media/generated/chat-") || !strings.HasSuffix(url, ".mp4") {
		t.Fatalf("url=%q", url)
	}
	if title != "动画 character_spin 512x768" {
		t.Fatalf("title=%q", title)
	}
	name := strings.TrimPrefix(url, "/media/generated/")
	got, err := os.ReadFile(filepath.Join(root, "tmp", "generated", name))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(mp4) {
		t.Fatalf("copied %d bytes, want %d", len(got), len(mp4))
	}
}

func TestPublishPicturebookPrefersOSSURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEER_FLOW_HOME", home)
	t.Setenv("OSS_KEY_PREFIX", "movieagent/chat/")

	paths, err := svcconfig.NewPaths(nil)
	if err != nil {
		t.Fatal(err)
	}
	userID := brandagent.DefaultUserID
	if err := paths.EnsureThreadDirs("chat", &userID); err != nil {
		t.Fatal(err)
	}
	hostOut, err := paths.ResolveVirtualPath("chat", svctools.PictureBookDefaultOut, &userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(hostOut), 0o755); err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	if err := os.WriteFile(hostOut, png, 0o644); err != nil {
		t.Fatal(err)
	}

	h := &ChatHandler{
		Paths:       paths,
		ProjectRoot: t.TempDir(),
		Objects:     fakeOSS{url: "https://cdn.example.com/movieagent/chat/demo.png"},
	}
	url, _ := h.publishPicturebook(map[string]any{"path": svctools.PictureBookDefaultOut})
	if url != "https://cdn.example.com/movieagent/chat/demo.png" {
		t.Fatalf("url=%q", url)
	}
}

func TestPublishPicturebookVideoPrefersOSSURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEER_FLOW_HOME", home)
	t.Setenv("OSS_KEY_PREFIX", "movieagent/chat/")

	paths, err := svcconfig.NewPaths(nil)
	if err != nil {
		t.Fatal(err)
	}
	userID := brandagent.DefaultUserID
	if err := paths.EnsureThreadDirs("chat", &userID); err != nil {
		t.Fatal(err)
	}
	hostOut, err := paths.ResolveVirtualPath("chat", svctools.PictureBookAnimateDefaultOut, &userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(hostOut), 0o755); err != nil {
		t.Fatal(err)
	}
	mp4 := []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70}
	if err := os.WriteFile(hostOut, mp4, 0o644); err != nil {
		t.Fatal(err)
	}

	h := &ChatHandler{
		Paths:       paths,
		ProjectRoot: t.TempDir(),
		Objects:     fakeOSS{url: "https://cdn.example.com/movieagent/chat/demo.mp4"},
	}
	url, title := h.publishPicturebookVideo(map[string]any{
		"path":     svctools.PictureBookAnimateDefaultOut,
		"template": "character_spin",
	})
	if url != "https://cdn.example.com/movieagent/chat/demo.mp4" {
		t.Fatalf("url=%q", url)
	}
	if !strings.Contains(title, "character_spin") {
		t.Fatalf("title=%q", title)
	}
}

func TestPublishPicturebookVideoFailsWhenOSSEnabledButPutFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEER_FLOW_HOME", home)

	paths, err := svcconfig.NewPaths(nil)
	if err != nil {
		t.Fatal(err)
	}
	userID := brandagent.DefaultUserID
	if err := paths.EnsureThreadDirs("chat", &userID); err != nil {
		t.Fatal(err)
	}
	hostOut, err := paths.ResolveVirtualPath("chat", svctools.PictureBookAnimateDefaultOut, &userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(hostOut), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostOut, []byte("mp4"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := &ChatHandler{
		Paths:       paths,
		ProjectRoot: t.TempDir(),
		Objects:     failingOSS{},
	}
	url, _ := h.publishPicturebookVideo(map[string]any{"path": svctools.PictureBookAnimateDefaultOut})
	if url != "" {
		t.Fatalf("expected empty url when OSS put fails, got %q", url)
	}
}
