package handler

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"movieagent/internal/service/providers/gemini"
	"movieagent/pkg/response"

	"github.com/gin-gonic/gin"
)

// ImageHandler exposes Nano Banana text-to-image / image-edit.
type ImageHandler struct {
	svc    *gemini.ImageService
	outDir string
}

func NewImageHandler(projectRoot string) *ImageHandler {
	out := filepath.Join(projectRoot, "tmp", "generated")
	_ = os.MkdirAll(out, 0o755)
	return &ImageHandler{
		svc:    gemini.NewImageService(gemini.Config{Model: gemini.ModelNanoBananaPro}),
		outDir: out,
	}
}

type generateImageBody struct {
	Prompt      string `json:"prompt"`
	Model       string `json:"model"`
	AspectRatio string `json:"aspect_ratio"`
	ImageSize   string `json:"image_size"`
	// Optional reference image as data URL or raw base64.
	RefImage string `json:"ref_image"`
}

// Generate runs Nano Banana Pro (Gemini 3 Pro Image).
// POST /api/v1/images/generate
func (h *ImageHandler) Generate(c *gin.Context) {
	var body generateImageBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, http.StatusBadRequest, 1, "invalid json")
		return
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		response.Fail(c, http.StatusBadRequest, 1, "prompt is required")
		return
	}

	in := gemini.GenerateInput{
		Prompt:      prompt,
		Model:       body.Model,
		AspectRatio: body.AspectRatio,
		ImageSize:   body.ImageSize,
	}
	if raw, mime, err := decodeMaybeDataURL(body.RefImage); err == nil && len(raw) > 0 {
		in.RefImage = raw
		in.RefMIME = mime
	}

	ctx := c.Request.Context()
	out, err := h.svc.Generate(ctx, in)
	if err != nil {
		code := http.StatusBadGateway
		if err == gemini.ErrNotConfigured {
			code = http.StatusServiceUnavailable
		}
		response.Fail(c, code, 1, err.Error())
		return
	}

	name := time.Now().Format("20060102-150405") + ".png"
	path := filepath.Join(h.outDir, name)
	if err := os.WriteFile(path, out.Data, 0o644); err != nil {
		response.Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}

	response.OK(c, gin.H{
		"model":     out.Model,
		"mime":      out.MIME,
		"bytes":     len(out.Data),
		"path":      path,
		"url":       "/media/generated/" + name,
		"image_b64": base64.StdEncoding.EncodeToString(out.Data),
	})
}

func decodeMaybeDataURL(s string) ([]byte, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, "", gemini.ErrEmptyImage
	}
	mime := "image/png"
	if strings.HasPrefix(s, "data:") {
		head, b64, ok := strings.Cut(s, ",")
		if !ok {
			return nil, "", gemini.ErrEmptyImage
		}
		if i := strings.Index(head, ":"); i >= 0 {
			if j := strings.Index(head, ";"); j > i {
				mime = head[i+1 : j]
			}
		}
		s = b64
	}
	b, err := base64.StdEncoding.DecodeString(s)
	return b, mime, err
}
