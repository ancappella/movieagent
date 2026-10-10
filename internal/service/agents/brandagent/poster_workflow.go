package brandagent

import (
	"context"
	"fmt"
	"strings"

	"movieagent/internal/service/assets"
	"movieagent/internal/service/sandbox"
)

const (
	PosterCreativePath = "/mnt/user-data/workspace/creative-design.md"
	PosterNodesPath    = "/mnt/user-data/workspace/poster-workflow.md"
)

// PosterWorkflowRequest drives the Gemini poster pipeline.
type PosterWorkflowRequest struct {
	ThreadID string

	Festival    string // required, e.g. 国庆节
	ProductHint string // e.g. 柠檬茶新品
	Brief       string // optional pre-filled 情感关键词/视觉元素; else Gemini brief node

	DrinkImage     []byte
	DrinkImagePath string
	DrinkMIME      string

	// Extra scene elements: image plus the user's note for how to use it.
	Elements []PosterElement

	// Brand fixed elements for the final composite node.
	BrandID            string
	BrandName          string
	BrandColors        assets.Colors
	LogoPNG            []byte
	LogoMIME           string
	LogoMeta           assets.LogoMeta
	SkipBrandComposite bool

	OutputPath string

	// OnNode is called when a workflow node starts (Status=run) or finishes.
	OnNode func(WorkflowNode)

	// RequireGemini disables heuristic fallbacks. Fail if Gemini errors.
	RequireGemini bool
}

// PosterElement is one user-supplied scene reference and its note.
type PosterElement struct {
	Note  string
	Image []byte
	MIME  string
}
type PosterWorkflowResult struct {
	SandboxID     string          `json:"sandbox_id"`
	Nodes         []WorkflowNode  `json:"nodes"`
	Festival      string          `json:"festival"`
	Brief         string          `json:"brief"`
	Creative      *CreativeDesign `json:"creative"`
	CreativePath  string          `json:"creative_path"`
	PromptPath    string          `json:"prompt_path"`
	OutputPath    string          `json:"output_path"`
	RenderPath    string          `json:"render_path,omitempty"`
	CompositePath string          `json:"composite_path,omitempty"`
	BrandID       string          `json:"brand_id,omitempty"`
	Prompt        string          `json:"prompt"`
	Bytes         int             `json:"bytes"`
	Stack         string          `json:"stack"` // e.g. gemini
}

