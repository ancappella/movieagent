package model_test

import (
	"os"
	"path/filepath"
	"testing"

	"movieagent/internal/model"
)

func TestLoadDefaultFromYAML(t *testing.T) {
	reg, err := model.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	card, err := reg.Lookup("brand-agent")
	if err != nil {
		t.Fatal(err)
	}
	if card.Name != "品牌 Agent" {
		t.Fatalf("name = %q", card.Name)
	}
	if card.Cover == nil || card.Cover.URL == "" {
		t.Fatal("expected cover from yaml")
	}
	if len(card.Skills) < 2 {
		t.Fatalf("skills = %#v", card.Skills)
	}
	if card.Extra["skill"] != "poster-workflow" {
		t.Fatalf("extra = %#v", card.Extra)
	}
}

func TestDefaultRegistryLoadedFromYAML(t *testing.T) {
	if err := model.EnsureDefaultLoaded(); err != nil {
		t.Fatal(err)
	}
	card, err := model.Default.Lookup("brand-agent")
	if err != nil {
		t.Fatal(err)
	}
	if card.Package == "" {
		t.Fatal("package empty")
	}
	if card.IconURL == "" {
		t.Fatal("icon_url empty")
	}
}

func TestLookupByAlias(t *testing.T) {
	card, err := model.Default.Lookup("brandagent")
	if err != nil {
		t.Fatal(err)
	}
	if card.ID != model.BrandAgentCardID {
		t.Fatalf("id = %q", card.ID)
	}
}

func TestLoadFileCustomYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cards.yaml")
	body := []byte(`
version: 1
agents:
  - id: demo-agent
    name: Demo
    description: test card
    mode: ask
    tags: [document]
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := model.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	card, err := reg.Lookup("demo-agent")
	if err != nil {
		t.Fatal(err)
	}
	if card.Mode != model.ModeAsk {
		t.Fatalf("mode = %q", card.Mode)
	}
}

func TestBrandAgentCardHelper(t *testing.T) {
	card := model.BrandAgentCard()
	if card.ID != model.BrandAgentCardID {
		t.Fatalf("id = %q", card.ID)
	}
	if card.Cover == nil {
		t.Fatal("expected cover")
	}
}
