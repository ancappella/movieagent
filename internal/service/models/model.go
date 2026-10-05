package models

import (
	"errors"
	"strings"
)

type Tag string

const (
	TagText  Tag = "text"
	TagImage Tag = "image"
	TagVideo Tag = "video"
	TagAudio Tag = "audio"
)

var (
	ErrNotFound   = errors.New("model not found")
	ErrEmptyID    = errors.New("model id is required")
	ErrInvalidID  = errors.New("invalid model id")
	ErrInvalidTag = errors.New("tag must be text, image, video, or audio")
	ErrDuplicate  = errors.New("duplicate model id")
)

// Model is a registered capability identified by a stable ID.
type Model struct {
	ID        string   `json:"id" yaml:"id"`
	Tag       Tag      `json:"tag" yaml:"tag"`
	Name      string   `json:"name" yaml:"name"`
	Provider  string   `json:"provider" yaml:"provider"`
	Upstream  string   `json:"upstream,omitempty" yaml:"upstream,omitempty"`
	BaseURL   string   `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	APIKeyEnv string   `json:"api_key_env,omitempty" yaml:"api_key_env,omitempty"`
	Aliases   []string `json:"aliases,omitempty" yaml:"aliases,omitempty"`
}

// FileConfig is the on-disk YAML catalog.
type FileConfig struct {
	Defaults map[string]string `yaml:"defaults"`
	Models   []Model           `yaml:"models"`
}

func Label(tag Tag) string {
	switch tag {
	case TagText:
		return "文本"
	case TagImage:
		return "图片"
	case TagVideo:
		return "视频"
	case TagAudio:
		return "语音"
	default:
		return string(tag)
	}
}

func ParseTag(raw string) (Tag, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "text", "llm", "chat", "文本":
		return TagText, nil
	case "image", "图片":
		return TagImage, nil
	case "video", "视频":
		return TagVideo, nil
	case "audio", "speech", "tts", "语音":
		return TagAudio, nil
	default:
		return "", ErrInvalidTag
	}
}

func validID(id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrEmptyID
	}
	if strings.ContainsAny(id, `/\:`) {
		return ErrInvalidID
	}
	return nil
}
