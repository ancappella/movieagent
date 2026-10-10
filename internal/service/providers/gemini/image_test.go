package gemini_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"movieagent/internal/service/providers/gemini"
)

func TestGenerateNanoBananaPro(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	var gotPath string
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{
								"inlineData": map[string]string{
									"mimeType": "image/png",
									"data":     base64.StdEncoding.EncodeToString(png),
								},
							},
						},
					},
				},
			},
		})
	}))
	defer ts.Close()

	svc := gemini.NewImageService(gemini.Config{
		BaseURL:    ts.URL,
		APIKey:     "test-key",
		HTTPClient: ts.Client(),
		Model:      "nano-banana-pro",
	})
	out, err := svc.Generate(context.Background(), gemini.GenerateInput{
		Prompt: "iced americano poster",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(gotPath, "/models/"+gemini.ModelNanoBananaPro+":generateContent") {
		t.Fatalf("path = %q", gotPath)
	}
	cfg := gotBody["generationConfig"].(map[string]any)
	mods := cfg["responseModalities"].([]any)
	if len(mods) == 0 || mods[0] != "TEXT" {
		t.Fatalf("modalities = %#v", mods)
	}
	if string(out.Data) != string(png) {
		t.Fatalf("unexpected image bytes")
	}
	if out.Model != gemini.ModelNanoBananaPro {
		t.Fatalf("model = %q", out.Model)
	}
}

func TestResolveImageModel(t *testing.T) {
	for _, raw := range []string{"nano-banana", "nano-banana-pro", "Nano Banana Pro", "gemini-3-pro-image-preview"} {
		if got := gemini.ResolveImageModel(raw); got != gemini.ModelNanoBananaPro {
			t.Fatalf("ResolveImageModel(%q) = %q", raw, got)
		}
	}
	if got := gemini.ResolveImageModel("gemini-2.5-flash-image"); got != "gemini-2.5-flash-image" {
		t.Fatalf("flash image model = %q", got)
	}
}

func TestGenerateWithReferenceImage(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]any{{
						"inlineData": map[string]string{
							"mimeType": "image/png",
							"data":     base64.StdEncoding.EncodeToString(png),
						},
					}},
				},
			}},
		})
	}))
	defer ts.Close()

	svc := gemini.NewImageService(gemini.Config{BaseURL: ts.URL, APIKey: "k", HTTPClient: ts.Client()})
	ref := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}
	_, err := svc.Generate(context.Background(), gemini.GenerateInput{
		Prompt:   "poster",
		RefImage: ref,
		RefMIME:  "image/png",
	})
	if err != nil {
		t.Fatal(err)
	}
	contents := gotBody["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	if len(parts) < 2 {
		t.Fatalf("expected image+text parts, got %#v", parts)
	}
	if _, ok := parts[0].(map[string]any)["inlineData"]; !ok {
		t.Fatalf("first part should be inlineData: %#v", parts[0])
	}
}
