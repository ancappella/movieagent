package brandagent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"movieagent/internal/service/providers/gemini"
)

// CreativeDesign is the output of the creative-design node.
type CreativeDesign struct {
	Concept     string `json:"concept"`
	Composition string `json:"composition"`
	Headline    string `json:"headline"`
	Palette     string `json:"palette"`
	Lighting    string `json:"lighting"`
	Props       string `json:"props"`
	Raw         string `json:"raw"`
	Model       string `json:"model,omitempty"`
}

// CreativeDesignInput feeds the creative-design node.
type CreativeDesignInput struct {
	Festival     string // e.g. 国庆节
	Emotion      string
	Visuals      string
	ProductHint  string // e.g. 柠檬茶新品
	RefImage     []byte // optional product photo for Gemini multimodal
	RefMIME      string
	ElementNotes string // user element remarks, one block of text
}

// CreativeDesigner is the creative-design workflow node.
type CreativeDesigner interface {
	Design(ctx context.Context, in CreativeDesignInput) (*CreativeDesign, error)
}

// CreativeDesignerFunc adapts a function.
type CreativeDesignerFunc func(ctx context.Context, in CreativeDesignInput) (*CreativeDesign, error)

func (f CreativeDesignerFunc) Design(ctx context.Context, in CreativeDesignInput) (*CreativeDesign, error) {
	return f(ctx, in)
}

// BriefDesigner produces the two-line festival brief (optionally multimodal).
type BriefDesigner interface {
	Brief(ctx context.Context, in CreativeDesignInput) (string, error)
}

// BriefDesignerFunc adapts a function.
type BriefDesignerFunc func(ctx context.Context, in CreativeDesignInput) (string, error)

func (f BriefDesignerFunc) Brief(ctx context.Context, in CreativeDesignInput) (string, error) {
	return f(ctx, in)
}

// HeuristicCreativeDesigner is an offline fallback for unit tests only.
type HeuristicCreativeDesigner struct{}

func (HeuristicCreativeDesigner) Design(_ context.Context, in CreativeDesignInput) (*CreativeDesign, error) {
	fest := strings.TrimSpace(in.Festival)
	if fest == "" {
		fest = "节日"
	}
	product := strings.TrimSpace(in.ProductHint)
	if product == "" {
		product = "饮品"
	}
	emotion := strings.TrimSpace(in.Emotion)
	if emotion == "" {
		emotion = "欢庆、清爽"
	}
	visuals := strings.TrimSpace(in.Visuals)
	if visuals == "" {
		visuals = "柠檬片、薄荷、冰块"
	}
	symbols := festivalSymbols(fest)
	palette := heuristicPalette(fest, product)
	extra := ""
	if notes := strings.TrimSpace(in.ElementNotes); notes != "" {
		extra = "\n用户元素备注：\n" + notes
	}
	raw := fmt.Sprintf(`概念：以一杯真实%s为绝对主视觉，%s氛围与产品色调自然融合，并带上可识别的节日符号
构图：竖版 3:4；杯体约占画面高度 45%%–55%%、居中略偏下；节日元素分置上下边角，负空间均衡，勿堆满压住产品
主文案：（不上图）
色调：%s
光线：柔和商业棚拍光，杯壁高光与水珠清晰，背景光与产品受光方向一致
点缀：饮品配料（%s）之外，必须出现节日元素：%s；每种节日符号体量小于杯体，2–3 个即可，靠边角，勿环绕遮挡%s`, product, fest, palette, visuals, symbols, extra)
	d := parseCreativeDesign(raw)
	d.Model = "heuristic"
	return d, nil
}

func heuristicPalette(festival, product string) string {
	switch {
	case strings.Contains(festival, "中秋"):
		return "以饮品琥珀色/柠檬黄为画面主色，月光银白与桂花浅金作氛围融合色；背景饱和度低于产品，禁止大面积朱红抢色"
	case strings.Contains(festival, "春节"):
		return "饮品原色为主，红金仅作边角点缀；背景明度压低，避免满屏红淹没杯体"
	case strings.Contains(festival, "国庆"):
		return "饮品清透原色为主，节日红金点缀不超过画面 20%；背景与杯体贴色过渡，勿高饱和对撞"
	default:
		_ = product
		return "以参考饮品的液体色为画面主色，节日色仅作低饱和氛围融合；背景与产品色相邻近，避免强对比割裂"
	}
}

