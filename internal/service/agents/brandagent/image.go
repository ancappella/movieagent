package brandagent

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"movieagent/internal/service/providers/gemini"
	"movieagent/internal/service/providers/sensenova"
	"movieagent/internal/service/sandbox"
)

// GenerateRequest is one image generation / edit call.
type GenerateRequest struct {
	Prompt   string
	RefImage []byte // optional primary reference (drink or poster)
	RefMIME  string
	// ExtraRefs are additional images (e.g. brand logo) for multimodal edit.
	ExtraRefs []ImageRef
}

// ImageRef is an extra multimodal image part.
type ImageRef struct {
	Data []byte
	MIME string
}

// ImageGenerator produces image bytes from a prompt and optional reference image.
type ImageGenerator interface {
	Generate(ctx context.Context, req GenerateRequest) ([]byte, error)
}

// ImageGeneratorFunc adapts a function to ImageGenerator.
type ImageGeneratorFunc func(ctx context.Context, req GenerateRequest) ([]byte, error)

func (f ImageGeneratorFunc) Generate(ctx context.Context, req GenerateRequest) ([]byte, error) {
	return f(ctx, req)
}

// PlaceholderImage returns a small valid PNG (tests / offline).
type PlaceholderImage struct{}

func (PlaceholderImage) Generate(_ context.Context, req GenerateRequest) ([]byte, error) {
	if len(req.RefImage) > 0 {
		if out, err := normalizeToPNG(req.RefImage, req.RefMIME); err == nil && len(out) > 0 {
			return out, nil
		}
	}
	sum := uint32(0)
	for i := 0; i < len(req.Prompt); i++ {
		sum = sum*33 + uint32(req.Prompt[i])
	}
	bg := color.RGBA{
		R: uint8(0x1A + (sum & 0x1F)),
		G: uint8(0x12 + ((sum >> 4) & 0x1F)),
		B: uint8(0x0C + ((sum >> 8) & 0x1F)),
		A: 255,
	}
	accent := color.RGBA{R: 0xC4, G: 0xA5, B: 0x74, A: 255}
	const w, h = 512, 768
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, bg)
		}
	}
	for y := h/2 - 40; y < h/2+40; y++ {
		for x := w/2 - 28; x < w/2+28; x++ {
			img.Set(x, y, accent)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// NanoBananaProImage calls Gemini 3 Pro Image (Nano Banana Pro).
type NanoBananaProImage struct {
	Service     *gemini.ImageService
	AspectRatio string
	ImageSize   string
}

func NewNanoBananaProFromEnv() (*NanoBananaProImage, error) {
	svc := gemini.NewImageService(gemini.Config{
		Model: gemini.ModelNanoBananaPro,
	})
	return &NanoBananaProImage{
		Service:     svc,
		AspectRatio: gemini.DefaultAspect,
		ImageSize:   gemini.DefaultSize,
	}, nil
}

func (s *NanoBananaProImage) Generate(ctx context.Context, req GenerateRequest) ([]byte, error) {
	if s == nil || s.Service == nil {
		return nil, gemini.ErrNotConfigured
	}
	aspect := s.AspectRatio
	if aspect == "" {
		aspect = gemini.DefaultAspect
	}
	size := s.ImageSize
	if size == "" {
		size = gemini.DefaultSize
	}
	var extra []gemini.ImageRef
	for _, r := range req.ExtraRefs {
		extra = append(extra, gemini.ImageRef{Data: r.Data, MIME: r.MIME})
	}
	out, err := s.Service.Generate(ctx, gemini.GenerateInput{
		Prompt:      req.Prompt,
		AspectRatio: aspect,
		ImageSize:   size,
		RefImage:    req.RefImage,
		RefMIME:     req.RefMIME,
		ExtraRefs:   extra,
	})
	if err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, gemini.ErrEmptyImage
	}
	return normalizeToPNG(out.Data, out.MIME)
}

// SenseNovaImage wraps SenseNova U1.5 Lite (fallback).
type SenseNovaImage struct {
	Service *sensenova.Service
	Size    string
}

func NewSenseNovaImageFromEnv() (*SenseNovaImage, error) {
	svc := sensenova.New(sensenova.Config{})
	if svc == nil {
		return nil, sensenova.ErrNotConfigured
	}
	return &SenseNovaImage{Service: svc, Size: sensenova.DefaultSize}, nil
}

func (s *SenseNovaImage) Generate(ctx context.Context, req GenerateRequest) ([]byte, error) {
	if s == nil || s.Service == nil {
		return nil, sensenova.ErrNotConfigured
	}
	size := s.Size
	if size == "" {
		size = sensenova.DefaultSize
	}
	noWatermark := false
	extend := true
	var (
		out *sensenova.GenerateOutput
		err error
	)
	if len(req.RefImage) > 0 {
		mime := req.RefMIME
		if mime == "" {
			mime = sniffMIME(req.RefImage)
		}
		dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(req.RefImage)
		out, err = s.Service.Edit(ctx, sensenova.EditInput{
			Prompt:         req.Prompt,
			ImageURL:       dataURL,
			Size:           size,
			ResponseFormat: "b64_json",
			OutputFormat:   "png",
			Watermark:      &noWatermark,
		})
	} else {
		out, err = s.Service.Generate(ctx, sensenova.GenerateInput{
			Prompt:         req.Prompt,
			Size:           size,
			ResponseFormat: "b64_json",
			OutputFormat:   "png",
			Watermark:      &noWatermark,
			PromptExtend:   &extend,
		})
	}
	if err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, sensenova.ErrEmptyImage
	}
	cleaned, err := stripCornerWatermark(out.Data)
	if err != nil {
		return out.Data, nil
	}
	return cleaned, nil
}

