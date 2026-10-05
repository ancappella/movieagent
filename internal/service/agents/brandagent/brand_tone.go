package brandagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"movieagent/internal/service/sandbox"
)

const (
	BrandToneDrinkPath         = "/mnt/user-data/uploads/images/drink-ref.png"
	BrandToneFestivalBriefPath = "/mnt/user-data/workspace/festival-brief.md"
	BrandToneStrategyPath      = "/mnt/user-data/workspace/strategy.md"
	BrandTonePromptPath        = "/mnt/user-data/workspace/brand-prompt.md"
	BrandToneOutputPath        = "/mnt/user-data/outputs/brand-campaign.png"

	// Deprecated aliases kept so older callers/tests compile during transition.
	BrandToneMetaPath     = "/mnt/user-data/workspace/brand-meta.json"
	BrandToneCampaignPath = BrandToneStrategyPath
)

// BrandToneRequest: beverage image + marketing strategy → poster.
type BrandToneRequest struct {
	ThreadID string

	// Drink image: provide bytes and/or a host/virtual path.
	DrinkImage     []byte
	DrinkImagePath string // host path or sandbox virtual path
	DrinkMIME      string

	// Marketing strategy text (required).
	Strategy string

	OutputPath string // optional; default BrandToneOutputPath
}

// BrandToneResult is the poster pipeline output (direct LLM, no skill).
type BrandToneResult struct {
	SandboxID    string `json:"sandbox_id"`
	DrinkPath    string `json:"drink_path"`
	StrategyPath string `json:"strategy_path"`
	PromptPath   string `json:"prompt_path"`
	OutputPath   string `json:"output_path"`
	Prompt       string `json:"prompt"`
	Bytes        int    `json:"bytes"`
}

// RunBrandTone runs the poster workflow directly against the LLM / image model
// (brief → creative_design → prompt → render). Does not load or follow any skill file.
func (a *BrandAgent) RunBrandTone(ctx context.Context, req BrandToneRequest) (*BrandToneResult, error) {
	if a == nil {
		return nil, fmt.Errorf("brandagent: nil agent")
	}
	if strings.TrimSpace(req.Strategy) == "" {
		return nil, fmt.Errorf("marketing strategy is required")
	}
	threadID := req.ThreadID
	if threadID == "" {
		threadID = "brand-tone"
	}

	festival, product := inferFestivalProduct(req.Strategy)
	wf, err := a.RunPosterWorkflow(ctx, PosterWorkflowRequest{
		ThreadID:       threadID,
		Festival:       festival,
		ProductHint:    product,
		Brief:          strings.TrimSpace(req.Strategy),
		DrinkImage:     req.DrinkImage,
		DrinkImagePath: req.DrinkImagePath,
		DrinkMIME:      req.DrinkMIME,
		OutputPath:     req.OutputPath,
	})
	if err != nil {
		return nil, err
	}

	drinkVirt := BrandToneDrinkPath
	if ext := extForMIME(req.DrinkMIME); req.DrinkMIME != "" && ext != ".png" {
		drinkVirt = "/mnt/user-data/uploads/images/drink-ref" + ext
	} else if len(req.DrinkImage) > 0 {
		if sniffMIME(req.DrinkImage) == "image/jpeg" {
			drinkVirt = "/mnt/user-data/uploads/images/drink-ref.jpg"
		}
	}

	return &BrandToneResult{
		SandboxID:    wf.SandboxID,
		DrinkPath:    drinkVirt,
		StrategyPath: BrandToneStrategyPath,
		PromptPath:   wf.PromptPath,
		OutputPath:   wf.OutputPath,
		Prompt:       wf.Prompt,
		Bytes:        wf.Bytes,
	}, nil
}

func inferFestivalProduct(strategy string) (festival, product string) {
	return InferFestivalProduct(strategy)
}

// InferFestivalProduct picks festival + product labels from freeform strategy text.
func InferFestivalProduct(strategy string) (festival, product string) {
	festival = "国庆节"
	product = "柠檬茶新品"
	s := strategy
	for _, f := range []string{"国庆节", "中秋节", "春节", "情人节", "端午节", "圣诞节"} {
		if strings.Contains(s, f) {
			festival = f
			break
		}
	}
	switch {
	case strings.Contains(s, "柠檬"):
		product = "柠檬茶新品"
	case strings.Contains(s, "咖啡") || strings.Contains(s, "美式"):
		product = "冰美式"
	}
	return festival, product
}

