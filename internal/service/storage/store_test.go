package storage

import (
	"context"
	"strings"
	"testing"
)

func TestLoadConfigDisabledByDefault(t *testing.T) {
	t.Setenv("OSS_ENDPOINT", "")
	t.Setenv("OSS_ACCESS_KEY_ID", "")
	t.Setenv("OSS_ACCESS_KEY_SECRET", "")
	t.Setenv("OSS_BUCKET", "")
	cfg := LoadConfig()
	if cfg.Enabled() {
		t.Fatal("expected disabled")
	}
	if NewFromEnv().Enabled() {
		t.Fatal("nop should be disabled")
	}
}

func TestChatImageKey(t *testing.T) {
	key := ChatImageKey("movieagent/chat/", "chat-demo.png")
	if !strings.HasPrefix(key, "movieagent/chat/") {
		t.Fatalf("prefix: %q", key)
	}
	if !strings.HasSuffix(key, "/chat-demo.png") {
		t.Fatalf("suffix: %q", key)
	}
}

func TestPublicURL(t *testing.T) {
	cfg := Config{
		Endpoint:   "oss-cn-hangzhou.aliyuncs.com",
		Bucket:     "my-bucket",
		PublicBase: "",
	}
	got := publicURL(cfg, "movieagent/chat/a.png")
	want := "https://my-bucket.oss-cn-hangzhou.aliyuncs.com/movieagent/chat/a.png"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	cfg.PublicBase = "https://cdn.example.com"
	got = publicURL(cfg, "movieagent/chat/a.png")
	if got != "https://cdn.example.com/movieagent/chat/a.png" {
		t.Fatalf("cdn url = %q", got)
	}
}

type memStore struct {
	lastKey string
	lastCT  string
	url     string
}

func (m *memStore) Enabled() bool { return true }
func (m *memStore) Put(_ context.Context, key string, data []byte, contentType string) (string, error) {
	m.lastKey = key
	m.lastCT = contentType
	if m.url != "" {
		return m.url, nil
	}
	return "https://cdn.example.com/" + key, nil
}

func TestMemStoreContract(t *testing.T) {
	m := &memStore{}
	u, err := m.Put(context.Background(), "k.png", []byte("x"), "image/png")
	if err != nil || !strings.Contains(u, "k.png") {
		t.Fatalf("url=%q err=%v", u, err)
	}
	if m.lastCT != "image/png" {
		t.Fatalf("ct=%q", m.lastCT)
	}
}