// festivalSymbols are recognizable motifs the creative brief must include.
func festivalSymbols(festival string) string {
	switch {
	case strings.Contains(festival, "中秋"):
		return "圆月、桂花、月饼、玉兔"
	case strings.Contains(festival, "国庆"):
		return "红金绸带、五角星、礼花光斑"
	case strings.Contains(festival, "春节"):
		return "红灯笼、窗花剪影、梅花"
	case strings.Contains(festival, "元宵"):
		return "花灯、圆月、汤圆"
	case strings.Contains(festival, "端午"):
		return "粽叶、龙舟剪影、艾草"
	case strings.Contains(festival, "情人"):
		return "玫瑰、心形光斑、丝带"
	case strings.Contains(festival, "圣诞"):
		return "冷杉枝、暖色灯串、红色球饰"
	default:
		return "该节日最典型的 2–3 个符号（灯笼、花卉、月相或节庆道具），不要只放饮品配料"
	}
}

func (HeuristicCreativeDesigner) Brief(_ context.Context, in CreativeDesignInput) (string, error) {
	return defaultFestivalBrief(in.Festival, in.ProductHint), nil
}

// GeminiCreativeDesigner uses Gemini Flash as art director (multimodal).
type GeminiCreativeDesigner struct {
	Service *gemini.TextService
}

// NewGeminiCreativeDesignerFromEnv builds a designer when GEMINI_API_KEY is set.
func NewGeminiCreativeDesignerFromEnv() (*GeminiCreativeDesigner, error) {
	if os.Getenv(gemini.APIKeyEnv) == "" {
		return nil, gemini.ErrNotConfigured
	}
	return &GeminiCreativeDesigner{
		Service: gemini.NewTextService(gemini.Config{}),
	}, nil
}

func (d *GeminiCreativeDesigner) Design(ctx context.Context, in CreativeDesignInput) (*CreativeDesign, error) {
	if d == nil || d.Service == nil {
		return nil, gemini.ErrNotConfigured
	}
	fest := strings.TrimSpace(in.Festival)
	if fest == "" {
		fest = "节日"
	}
	product := strings.TrimSpace(in.ProductHint)
	if product == "" {
		product = "饮品新品"
	}
	emotion := strings.TrimSpace(in.Emotion)
	visuals := strings.TrimSpace(in.Visuals)

	symbols := festivalSymbols(fest)
	elementHint := ""
	if notes := strings.TrimSpace(in.ElementNotes); notes != "" {
		elementHint = "\n用户上传的元素备注（必须体现在点缀里，按备注使用，勿压住产品）：\n" + notes + "\n"
	}
	prompt := fmt.Sprintf(`你是资深饮料品牌海报艺术总监。请结合参考产品图（若有）输出「创意设计简报」。
只输出下列 6 行，不要编号、不要解释、不要 markdown。

节日：%s
产品：%s
情感关键词：%s
视觉元素：%s
本节日必须出现的符号：%s
%s
输出格式（必须逐行）：
概念：一句话创意方向（产品是绝对主视觉，同时点明要用哪些节日符号）
构图：竖版 3:4；杯体约占画面高度 45%%–55%%；写明节日符号分别落在哪些边角，不能挤在一起
主文案：（不上图）写「无」
色调：以参考饮品液体色为主色；节日色如何低饱和融入背景
光线：光线与质感（背景光与产品受光方向一致，可借节日光源如月光、灯笼光）
点缀：先写饮品配料，再写 2–3 个具体节日符号的体量与落位（每个都小于杯体）

硬性约束：
- 必须保留参考图中的真实饮品外观，禁止换成别的产品
- 画面里必须有可识别的节日元素，不能只有柠檬、薄荷、冰块
- 节日符号选「本节日必须出现的符号」里的 2–3 个，造型具体，不要抽象成纯色块
- 产品与背景占比均衡：节日元素不得压住杯体，也不得多到淹没产品
- 色调融合：节日氛围色从饮品原色延展，邻近色过渡
- 最终海报为纯视觉，画面上不要任何文字/口号/logo/水印`, fest, product, emotion, visuals, symbols, elementHint)

	raw, err := d.Service.CompleteWith(ctx, gemini.CompleteInput{
		Prompt:   prompt,
		RefImage: in.RefImage,
		RefMIME:  in.RefMIME,
	})
	if err != nil {
		return nil, err
	}
	out := parseCreativeDesign(raw)
	out.Model = d.Service.Model()
	return out, nil
}

