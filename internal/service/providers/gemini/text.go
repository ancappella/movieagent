package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// ModelFlash is the default creative / brief text model.
	ModelFlash = "gemini-2.5-flash"
	// ModelFlashLatest prefers catalog default when available.
	ModelFlashLatest = "gemini-3.5-flash"
)

// TextService calls Gemini generateContent for text (optionally multimodal).
type TextService struct {
	model      string
	baseURL    string
	apiKey     string
	httpClient *http.Client
	retries    int
}

// NewTextService builds a text client. Defaults to gemini-2.5-flash + GEMINI_API_KEY.
// Honors GEMINI_MODEL / GEMINI_BASE_URL when cfg fields are empty.
func NewTextService(cfg Config) *TextService {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("GEMINI_MODEL"))
	}
	if model == "" {
		model = ModelFlash
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(os.Getenv("GEMINI_BASE_URL")), "/")
	}
	if base == "" {
		base = DefaultBaseURL
	}
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		key = os.Getenv(APIKeyEnv)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = NewHTTPClient(120 * time.Second)
	}
	return &TextService{
		model:      model,
		baseURL:    base,
		apiKey:     key,
		httpClient: client,
		retries:    3,
	}
}

// CompleteInput is a text (and optional image) completion request.
type CompleteInput struct {
	Prompt   string
	RefImage []byte
	RefMIME  string
	Model    string
}

// Complete returns plain text from the model.
func (s *TextService) Complete(ctx context.Context, prompt string) (string, error) {
	return s.CompleteWith(ctx, CompleteInput{Prompt: prompt})
}

// CompleteWith supports multimodal prompts (product photo + text).
func (s *TextService) CompleteWith(ctx context.Context, in CompleteInput) (string, error) {
	if s == nil || s.apiKey == "" {
		return "", ErrNotConfigured
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return "", ErrEmptyPrompt
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = s.model
	}

	parts := []part{}
	if len(in.RefImage) > 0 {
		mime := strings.TrimSpace(in.RefMIME)
		if mime == "" {
			mime = detectImageMIME(in.RefImage)
		}
		parts = append(parts, part{
			InlineData: &inlineData{
				MIMEType: mime,
				Data:     base64.StdEncoding.EncodeToString(in.RefImage),
			},
		})
	}
	parts = append(parts, part{Text: prompt})

	body := generateRequest{
		Contents: []content{{Parts: parts}},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", s.baseURL, model, s.apiKey)
	retries := s.retries
	if retries < 1 {
		retries = 1
	}
	var last error
	for attempt := 1; attempt <= retries; attempt++ {
		text, err := s.doComplete(ctx, url, raw)
		if err == nil {
			return text, nil
		}
		last = err
		if attempt == retries || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	return "", fmt.Errorf("gemini text after %d attempts: %w", retries, last)
}

func (s *TextService) doComplete(ctx context.Context, url string, raw []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("gemini http %d: %s", resp.StatusCode, truncate(string(respBody), 400))
	}

	var out generateResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("gemini api: %s", out.Error.Message)
	}
	var b strings.Builder
	for _, c := range out.Candidates {
		for _, p := range c.Content.Parts {
			if t := strings.TrimSpace(p.Text); t != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(t)
			}
		}
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return "", fmt.Errorf("gemini returned empty text")
	}
	return text, nil
}

// Model returns the configured text model id.
func (s *TextService) Model() string {
	if s == nil {
		return ""
	}
	return s.model
}