// DefaultImageGenerator prefers Nano Banana Pro, then SenseNova, then placeholder.
func DefaultImageGenerator() ImageGenerator {
	var chain []ImageGenerator
	if os.Getenv(gemini.APIKeyEnv) != "" {
		if img, err := NewNanoBananaProFromEnv(); err == nil {
			chain = append(chain, img)
		}
	}
	if os.Getenv(sensenova.APIKeyEnv) != "" {
		if img, err := NewSenseNovaImageFromEnv(); err == nil {
			chain = append(chain, img)
		}
	}
	if len(chain) == 0 {
		return PlaceholderImage{}
	}
	if len(chain) == 1 {
		return chain[0]
	}
	return FallbackImage{Generators: chain}
}

// MustGeminiImageGenerator returns Nano Banana Pro only (no SenseNova fallback).
func MustGeminiImageGenerator() (ImageGenerator, error) {
	if os.Getenv(gemini.APIKeyEnv) == "" {
		return nil, gemini.ErrNotConfigured
	}
	return NewNanoBananaProFromEnv()
}

// FallbackImage tries generators in order until one succeeds.
type FallbackImage struct {
	Generators []ImageGenerator
}

func (f FallbackImage) Generate(ctx context.Context, req GenerateRequest) ([]byte, error) {
	var last error
	for _, g := range f.Generators {
		if g == nil {
			continue
		}
		out, err := g.Generate(ctx, req)
		if err == nil && len(out) > 0 {
			return out, nil
		}
		last = err
	}
	if last != nil {
		return nil, last
	}
	return nil, fmt.Errorf("no image generator available")
}

func sniffMIME(data []byte) string {
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

func normalizeToPNG(data []byte, mime string) ([]byte, error) {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}) {
		return data, nil
	}
	if strings.Contains(mime, "jpeg") || strings.Contains(mime, "jpg") || bytes.HasPrefix(data, []byte{0xff, 0xd8}) {
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return data, nil
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	return data, nil
}

func writeSandboxBytes(sb sandbox.Sandbox, virtualPath string, data []byte) error {
	if sb == nil {
		return fmt.Errorf("sandbox is nil")
	}
	ls, ok := sb.(*sandbox.LocalSandbox)
	if !ok {
		return fmt.Errorf("writeSandboxBytes requires *sandbox.LocalSandbox")
	}
	host, err := resolveHostPath(ls, virtualPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(host), 0o755); err != nil {
		return err
	}
	return os.WriteFile(host, data, 0o644)
}

func readSandboxBytes(sb sandbox.Sandbox, virtualPath string) ([]byte, error) {
	ls, ok := sb.(*sandbox.LocalSandbox)
	if !ok {
		return nil, fmt.Errorf("readSandboxBytes requires *sandbox.LocalSandbox")
	}
	host, err := resolveHostPath(ls, virtualPath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(host)
}

func resolveHostPath(ls *sandbox.LocalSandbox, virtual string) (string, error) {
	virtual = filepath.ToSlash(virtual)
	var best sandbox.PathMapping
	bestLen := -1
	for _, m := range ls.Mappings() {
		prefix := strings.TrimRight(m.ContainerPath, "/")
		if virtual != prefix && !strings.HasPrefix(virtual, prefix+"/") {
			continue
		}
		if len(prefix) <= bestLen {
			continue
		}
		best = m
		bestLen = len(prefix)
	}
	if bestLen < 0 {
		return "", fmt.Errorf("no mapping for %s", virtual)
	}
	prefix := strings.TrimRight(best.ContainerPath, "/")
	rel := strings.TrimPrefix(virtual, prefix)
	rel = strings.TrimPrefix(rel, "/")
	return filepath.Join(best.LocalPath, filepath.FromSlash(rel)), nil
}

func stripCornerWatermark(data []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 64 || h < 64 {
		return data, nil
	}
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, src.At(x, y))
		}
	}
	x0 := b.Min.X + int(float64(w)*0.72)
	y0 := b.Min.Y + int(float64(h)*0.90)
	sampleY := y0 - max(8, h/40)
	if sampleY < b.Min.Y {
		sampleY = b.Min.Y
	}
	fill := averageRGBA(src, x0, b.Max.X, sampleY, sampleY+max(4, h/80))
	for y := y0; y < b.Max.Y; y++ {
		for x := x0; x < b.Max.X; x++ {
			out.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func averageRGBA(img image.Image, x0, x1, y0, y1 int) color.RGBA {
	b := img.Bounds()
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	var r, g, bl, a, n uint64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			rr, gg, bb, aa := img.At(x, y).RGBA()
			r += uint64(rr >> 8)
			g += uint64(gg >> 8)
			bl += uint64(bb >> 8)
			a += uint64(aa >> 8)
			n++
		}
	}
	if n == 0 {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	return color.RGBA{
		R: uint8(r / n),
		G: uint8(g / n),
		B: uint8(bl / n),
		A: uint8(a / n),
	}
}
