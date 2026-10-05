// Package sensenova calls SenseTime SenseNova Token Plan APIs.
//
// SenseNova U1.5 Lite is an image model (text-to-image / image-edit), not chat.
// Model ID: sensenova-u1.5-lite
// Base URL: https://token.sensenova.cn/v1
// Auth:     Authorization: Bearer <SENSENOVA_API_KEY>
package sensenova

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	ModelU15Lite   = "sensenova-u1.5-lite"
	DefaultModel   = ModelU15Lite
	DefaultBaseURL = "https://token.sensenova.cn/v1"
	DefaultSize    = "1024x1024"

	generationPath = "/images/generations"
	editPath       = "/images/edits"

	APIKeyEnv = "SENSENOVA_API_KEY"
)

var (
	ErrNotConfigured = errors.New("sensenova api key is not configured")
	ErrEmptyPrompt   = errors.New("prompt is required")
	ErrEmptyImage    = errors.New("sensenova returned no image")
	ErrUnauthorized  = errors.New("sensenova api key is invalid or expired")
)

type Config struct {
	Model      string
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

type Service struct {
	model      string
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type GenerateInput struct {
	Prompt       string
	Model        string
	Size         string
	N            int
	Watermark    *bool
	PromptExtend *bool
	// ResponseFormat: "url" or "b64_json"
	ResponseFormat string
	OutputFormat   string
}

type EditInput struct {
	Prompt         string
	Model          string
	ImageURL       string
	Size           string
	Watermark      *bool
	ResponseFormat string
	OutputFormat   string
}

type GenerateOutput struct {
	Model string
	MIME  string
	Data  []byte
	URL   string
}

type generateRequest struct {
	Model          string         `json:"model"`
	Prompt         string         `json:"prompt"`
	Size           string         `json:"size,omitempty"`
	N              int            `json:"n,omitempty"`
	Watermark      *bool          `json:"watermark"`
	PromptExtend   *bool          `json:"prompt_extend,omitempty"`
	ResponseFormat string         `json:"response_format,omitempty"`
	OutputFormat   string         `json:"output_format,omitempty"`
	Image          string         `json:"image,omitempty"`
	Images         []editImageRef `json:"images,omitempty"`
}

type editImageRef struct {
	ImageURL string `json:"image_url"`
}

type generateResponse struct {
	Created int            `json:"created"`
	Data    []generateItem `json:"data"`
	Error   *apiError      `json:"error"`
}

type generateItem struct {
	URL     string `json:"url"`
	B64JSON string `json:"b64_json"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

func New(cfg Config) *Service {
	model := ResolveModel(cfg.Model)
	if model == "" {
		model = DefaultModel
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		key = os.Getenv(APIKeyEnv)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &Service{
		model:      model,
		baseURL:    base,
		apiKey:     key,
		httpClient: client,
	}
}

func ResolveModel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", ModelU15Lite, "u15-lite", "u1.5-lite", "sensenova-u15-lite",
		"sensenova u15 lite", "sensenova u1.5 lite":
		return ModelU15Lite
	default:
		return strings.TrimSpace(raw)
	}
}

func IsModel(id string) bool {
	return ResolveModel(id) == ModelU15Lite ||
		strings.EqualFold(strings.TrimSpace(id), ModelU15Lite)
}

// Generate runs text-to-image via POST /images/generations.
func (s *Service) Generate(ctx context.Context, in GenerateInput) (*GenerateOutput, error) {
	if s.apiKey == "" {
		return nil, ErrNotConfigured
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}

	model := ResolveModel(in.Model)
	if model == "" {
		model = s.model
	}
	size := strings.TrimSpace(in.Size)
	if size == "" {
		size = DefaultSize
	}
	n := in.N
	if n <= 0 {
		n = 1
	}
	format := strings.TrimSpace(in.ResponseFormat)
	if format == "" {
		format = "url"
	}
	watermark := in.Watermark
	if watermark == nil {
		off := false
		watermark = &off
	}

	reqBody := generateRequest{
		Model:          model,
		Prompt:         prompt,
		Size:           size,
		N:              n,
		Watermark:      watermark,
		PromptExtend:   in.PromptExtend,
		ResponseFormat: format,
		OutputFormat:   in.OutputFormat,
	}
	return s.doImage(ctx, s.baseURL+generationPath, reqBody, model)
}

// Edit runs image editing via POST /images/edits.
func (s *Service) Edit(ctx context.Context, in EditInput) (*GenerateOutput, error) {
	if s.apiKey == "" {
		return nil, ErrNotConfigured
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}
	imageURL := strings.TrimSpace(in.ImageURL)
	if imageURL == "" {
		return nil, fmt.Errorf("image url is required")
	}

	model := ResolveModel(in.Model)
	if model == "" {
		model = s.model
	}
	format := strings.TrimSpace(in.ResponseFormat)
	if format == "" {
		format = "url"
	}

	reqBody := generateRequest{
		Model:          model,
		Prompt:         prompt,
		Images:         []editImageRef{{ImageURL: imageURL}},
		Size:           in.Size,
		Watermark:      in.Watermark,
		ResponseFormat: format,
		OutputFormat:   in.OutputFormat,
	}
	return s.doImage(ctx, s.baseURL+editPath, reqBody, model)
}

func (s *Service) doImage(ctx context.Context, url string, body generateRequest, model string) (*GenerateOutput, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: %s", ErrUnauthorized, truncate(string(respBody), 200))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sensenova http %d: %s", resp.StatusCode, truncate(string(respBody), 400))
	}

	var out generateResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return nil, fmt.Errorf("sensenova api: %s", out.Error.Message)
	}
	if len(out.Data) == 0 {
		return nil, ErrEmptyImage
	}

	item := out.Data[0]
	result := &GenerateOutput{Model: model, URL: item.URL}
	if item.B64JSON != "" {
		data, err := base64.StdEncoding.DecodeString(item.B64JSON)
		if err != nil {
			return nil, fmt.Errorf("decode b64_json: %w", err)
		}
		result.Data = data
		result.MIME = "image/png"
	}
	if result.URL == "" && len(result.Data) == 0 {
		return nil, ErrEmptyImage
	}
	return result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
