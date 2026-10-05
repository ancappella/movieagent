package sensenova_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"movieagent/internal/service/providers/sensenova"
)

func TestGenerateTextToImage(t *testing.T) {
	var gotPath string
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"created": 1,
			"data": []map[string]string{
				{"url": "https://example.com/a.png"},
			},
		})
	}))
	defer ts.Close()

	svc := sensenova.New(sensenova.Config{
		BaseURL:    ts.URL,
		APIKey:     "sk-test",
		HTTPClient: ts.Client(),
	})

	watermark := false
	extend := true
	out, err := svc.Generate(context.Background(), sensenova.GenerateInput{
		Prompt:         "a cat",
		Size:           "1024x1024",
		Watermark:      &watermark,
		PromptExtend:   &extend,
		ResponseFormat: "url",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if gotPath != "/images/generations" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["model"] != sensenova.ModelU15Lite {
		t.Fatalf("model = %#v", gotBody["model"])
	}
	if out.URL != "https://example.com/a.png" || out.Model != sensenova.ModelU15Lite {
		t.Fatalf("out = %#v", out)
	}
}

func TestResolveAliases(t *testing.T) {
	for _, raw := range []string{"u15-lite", "SenseNova U15 Lite", "sensenova-u1.5-lite"} {
		if got := sensenova.ResolveModel(raw); got != sensenova.ModelU15Lite {
			t.Fatalf("ResolveModel(%q) = %q", raw, got)
		}
	}
}

func TestGenerateRequiresAPIKey(t *testing.T) {
	t.Setenv(sensenova.APIKeyEnv, "")
	svc := sensenova.New(sensenova.Config{APIKey: ""})
	_, err := svc.Generate(context.Background(), sensenova.GenerateInput{Prompt: "x"})
	if err != sensenova.ErrNotConfigured {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}