// WorkflowNode is one named step in the poster pipeline.
type WorkflowNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // ok | skip | error
	Model  string `json:"model,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// RunPosterWorkflow executes the Gemini poster stack:
//
//	ingest → brief(gemini-flash) → creative_design(gemini-flash) → prompt → render(nano-banana-pro)
func (a *BrandAgent) RunPosterWorkflow(ctx context.Context, req PosterWorkflowRequest) (*PosterWorkflowResult, error) {
	if a == nil {
		return nil, fmt.Errorf("brandagent: nil agent")
	}
	threadID := req.ThreadID
	if threadID == "" {
		threadID = "poster-workflow"
	}
	festival := strings.TrimSpace(req.Festival)
	if festival == "" {
		return nil, fmt.Errorf("festival is required")
	}
	product := strings.TrimSpace(req.ProductHint)
	if product == "" {
		product = "饮品新品"
	}

	sb, sandboxID, err := a.AcquireSandbox(threadID)
	if err != nil {
		return nil, err
	}

	nodes := make([]WorkflowNode, 0, 5)
	mark := func(id, name, status, model, detail string) {
		n := WorkflowNode{ID: id, Name: name, Status: status, Model: model, Detail: detail}
		nodes = append(nodes, n)
		if req.OnNode != nil {
			req.OnNode(n)
		}
	}
	begin := func(id, name string) {
		if req.OnNode != nil {
			req.OnNode(WorkflowNode{ID: id, Name: name, Status: "run"})
		}
	}

	// --- node: ingest ---
	begin("ingest", "载入产品图")
	drinkBytes, drinkMIME, err := resolveDrinkImage(sb, BrandToneRequest{
		DrinkImage:     req.DrinkImage,
		DrinkImagePath: req.DrinkImagePath,
		DrinkMIME:      req.DrinkMIME,
	})
	if err != nil {
		mark("ingest", "载入产品图", "error", "", err.Error())
		return nil, fmt.Errorf("ingest: %w", err)
	}
	drinkVirt := BrandToneDrinkPath
	if ext := extForMIME(drinkMIME); ext != ".png" {
		drinkVirt = "/mnt/user-data/uploads/images/drink-ref" + ext
	}
	if err := writeSandboxBytes(sb, drinkVirt, drinkBytes); err != nil {
		mark("ingest", "载入产品图", "error", "", err.Error())
		return nil, err
	}
	mark("ingest", "载入产品图", "ok", "", drinkVirt)
	elementNotes := ingestElements(sb, req.Elements)

	designIn := CreativeDesignInput{
		Festival:     festival,
		ProductHint:  product,
		RefImage:     drinkBytes,
		RefMIME:      drinkMIME,
		ElementNotes: elementNotes,
	}

	// --- node: brief (Gemini Flash multimodal) ---
	begin("brief", "节日简报")
	brief := strings.TrimSpace(req.Brief)
	briefModel := "caller"
	if brief == "" {
		brief, briefModel, err = a.runBriefNode(ctx, designIn, req.RequireGemini)
		if err != nil {
			mark("brief", "节日简报", "error", briefModel, err.Error())
			return nil, fmt.Errorf("brief: %w", err)
		}
	}
	if err := sb.WriteFile(BrandToneFestivalBriefPath, brief+"\n", false); err != nil {
		mark("brief", "节日简报", "error", briefModel, err.Error())
		return nil, err
	}
	_ = sb.WriteFile(BrandToneStrategyPath, brief+"\n", false)
	emotion, visuals := parseFestivalBrief(brief)
	designIn.Emotion = emotion
	designIn.Visuals = visuals
	mark("brief", "节日简报", "ok", briefModel, truncateRunes(brief, 80))

	// --- node: creative_design (Gemini Flash multimodal) ---
	begin("creative_design", "创意设计")
	creative, creativeModel, err := a.runCreativeNode(ctx, designIn, req.RequireGemini)
	if err != nil {
		mark("creative_design", "创意设计", "error", creativeModel, err.Error())
		return nil, fmt.Errorf("creative_design: %w", err)
	}
	creativeText := creative.String()
	if err := sb.WriteFile(PosterCreativePath, creativeText+"\n", false); err != nil {
		mark("creative_design", "创意设计", "error", creativeModel, err.Error())
		return nil, err
	}
	mark("creative_design", "创意设计", "ok", creativeModel, truncateRunes(creative.Concept, 60))

	// --- node: prompt ---
	begin("prompt", "组装出图 Prompt")
	prompt := assemblePosterPrompt(festival, product, emotion, visuals, creative, elementNotes)
	if err := sb.WriteFile(BrandTonePromptPath, prompt+"\n", false); err != nil {
		mark("prompt", "组装出图 Prompt", "error", "local", err.Error())
		return nil, err
	}
	mark("prompt", "组装出图 Prompt", "ok", "local", truncateRunes(prompt, 80))

	// --- node: render (Nano Banana Pro / gemini-3-pro-image) ---
	begin("render", "出图")
	outPath := req.OutputPath
	if outPath == "" {
		outPath = BrandToneOutputPath
	}
	img := a.image
	renderModel := "nano-banana-pro"
	if req.RequireGemini {
		geminiImg, gerr := MustGeminiImageGenerator()
		if gerr != nil {
			mark("render", "出图", "error", "nano-banana-pro", gerr.Error())
			return nil, fmt.Errorf("render: %w", gerr)
		}
		img = geminiImg
	} else if _, ok := img.(*NanoBananaProImage); ok {
		renderModel = "nano-banana-pro"
	} else if img == nil {
		geminiImg, gerr := MustGeminiImageGenerator()
		if gerr != nil {
			mark("render", "出图", "error", "nano-banana-pro", gerr.Error())
			return nil, fmt.Errorf("render: %w", gerr)
		}
		img = geminiImg
	} else {
		renderModel = "image-generator"
	}
	var extra []ImageRef
	for _, el := range req.Elements {
		if len(el.Image) == 0 {
			continue
		}
		mime := el.MIME
		if mime == "" {
			mime = "image/png"
		}
		extra = append(extra, ImageRef{Data: el.Image, MIME: mime})
	}
	png, err := img.Generate(ctx, GenerateRequest{
		Prompt:    prompt,
		RefImage:  drinkBytes,
		RefMIME:   drinkMIME,
		ExtraRefs: extra,
	})
	if err != nil {
		mark("render", "出图", "error", renderModel, err.Error())
		return nil, fmt.Errorf("render: %w", err)
	}
	if err := writeSandboxBytes(sb, outPath, png); err != nil {
		mark("render", "出图", "error", renderModel, err.Error())
		return nil, err
	}
	mark("render", "出图", "ok", renderModel, fmt.Sprintf("%d bytes → %s", len(png), outPath))
	renderPath := outPath
	finalPNG := png
	finalPath := outPath
	compositePath := ""

	// --- node: brand_composite (brand fixed elements + aesthetic blend) ---
	begin("brand_composite", "品牌元素整合")
	if !req.SkipBrandComposite && len(req.LogoPNG) > 0 {
		comp := a.compositor
		if comp == nil {
			comp = AestheticBrandCompositor{Image: a.image}
		}
		compIn := BrandCompositeInput{
			PosterPNG: png,
			LogoPNG:   req.LogoPNG,
			LogoMIME:  req.LogoMIME,
			BrandID:   req.BrandID,
			BrandName: req.BrandName,
			Colors:    req.BrandColors,
			LogoMeta:  req.LogoMeta,
			Festival:  festival,
			Product:   product,
		}
		_ = writeSandboxBytes(sb, BrandLogoPath, req.LogoPNG)
		compPrompt := assembleBrandCompositePrompt(compIn)
		_ = sb.WriteFile(BrandCompositeMetaPath, compPrompt+"\n", false)

		blended, cerr := comp.Composite(ctx, compIn)
		if cerr != nil {
			mark("brand_composite", "品牌元素整合", "error", "brand-composite", cerr.Error())
			return nil, fmt.Errorf("brand_composite: %w", cerr)
		}
		compositePath = BrandCompositePath
		if err := writeSandboxBytes(sb, compositePath, blended); err != nil {
			mark("brand_composite", "品牌元素整合", "error", "brand-composite", err.Error())
			return nil, err
		}
		finalPNG = blended
		finalPath = compositePath
		mark("brand_composite", "品牌元素整合", "ok", "brand-composite", fmt.Sprintf("%d bytes → %s", len(blended), compositePath))
	} else {
		mark("brand_composite", "品牌元素整合", "skip", "", "no brand logo")
	}

	trace := formatWorkflowTrace(nodes, festival, product, brief, creativeText, prompt)
	_ = sb.WriteFile(PosterNodesPath, trace+"\n", false)

	stack := "mixed"
	if req.RequireGemini {
		stack = "gemini"
	}

	return &PosterWorkflowResult{
		SandboxID:     sandboxID,
		Nodes:         nodes,
		Festival:      festival,
		Brief:         brief,
		Creative:      creative,
		CreativePath:  PosterCreativePath,
		PromptPath:    BrandTonePromptPath,
		OutputPath:    finalPath,
		RenderPath:    renderPath,
		CompositePath: compositePath,
		BrandID:       req.BrandID,
		Prompt:        prompt,
		Bytes:         len(finalPNG),
		Stack:         stack,
	}, nil
}

func (a *BrandAgent) runBriefNode(ctx context.Context, in CreativeDesignInput, requireGemini bool) (brief, model string, err error) {
	if g, ok := a.creative.(*GeminiCreativeDesigner); ok && g != nil {
		b, e := g.Brief(ctx, in)
		if e == nil {
			return b, g.Service.Model(), nil
		}
		if requireGemini {
			return "", modelName(g), e
		}
	} else if requireGemini {
		g, e := NewGeminiCreativeDesignerFromEnv()
		if e != nil {
			return "", "gemini-flash", e
		}
		b, e := g.Brief(ctx, in)
		if e != nil {
			return "", g.Service.Model(), e
		}
		return b, g.Service.Model(), nil
	} else if bd, ok := a.creative.(BriefDesigner); ok && bd != nil {
		b, e := bd.Brief(ctx, in)
		if e == nil {
			return b, "brief-designer", nil
		}
	}
	return defaultFestivalBrief(in.Festival, in.ProductHint), "template", nil
}

func (a *BrandAgent) runCreativeNode(ctx context.Context, in CreativeDesignInput, requireGemini bool) (*CreativeDesign, string, error) {
	designer := a.creative
	if designer == nil {
		if requireGemini {
			g, err := NewGeminiCreativeDesignerFromEnv()
			if err != nil {
				return nil, "gemini-flash", err
			}
			designer = g
		} else {
			designer = DefaultCreativeDesigner()
		}
	}
	if requireGemini {
		if _, ok := designer.(*GeminiCreativeDesigner); !ok {
			g, err := NewGeminiCreativeDesignerFromEnv()
			if err != nil {
				return nil, "gemini-flash", err
			}
			designer = g
		}
	}
	creative, err := designer.Design(ctx, in)
	if err != nil {
		if requireGemini {
			return nil, modelName(designer), err
		}
		fallback, ferr := HeuristicCreativeDesigner{}.Design(ctx, in)
		if ferr != nil {
			return nil, modelName(designer), err
		}
		return fallback, "heuristic", nil
	}
	model := creative.Model
	if model == "" {
		model = modelName(designer)
	}
	return creative, model, nil
}

func modelName(d CreativeDesigner) string {
	if g, ok := d.(*GeminiCreativeDesigner); ok && g != nil && g.Service != nil {
		return g.Service.Model()
	}
	if _, ok := d.(HeuristicCreativeDesigner); ok {
		return "heuristic"
	}
	return "creative-designer"
}

func defaultFestivalBrief(festival, product string) string {
	switch {
	case strings.Contains(festival, "国庆"):
		return strings.TrimSpace(`情感关键词：爱国、欢庆、清爽
