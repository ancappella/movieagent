// Package storage uploads agent-runtime artifacts (chat images) to object storage.
package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// ObjectStore persists bytes and returns a browser-reachable URL.
type ObjectStore interface {
	Enabled() bool
	Put(ctx context.Context, key string, data []byte, contentType string) (publicURL string, err error)
}

// Config for Aliyun OSS.
type Config struct {
	Endpoint        string
	AccessKeyID     string
	AccessKeySecret string
	Bucket          string
	// PublicBase is optional CDN / custom domain, e.g. https://cdn.example.com
	PublicBase string
	// KeyPrefix e.g. "movieagent/chat/"
	KeyPrefix string
}

// LoadConfig reads OSS_* from the environment. Empty credentials ⇒ disabled.
func LoadConfig() Config {
	prefix := strings.TrimSpace(os.Getenv("OSS_KEY_PREFIX"))
	if prefix == "" {
		prefix = "movieagent/chat/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return Config{
		Endpoint:        strings.TrimSpace(firstEnv("OSS_ENDPOINT", "ALIBABA_CLOUD_OSS_ENDPOINT")),
		AccessKeyID:     strings.TrimSpace(firstEnv("OSS_ACCESS_KEY_ID", "ALIBABA_CLOUD_ACCESS_KEY_ID")),
		AccessKeySecret: strings.TrimSpace(firstEnv("OSS_ACCESS_KEY_SECRET", "ALIBABA_CLOUD_ACCESS_KEY_SECRET")),
		Bucket:          strings.TrimSpace(firstEnv("OSS_BUCKET", "OSS_BUCKET_NAME")),
		PublicBase:      strings.TrimRight(strings.TrimSpace(os.Getenv("OSS_PUBLIC_BASE_URL")), "/"),
		KeyPrefix:       prefix,
	}
}

func (c Config) Enabled() bool {
	return c.Endpoint != "" && c.AccessKeyID != "" && c.AccessKeySecret != "" && c.Bucket != ""
}

// NewFromEnv builds an OSS client when configured; otherwise a no-op store.
func NewFromEnv() ObjectStore {
	cfg := LoadConfig()
	if !cfg.Enabled() {
		return Nop{}
	}
	s, err := NewOSS(cfg)
	if err != nil {
		return Nop{}
	}
	return s
}

// Nop is disabled storage (local /media fallback).
type Nop struct{}

func (Nop) Enabled() bool { return false }

func (Nop) Put(context.Context, string, []byte, string) (string, error) {
	return "", fmt.Errorf("object storage not configured")
}

// ChatImageKey builds a unique object key under the configured prefix.
func ChatImageKey(prefix, filename string) string {
	filename = baseName(filename)
	day := time.Now().Format("2006/01/02")
	return strings.TrimPrefix(prefix, "/") + day + "/" + filename
}

func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func publicURL(cfg Config, key string) string {
	key = strings.TrimPrefix(key, "/")
	if cfg.PublicBase != "" {
		return cfg.PublicBase + "/" + key
	}
	ep := strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://")
	ep = strings.TrimSuffix(ep, "/")
	return fmt.Sprintf("https://%s.%s/%s", cfg.Bucket, ep, key)
}
