package storage

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSS is an Aliyun Object Storage Service client.
type OSS struct {
	cfg    Config
	bucket *oss.Bucket
}

// NewOSS connects to the configured bucket.
func NewOSS(cfg Config) (*OSS, error) {
	if !cfg.Enabled() {
		return nil, fmt.Errorf("oss config incomplete")
	}
	endpoint := cfg.Endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	client, err := oss.New(endpoint, cfg.AccessKeyID, cfg.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("oss client: %w", err)
	}
	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("oss bucket: %w", err)
	}
	return &OSS{cfg: cfg, bucket: bucket}, nil
}

func (s *OSS) Enabled() bool { return s != nil && s.bucket != nil }

func (s *OSS) Put(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if !s.Enabled() {
		return "", fmt.Errorf("oss not enabled")
	}
	_ = ctx
	key = strings.TrimPrefix(key, "/")
	opts := []oss.Option{}
	if ct := strings.TrimSpace(contentType); ct != "" {
		opts = append(opts, oss.ContentType(ct))
	}
	opts = append(opts, oss.ObjectACL(oss.ACLPublicRead))
	if err := s.bucket.PutObject(key, bytes.NewReader(data), opts...); err != nil {
		return "", fmt.Errorf("oss put %s: %w", key, err)
	}
	return publicURL(s.cfg, key), nil
}
