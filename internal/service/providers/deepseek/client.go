package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	DefaultBaseURL = "https://api.deepseek.com"
	DefaultModel   = "deepseek-chat"
	APIKeyEnv      = "DEEPSEEK_API_KEY"
)

var (
	ErrNotConfigured = errors.New("deepseek api key is not configured")
	ErrEmptyPrompt   = errors.New("empty prompt")
)

// Config configures the DeepSeek OpenAI-compatible chat client.
type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

// Client calls DeepSeek chat/completions.
type Client struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// New builds a DeepSeek client. Falls back to DEEPSEEK_* / OPENAI_API_KEY env.
func New(cfg Config) *Client {
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		key = firstEnv(APIKeyEnv, "OPENAI_API_KEY")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(os.Getenv("DEEPSEEK_BASE_URL")), "/")
	}
	if base == "" {
		base = DefaultBaseURL
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("DEEPSEEK_MODEL"))
	}
	if model == "" {
		model = DefaultModel
	}
	client := cfg.HTTPClient
	if client == nil {
		// No overall Timeout so long streams are not cut mid-body.
		client = &http.Client{Timeout: 0}
	}
	return &Client{
		apiKey:     key,
		baseURL:    base,
		model:      model,
		httpClient: client,
	}
}

// Message is one chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ChatStream calls DeepSeek with stream=true and invokes onDelta for each content piece.
func (c *Client) ChatStream(ctx context.Context, messages []Message, onDelta func(delta string) error) error {
	if c == nil || c.apiKey == "" {
		return ErrNotConfigured
	}
	if len(messages) == 0 {
		return ErrEmptyPrompt
	}
	if onDelta == nil {
		return fmt.Errorf("onDelta required")
	}

	body, err := json.Marshal(chatRequest{Model: c.model, Messages: messages, Stream: true})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("deepseek http %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}

	scanner := bufio.NewScanner(resp.Body)
	// DeepSeek deltas are small; bump buffer for safety.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			return nil
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return fmt.Errorf("deepseek api: %s", chunk.Error.Message)
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content == "" {
				continue
			}
			if err := onDelta(ch.Delta.Content); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		return err
	}
	return nil
}

// Model returns the configured model id.
func (c *Client) Model() string {
	if c == nil {
		return ""
	}
	return c.model
}

// Configured reports whether an API key is available.
func (c *Client) Configured() bool {
	return c != nil && c.apiKey != ""
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
