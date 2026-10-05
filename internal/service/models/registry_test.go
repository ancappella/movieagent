package models_test

import (
	"testing"

	"movieagent/internal/service/models"
)

func TestLoadDefaultRegistersLinmuModels(t *testing.T) {
	reg, err := models.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	wantIDs := []string{
		"gemini-3.5-flash",
		"gemini-2.5-flash",
		"qwen3.8-flash",
		"gpt4-o",
		"deepseek-r1",
		"deepseek-v3",
		"gemini-3-pro-image",
		"gemini-3-pro-image-preview",
		"gemini-2.5-flash-image",
		"flux-2-klein",
		"flux-2-klein-4b",
		"sensenova-u1.5-lite",
		"MiniMax-H3",
		"gemini-omni-1.1-flash",
		"gemini-omni-flash-preview",
		"veo-3.1-generate-preview",
		"veo-3.1-fast-generate-preview",
		"gemini-2.5-flash-preview-tts",
		"gemini-2.5-pro-preview-tts",
	}
	if reg.Len() != len(wantIDs) {
		t.Fatalf("registry len = %d, want %d", reg.Len(), len(wantIDs))
	}
	for _, id := range wantIDs {
		if _, err := reg.Lookup(id); err != nil {
			t.Fatalf("Lookup(%q): %v", id, err)
		}
	}
}

func TestLookupByAlias(t *testing.T) {
	reg, err := models.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	m, err := reg.Lookup("nano-banana-pro")
	if err != nil {
		t.Fatalf("alias lookup: %v", err)
	}
	if m.ID != "gemini-3-pro-image" {
		t.Fatalf("id = %q, want gemini-3-pro-image", m.ID)
	}
	if m.BaseURL == "" || m.Provider != "gemini" {
		t.Fatalf("expected provider config, got %#v", m)
	}
}

func TestListByTag(t *testing.T) {
	reg, err := models.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	images := reg.List("image")
	if len(images) != 6 {
		t.Fatalf("image models = %d, want 6", len(images))
	}

	m, err := reg.Lookup("u15-lite")
	if err != nil {
		t.Fatalf("Lookup u15-lite: %v", err)
	}
	if m.ID != "sensenova-u1.5-lite" || m.Provider != "sensenova" {
		t.Fatalf("unexpected model: %#v", m)
	}
	texts := reg.List("text")
	if len(texts) != 6 {
		t.Fatalf("text models = %d, want 6", len(texts))
	}
}