// Brief uses Gemini Flash to extract the two-line festival brief from festival + product photo.
func (d *GeminiCreativeDesigner) Brief(ctx context.Context, in CreativeDesignInput) (string, error) {
	if d == nil || d.Service == nil {
		return "", gemini.ErrNotConfigured
	}
	fest := strings.TrimSpace(in.Festival)
	if fest == "" {
		fest = "节日"
	}
	product := strings.TrimSpace(in.ProductHint)
	if product == "" {
		product = "饮品新品"
	}
	prompt := fmt.Sprintf(`你是品牌策划。根据节日与产品参考图，只输出两行简报，不要解释、不要 markdown：

情感关键词：…（围绕节日，可被产品氛围微调，顿号分隔）
视觉元素：…（恰好 3 个，优先从图中抽取，不足用节日经典元素补齐）

节日：%s
产品：%s`, fest, product)

	raw, err := d.Service.CompleteWith(ctx, gemini.CompleteInput{
		Prompt:   prompt,
		RefImage: in.RefImage,
		RefMIME:  in.RefMIME,
	})
	if err != nil {
		return "", err
	}
	return normalizeFestivalBrief(raw, fest, product), nil
}

// DefaultCreativeDesigner prefers Gemini; tests should inject Heuristic explicitly.
func DefaultCreativeDesigner() CreativeDesigner {
	if d, err := NewGeminiCreativeDesignerFromEnv(); err == nil {
		return d
	}
	return HeuristicCreativeDesigner{}
}

// MustGeminiCreativeDesigner requires Gemini; errors if key missing.
func MustGeminiCreativeDesigner() (CreativeDesigner, error) {
	return NewGeminiCreativeDesignerFromEnv()
}

func parseCreativeDesign(raw string) *CreativeDesign {
	d := &CreativeDesign{Raw: strings.TrimSpace(raw)}
	for _, line := range strings.Split(d.Raw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		switch {
		case strings.HasPrefix(line, "概念："):
			d.Concept = strings.TrimSpace(strings.TrimPrefix(line, "概念："))
		case strings.HasPrefix(line, "构图："):
			d.Composition = strings.TrimSpace(strings.TrimPrefix(line, "构图："))
		case strings.HasPrefix(line, "主文案："):
			d.Headline = strings.TrimSpace(strings.TrimPrefix(line, "主文案："))
		case strings.HasPrefix(line, "色调："):
			d.Palette = strings.TrimSpace(strings.TrimPrefix(line, "色调："))
		case strings.HasPrefix(line, "光线："):
			d.Lighting = strings.TrimSpace(strings.TrimPrefix(line, "光线："))
		case strings.HasPrefix(line, "点缀："):
			d.Props = strings.TrimSpace(strings.TrimPrefix(line, "点缀："))
		}
	}
	return d
}

func normalizeFestivalBrief(raw, festival, product string) string {
	emotion, visuals := parseFestivalBrief(raw)
	if emotion != "" && visuals != "" {
		return "情感关键词：" + emotion + "\n视觉元素：" + visuals
	}
	return defaultFestivalBrief(festival, product)
}

func (d *CreativeDesign) String() string {
	if d == nil {
		return ""
	}
	if strings.TrimSpace(d.Raw) != "" {
		return strings.TrimSpace(d.Raw)
	}
	var b strings.Builder
	write := func(k, v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		fmt.Fprintf(&b, "%s：%s\n", k, strings.TrimSpace(v))
	}
	write("概念", d.Concept)
	write("构图", d.Composition)
	write("主文案", d.Headline)
	write("色调", d.Palette)
	write("光线", d.Lighting)
	write("点缀", d.Props)
	return strings.TrimSpace(b.String())
}