视觉元素：柠檬片、薄荷、冰块`)
	case strings.Contains(festival, "中秋"):
		return strings.TrimSpace(`情感关键词：团圆、温馨、清甜
视觉元素：柠檬片、月光、桂花`)
	case strings.Contains(festival, "春节"):
		return strings.TrimSpace(`情感关键词：喜庆、团圆、焕新
视觉元素：柠檬片、红金点缀、冰块`)
	default:
		_ = product
		return strings.TrimSpace(fmt.Sprintf(`情感关键词：%s氛围、清爽、心动
视觉元素：柠檬片、薄荷、冰块`, festival))
	}
}

func assemblePosterPrompt(festival, product, emotion, visuals string, c *CreativeDesign, elementNotes string) string {
	if emotion == "" {
		emotion = "欢庆、清爽"
	}
	if visuals == "" {
		visuals = "柠檬片、薄荷、冰块"
	}
	concept, composition, palette, lighting, props := "", "", "", "", ""
	if c != nil {
		concept = c.Concept
		composition = c.Composition
		palette = c.Palette
		lighting = c.Lighting
		props = c.Props
	}
	if concept == "" {
		concept = fmt.Sprintf("%s主题下突出%s真实产品", festival, product)
	}
	if composition == "" {
		composition = "竖版 3:4；杯体约占画面高度 45%–55%，居中略偏下；背景与道具占上下边角，负空间均衡"
	}
	if palette == "" {
		palette = "以饮品液体原色为主色，节日氛围色低饱和融入背景，邻近色过渡融合"
	}
	if lighting == "" {
		lighting = "柔和商业摄影光，杯壁高光清晰，背景光与产品受光方向一致"
	}
	if props == "" {
		props = visuals
	}

	elementBlock := ""
	if strings.TrimSpace(elementNotes) != "" {
		elementBlock = "\n\n用户指定元素（须按备注使用对应参考图，体量小于杯体，勿遮挡产品）：\n" + strings.TrimSpace(elementNotes)
	}

	return strings.TrimSpace(fmt.Sprintf(`竖版 3:4 节日营销海报，纯视觉画面，画面上不要出现任何文字。