func resolveDrinkImage(sb sandbox.Sandbox, req BrandToneRequest) ([]byte, string, error) {
	if len(req.DrinkImage) > 0 {
		mime := req.DrinkMIME
		if mime == "" {
			mime = sniffMIME(req.DrinkImage)
		}
		return req.DrinkImage, mime, nil
	}
	path := strings.TrimSpace(req.DrinkImagePath)
	if path == "" {
		return nil, "", fmt.Errorf("drink image is required (bytes or path)")
	}
	if strings.HasPrefix(path, "/mnt/") {
		data, err := readSandboxBytes(sb, path)
		if err != nil {
			return nil, "", fmt.Errorf("read drink image %s: %w", path, err)
		}
		mime := req.DrinkMIME
		if mime == "" {
			mime = sniffMIME(data)
		}
		return data, mime, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read drink image %s: %w", path, err)
	}
	mime := req.DrinkMIME
	if mime == "" {
		mime = sniffMIME(data)
	}
	return data, mime, nil
}

func extForMIME(mime string) string {
	switch strings.ToLower(mime) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

// ExampleCoffeeStrategy keeps the previous helper name; returns National Day lemon-tea brief.
func ExampleCoffeeStrategy() string {
	return ExampleNationalDayLemonBrief()
}

// ExampleNationalDayLemonBrief is the two-line festival brief for 国庆节 + 柠檬茶.
func ExampleNationalDayLemonBrief() string {
	return strings.TrimSpace(`情感关键词：爱国、欢庆、清爽
视觉元素：柠檬片、薄荷、冰块`)
}

// ExampleDrinkPNG returns a tiny placeholder drink-like PNG for demos/tests.
func ExampleDrinkPNG() []byte {
	img, _ := PlaceholderImage{}.Generate(context.Background(), GenerateRequest{Prompt: "drink"})
	return img
}

// DefaultBrandToneModelPrompt is used when no festival brief is provided.
func DefaultBrandToneModelPrompt() string {
	return assembleBrandTonePrompt(ExampleNationalDayLemonBrief())
}

func assembleBrandTonePrompt(brief string) string {
	emotion, visuals := parseFestivalBrief(brief)
	if emotion == "" {
		emotion = "欢庆、清爽"
	}
	if visuals == "" {
		visuals = "柠檬片、薄荷、冰块"
	}
	return strings.TrimSpace(fmt.Sprintf(`竖版 3:4 节日营销海报，纯视觉画面，画面上不要出现任何文字。必须保留参考图中的饮品主体（杯型、液体颜色、柠檬片/薄荷/冰块等配料），不要换成别的产品。

情感氛围：%s
融入视觉元素：%s（体量小于杯体，自然融合，勿生硬堆砌）
构图均衡：杯体约占画面高度 45%%–55%%，居中略偏下；背景与道具占边角，左右视觉重量平衡，勿淹没或顶满。
色调融合：以饮品液体原色为主色锚点，节日氛围色低饱和融入背景，邻近色过渡；禁止背景高饱和大色块与产品强对撞。
高级写实商业摄影，干净质感；主视觉为参考图这杯柠檬茶；节日氛围与产品一体，不遮挡杯体。
硬性禁止：任何中文、英文、数字、标语、口号、logo、水印、稿定、日日新、sensenova、字幕条。`,
		emotion, visuals,
	))
}

func parseFestivalBrief(brief string) (emotion, visuals string) {
	for _, line := range strings.Split(brief, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "情感关键词："):
			emotion = strings.TrimSpace(strings.TrimPrefix(line, "情感关键词："))
		case strings.HasPrefix(line, "视觉元素："):
			visuals = strings.TrimSpace(strings.TrimPrefix(line, "视觉元素："))
		}
	}
	// If caller passed a full scene prompt (no brief headers), use as emotion block only.
	if emotion == "" && visuals == "" {
		s := strings.TrimSpace(brief)
		if s != "" && !strings.HasPrefix(s, "#") {
			return s, "柠檬片、薄荷、冰块"
		}
	}
	return emotion, visuals
}

// SandboxForThread returns the LocalSandbox for an existing session/thread.
func (a *BrandAgent) SandboxForThread(threadID string) (sandbox.Sandbox, error) {
	sb, _, err := a.AcquireSandbox(threadID)
	return sb, err
}

// LoadHostImage is a tiny helper for demos: read a host image file.
func LoadHostImage(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return data, sniffMIME(data), nil
}

// MustAbs joins and cleans a path under base.
func MustAbs(base, name string) string {
	return filepath.Clean(filepath.Join(base, name))
}
