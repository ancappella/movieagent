// Package gemini calls Google Gemini generateContent APIs used by movieagent.
package gemini

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
	// ModelNanoBananaPro is Gemini 3 Pro Image (Nano Banana Pro).
	// Prefer GA id; preview alias still accepted via ResolveImageModel.
	ModelNanoBananaPro        = "gemini-3-pro-image"
	ModelNanoBananaProPreview = "gemini-3-pro-image-preview"

	DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	APIKeyEnv      = "GEMINI_API_KEY"
	DefaultAspect  = "3:4"
	DefaultSize    = "2K"
)

var (
	ErrNotConfigured = errors.New("gemini api key is not configured")
	ErrEmptyPrompt   = errors.New("prompt is required")
	ErrEmptyImage    = errors.New("gemini returned no image")
)

type Config struct {
	Model       string
	BaseURL     string
	APIKey      string
	HTTPClient  *http.Client
	AspectRatio string
	ImageSize   string
}

type ImageService struct {
	model       string
	baseURL     string
	apiKey      string
	httpClient  *http.Client
	aspectRatio string
	imageSize   string
}

type GenerateInput struct {
	Prompt      string
	Model       string
	AspectRatio string
	ImageSize   string
	// Optional reference image for image+text editing (Nano Banana).
	RefImage []byte
	RefMIME  string
	// ExtraRefs are additional images (e.g. brand logo) sent after RefImage.
	ExtraRefs []ImageRef
}

// ImageRef is one inline image part for multimodal generateContent.
type ImageRef struct {
	Data []byte
	MIME string
}

type GenerateOutput struct {
	Model string
	MIME  string
	Data  []byte
}

type generateRequest struct {
	Contents         []content         `json:"contents"`
	GenerationConfig *generationConfig `json:"generationConfig,omitempty"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inlineData,omitempty"`
}

type inlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type generationConfig struct {
	ResponseModalities []string     `json:"responseModalities"`
	ImageConfig        *imageConfig `json:"imageConfig,omitempty"`
}

type imageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ImageSize   string `json:"imageSize,omitempty"`
}

type generateResponse struct {
	Candidates []candidate `json:"candidates"`
	Error      *apiError   `json:"error"`
}

type candidate struct {
	Content content `json:"content"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

func NewImageService(cfg Config) *ImageService {
	model := ResolveImageModel(cfg.Model)
	if model == "" {
		model = ModelNanoBananaPro
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
		client = NewHTTPClient(180 * time.Second)
	}
	aspect := strings.TrimSpace(cfg.AspectRatio)
	if aspect == "" {
		aspect = DefaultAspect
	}
	size := strings.TrimSpace(cfg.ImageSize)
	if size == "" {
		size = DefaultSize
	}
	return &ImageService{
		model:       model,
		baseURL:     base,
		apiKey:      key,
		httpClient:  client,
		aspectRatio: aspect,
		imageSize:   size,
	}
}

func ResolveImageModel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "nano-banana", "nano banana", "nanobanana",
		"nano-banana-pro", "nano banana pro", "nanobanana-pro",
		"gemini-3-pro-image", "gemini-3-pro-image-preview", "gemini 3.0 pro image preview":
		return ModelNanoBananaPro
	case "nano-banana-flash", "gemini-2.5-flash-image", "gemini-2.5-flash-image-preview":
		return "gemini-2.5-flash-image"
	default:
		return strings.TrimSpace(raw)
	}
}

func IsNanoBananaPro(id string) bool {
	return ResolveImageModel(id) == ModelNanoBananaPro
}

// Generate runs text-to-image via models/{id}:generateContent.
func (s *ImageService) Generate(ctx context.Context, in GenerateInput) (*GenerateOutput, error) {
	if s.apiKey == "" {
		return nil, ErrNotConfigured
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}
	model := ResolveImageModel(in.Model)
	if model == "" {
		model = s.model
	}
	aspect := strings.TrimSpace(in.AspectRatio)
	if aspect == "" {
		aspect = s.aspectRatio
	}
	size := strings.TrimSpace(in.ImageSize)
	if size == "" {
		size = s.imageSize
	}

	parts := []part{}
	appendImage := func(data []byte, mime string) {
		if len(data) == 0 {
			return
		}
		mime = strings.TrimSpace(mime)
		if mime == "" {
			mime = detectImageMIME(data)
		}
		parts = append(parts, part{
			InlineData: &inlineData{
				MIMEType: mime,
				Data:     base64.StdEncoding.EncodeToString(data),
			},
		})
	}
	appendImage(in.RefImage, in.RefMIME)
	for _, ref := range in.ExtraRefs {
		appendImage(ref.Data, ref.MIME)
	}
	parts = append(parts, part{Text: prompt})

	body := generateRequest{
		Contents: []content{{
			Parts: parts,
		}},
		GenerationConfig: &generationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
			ImageConfig: &imageConfig{
				AspectRatio: aspect,
				ImageSize:   size,
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", s.baseURL, model, s.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini http %d: %s", resp.StatusCode, truncate(string(respBody), 400))
	}

	var out generateResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return nil, fmt.Errorf("gemini api: %s", out.Error.Message)
	}
	for _, c := range out.Candidates {
		for _, p := range c.Content.Parts {
			if p.InlineData == nil || p.InlineData.Data == "" {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(p.InlineData.Data)
			if err != nil {
				return nil, fmt.Errorf("decode inline image: %w", err)
			}
			mime := p.InlineData.MIMEType
			if mime == "" {
				mime = "image/png"
			}
			return &GenerateOutput{Model: model, MIME: mime, Data: data}, nil
		}
	}
	return nil, ErrEmptyImage
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func detectImageMIME(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}):
		return "image/png"
	case bytes.HasPrefix(data, []byte{0xff, 0xd8}):
		return "image/jpeg"
	case len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	default:
		return "image/png"
	}
}