必须保留参考图中的饮品主体（杯型、液体颜色、配料），不要换成别的产品。后续参考图是用户指定元素，只按备注融入，不要替换饮品。

节日氛围：%s
产品：%s
创意概念：%s
构图：%s
情感氛围：%s
色调：%s
光线：%s
点缀与视觉元素：%s（体量小于杯体，靠边角落位）
必须出现的节日元素：%s（选 2–3 个，造型可识别，不能只有饮品配料）%s

构图均衡（必须遵守）：
- 杯体是主视觉，约占画面高度约一半，四周留有呼吸感；背景氛围完整但不抢主体
- 禁止杯体过小淹没在月饼/道具堆中，也禁止杯体顶满画幅导致背景挤成边条
- 前景道具与背景元素分置上下/左右，视觉重量左右大致平衡

色调融合（必须遵守）：
- 以参考饮品的液体色与杯壁质感为画面主色锚点
- 节日色（月光、桂花金、节日红等）降低饱和度后融入背景与光晕，与饮品色同色系或邻近色过渡
- 禁止背景大面积高饱和色块与饮品形成强对比割裂；背景明度/饱和度整体低于产品主体

高级写实商业摄影，干净质感，产品与场景一体感强。

硬性禁止：任何中文、英文、数字、标语、口号、logo、水印、稿定、日日新、sensenova、YOUR LOGO、字幕条、角标文字；不要在画面上排版任何可读文字。`,
		festival, product, concept, composition, emotion, palette, lighting, props, festivalSymbols(festival), elementBlock,
	))
}

func ingestElements(sb sandbox.Sandbox, elements []PosterElement) string {
	if len(elements) == 0 {
		return ""
	}
	var notes strings.Builder
	n := 0
	for _, el := range elements {
		if len(el.Image) == 0 && strings.TrimSpace(el.Note) == "" {
			continue
		}
		n++
		note := strings.TrimSpace(el.Note)
		if note == "" {
			note = "按参考图自然融入，体量小于杯体"
		}
		fmt.Fprintf(&notes, "%d. %s\n", n, note)
		if len(el.Image) == 0 {
			continue
		}
		ext := extForMIME(el.MIME)
		if ext == "" {
			ext = ".png"
		}
		path := fmt.Sprintf("/mnt/user-data/uploads/images/element-%d%s", n, ext)
		_ = writeSandboxBytes(sb, path, el.Image)
	}
	text := strings.TrimSpace(notes.String())
	if text != "" {
		_ = sb.WriteFile("/mnt/user-data/workspace/elements.md", text+"\n", false)
	}
	return text
}

func formatWorkflowTrace(nodes []WorkflowNode, festival, product, brief, creative, prompt string) string {
	var b strings.Builder
	b.WriteString("# Poster Workflow (Gemini)\n\n")
	fmt.Fprintf(&b, "festival: %s\nproduct: %s\n\n", festival, product)
	b.WriteString("## Nodes\n")
	for _, n := range nodes {
		fmt.Fprintf(&b, "- [%s] %s (%s)", n.Status, n.Name, n.ID)
		if n.Model != "" {
			fmt.Fprintf(&b, " model=%s", n.Model)
		}
		if n.Detail != "" {
			fmt.Fprintf(&b, ": %s", n.Detail)
		}
		b.WriteByte('\n')
	}
	b.WriteString("\n## Brief\n")
	b.WriteString(brief)
	b.WriteString("\n\n## Creative Design\n")
	b.WriteString(creative)
	b.WriteString("\n\n## Prompt\n")
	b.WriteString(prompt)
	b.WriteByte('\n')
	return b.String()
}

func truncateRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}
