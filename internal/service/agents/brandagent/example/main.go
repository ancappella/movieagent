package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"movieagent/internal/service/agents/brandagent"
	"movieagent/internal/service/assets"
	"movieagent/internal/service/config"
)

// Nano Banana Pro poster demo (routes Google API via Clash Verge :7897 when present).
//
//	go run ./internal/service/agents/brandagent/example [drink-image-path]
func main() {
	ensureClashProxyEnv()

	base := filepath.Join(os.TempDir(), "movieagent-nano-banana-demo")
	_ = os.RemoveAll(base)
	if err := os.MkdirAll(base, 0o755); err != nil {
		exit(err)
	}

	paths, err := config.NewPaths(&base)
	if err != nil {
		exit(err)
	}

	image, err := brandagent.MustGeminiImageGenerator()
	if err != nil {
		exit(fmt.Errorf("nano-banana-pro: %w", err))
	}

	agent, err := brandagent.New(brandagent.Options{
		Name:     "nano-banana-poster",
		UserID:   "demo",
		Paths:    paths,
		Image:    image,
		Creative: brandagent.HeuristicCreativeDesigner{},
	})
	if err != nil {
		exit(err)
	}

	drink := loadDrinkImage()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	festival := firstEnv("FESTIVAL")
	if festival == "" || festival == "(auto)" {
		festival = "国庆节"
	}
	product := firstEnv("PRODUCT")
	if product == "" || product == "(auto)" {
		product = "柠檬茶新品"
	}
	brief := firstEnv("BRIEF")
	if brief == "" || brief == "(auto)" {
		if festival == "中秋节" {
			brief = "情感关键词：团圆、温馨、清甜\n视觉元素：柠檬片、月光、桂花"
		} else {
			brief = brandagent.ExampleNationalDayLemonBrief()
		}
	}

	fmt.Println("image_model: nano-banana-pro")
	fmt.Println("festival:", festival)
	fmt.Println("product:", product)
	fmt.Println("proxy:", firstEnv("GEMINI_HTTP_PROXY", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY"))

	req := brandagent.PosterWorkflowRequest{
		ThreadID:    "demo-nbp",
		Festival:    festival,
		ProductHint: product,
		Brief:       brief,
		DrinkImage:  drink,
	}
	brandID := firstEnv("BRAND_ID")
	if brandID == "" || brandID == "(auto)" {
		brandID = "ningdao"
	}
	projectRoot, _ := filepath.Abs(".")
	assetsYAML := filepath.Join(projectRoot, "internal", "service", "models", "assets", "assets.yaml")
	if cat, err := loadBrand(assetsYAML); err == nil {
		if b, err := cat.FindBrand(brandID); err == nil {
			req.BrandID = b.ID
			req.BrandName = b.Name
			req.BrandColors = b.Colors
			req.LogoMeta = b.Visuals.Logo.Meta
			fmt.Println("brand:", b.ID, b.Name)
		}
		if path, logo, meta, err := cat.ResolveLogoPNG(projectRoot, brandID); err == nil {
			req.LogoPNG = logo
			req.LogoMIME = "image/png"
			req.LogoMeta = meta
			fmt.Println("logo:", path)
		} else {
			fmt.Println("logo: skip —", err)
		}
	}

	res, err := agent.RunPosterWorkflow(ctx, req)
	if err != nil {
		exit(err)
	}

	for _, n := range res.Nodes {
		fmt.Printf("[%s] %s (%s) model=%s %s\n", n.Status, n.Name, n.ID, n.Model, n.Detail)
	}
	fmt.Println("bytes:", res.Bytes)
	fmt.Println("output:", res.OutputPath)

	host, err := paths.SandboxOutputsDir("demo-nbp", strPtr("demo"))
	if err != nil {
		exit(err)
	}
	_ = os.MkdirAll("tmp", 0o755)
	copyOut := func(name, dest string) {
		src := filepath.Join(host, name)
		if data, err := os.ReadFile(src); err == nil {
			_ = os.WriteFile(dest, data, 0o644)
			fmt.Println("copied:", dest)
		}
	}
	copyOut("brand-campaign.png", "tmp/brand-campaign.png")
	copyOut("brand-campaign-final.png", "tmp/brand-campaign-final.png")
	if res.CompositePath != "" {
		copyOut("brand-campaign-final.png", "tmp/brand-campaign-midautumn.png")
	} else {
		copyOut("brand-campaign.png", "tmp/brand-campaign-midautumn.png")
	}
	if sb, err := agent.SandboxForThread("demo-nbp"); err == nil {
		if raw, err := sb.ReadFile(brandagent.PosterCreativePath, 0, 0); err == nil {
			_ = os.WriteFile("tmp/creative-design.md", []byte(raw), 0o644)
		}
	}
}

func loadBrand(assetsYAML string) (*assets.Catalog, error) {
	return assets.LoadCatalog(assetsYAML)
}

func ensureClashProxyEnv() {
	if os.Getenv("HTTPS_PROXY") != "" || os.Getenv("https_proxy") != "" || os.Getenv("GEMINI_HTTP_PROXY") != "" {
		return
	}
	// Prefer Clash Verge mixed-port over Cursor sandbox proxy for Google APIs.
	os.Setenv("GEMINI_HTTP_PROXY", "http://127.0.0.1:7897")
	os.Setenv("HTTPS_PROXY", "http://127.0.0.1:7897")
	os.Setenv("HTTP_PROXY", "http://127.0.0.1:7897")
	os.Setenv("ALL_PROXY", "http://127.0.0.1:7897")
	os.Setenv("NO_PROXY", "127.0.0.1,localhost")
	os.Setenv("no_proxy", "127.0.0.1,localhost")
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "(auto)"
}

func loadDrinkImage() []byte {
	if len(os.Args) > 1 {
		data, err := os.ReadFile(os.Args[1])
		if err != nil {
			exit(err)
		}
		return data
	}
	candidates := []string{
		"/Users/amguolin/.cursor/projects/Users-amguolin-movieagent/assets/image-c7e4a0b8-ba53-49d5-b30d-25e622f1a416.jpg",
		"tmp/drink-ref.jpg",
		"tmp/drink-ref.png",
	}
	for _, c := range candidates {
		if data, err := os.ReadFile(c); err == nil && len(data) > 1000 {
			fmt.Println("using drink:", c)
			return data
		}
	}
	return brandagent.ExampleDrinkPNG()
}

func strPtr(s string) *string { return &s }

func exit(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
