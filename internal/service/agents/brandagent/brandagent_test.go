package brandagent_test

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"movieagent/internal/service/agents/brandagent"
	"movieagent/internal/service/config"
)

func TestRunBrandToneWritesImageToLocalSandbox(t *testing.T) {
	base := t.TempDir()
	paths, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}

	agent, err := brandagent.New(brandagent.Options{
		UserID:   "alice",
		Paths:    paths,
		Image:    brandagent.PlaceholderImage{},
		Creative: brandagent.HeuristicCreativeDesigner{},
	})
	if err != nil {
		t.Fatal(err)
	}

	drink := brandagent.ExampleDrinkPNG()
	ctx := context.Background()
	res, err := agent.RunBrandTone(ctx, brandagent.BrandToneRequest{
		ThreadID:   "t-brand-1",
		DrinkImage: drink,
		DrinkMIME:  "image/png",
		Strategy:   brandagent.ExampleCoffeeStrategy(),
	})
	if err != nil {
		t.Fatalf("RunBrandTone: %v", err)
	}
	if res.OutputPath != brandagent.BrandToneOutputPath {
		t.Fatalf("output path = %q", res.OutputPath)
	}
	if res.Bytes < 100 {
		t.Fatalf("expected png bytes, got %d", res.Bytes)
	}
	if !strings.HasPrefix(res.SandboxID, "local:alice:") {
		t.Fatalf("sandbox id = %q", res.SandboxID)
	}

	sb, err := agent.SandboxForThread("t-brand-1")
	if err != nil {
		t.Fatal(err)
	}
	strategy, err := sb.ReadFile(brandagent.BrandToneStrategyPath, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strategy, "情感关键词") || !strings.Contains(strategy, "柠檬片") {
		t.Fatalf("strategy = %q", strategy)
	}
	prompt, err := sb.ReadFile(brandagent.BrandTonePromptPath, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "节日营销海报") || !strings.Contains(prompt, "创意概念") {
		t.Fatalf("prompt missing workflow poster copy: %s", prompt)
	}
	creative, err := sb.ReadFile(brandagent.PosterCreativePath, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(creative, "概念：") {
		t.Fatalf("creative design missing: %q", creative)
	}
	if strings.Contains(prompt, "LOCKED") {
		t.Fatalf("prompt should not use LOCKED labels: %s", prompt)
	}

	hostOut, err := paths.SandboxOutputsDir("t-brand-1", strPtr("alice"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(hostOut, "brand-campaign.png"))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if img.Bounds().Dx() < 8 {
		t.Fatalf("unexpected image size: %v", img.Bounds())
	}
}

func TestRunPosterWorkflowNodes(t *testing.T) {
	base := t.TempDir()
	paths, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := brandagent.New(brandagent.Options{
		UserID:   "alice",
		Paths:    paths,
		Image:    brandagent.PlaceholderImage{},
		Creative: brandagent.HeuristicCreativeDesigner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := agent.RunPosterWorkflow(context.Background(), brandagent.PosterWorkflowRequest{
		ThreadID:    "t-poster",
		Festival:    "国庆节",
		ProductHint: "柠檬茶新品",
		Brief:       brandagent.ExampleNationalDayLemonBrief(),
		DrinkImage:  brandagent.ExampleDrinkPNG(),
		DrinkMIME:   "image/png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 6 {
		t.Fatalf("nodes = %d %+v", len(res.Nodes), res.Nodes)
	}
	for _, n := range res.Nodes {
		if n.ID == "brand_composite" {
			if n.Status != "skip" {
				t.Fatalf("brand_composite without logo should skip, got %s", n.Status)
			}
			continue
		}
		if n.Status != "ok" {
			t.Fatalf("node %s status=%s detail=%s", n.ID, n.Status, n.Detail)
		}
	}
	want := []string{"ingest", "brief", "creative_design", "prompt", "render", "brand_composite"}
	for i, id := range want {
		if res.Nodes[i].ID != id {
			t.Fatalf("node[%d]=%s want %s", i, res.Nodes[i].ID, id)
		}
	}
	if res.Creative == nil || res.Creative.Concept == "" {
		t.Fatal("expected creative concept")
	}
	if strings.Contains(res.Prompt, "主文案（自然排版") || strings.Contains(res.Prompt, "「国庆节") {
		t.Fatalf("prompt should not put on-image copy: %s", res.Prompt)
	}
	if !strings.Contains(res.Prompt, "不要出现任何文字") {
		t.Fatalf("prompt should forbid text: %s", res.Prompt)
	}
}

func TestRunBrandToneRequiresStrategyAndDrink(t *testing.T) {
	base := t.TempDir()
	paths, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := brandagent.New(brandagent.Options{UserID: "alice", Paths: paths, Image: brandagent.PlaceholderImage{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = agent.RunBrandTone(context.Background(), brandagent.BrandToneRequest{
		ThreadID:   "t2",
		DrinkImage: brandagent.ExampleDrinkPNG(),
	})
	if err == nil {
		t.Fatal("expected error for empty strategy")
	}
	_, err = agent.RunBrandTone(context.Background(), brandagent.BrandToneRequest{
		ThreadID: "t3",
		Strategy: brandagent.ExampleCoffeeStrategy(),
	})
	if err == nil {
		t.Fatal("expected error for missing drink image")
	}
}

func strPtr(s string) *string { return &s }
