package brandagent

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"strings"

	"movieagent/internal/service/assets"
)

const (
	BrandLogoPath          = "/mnt/user-data/uploads/images/brand-logo.png"
	BrandCompositePath     = "/mnt/user-data/outputs/brand-campaign-final.png"
	BrandCompositeMetaPath = "/mnt/user-data/workspace/brand-composite.md"
)

// BrandCompositeInput feeds the final brand-fixed-elements node.
type BrandCompositeInput struct {
	PosterPNG []byte
	LogoPNG   []byte
	LogoMIME  string
	BrandID   string
	BrandName string
	Colors    assets.Colors
	LogoMeta  assets.LogoMeta
	Festival  string
	Product   string
}

// BrandCompositor integrates brand-fixed elements into a finished poster.
type BrandCompositor interface {
	Composite(ctx context.Context, in BrandCompositeInput) ([]byte, error)
}

// AestheticBrandCompositor uses an image model to blend logo + brand colors
// into the poster with aesthetic balance (preferred). Falls back to local overlay.
type AestheticBrandCompositor struct {
	Image ImageGenerator
}

func (c AestheticBrandCompositor) Composite(ctx context.Context, in BrandCompositeInput) ([]byte, error) {
	if len(in.PosterPNG) == 0 {
		return nil, fmt.Errorf("poster image is required")
	}
	if len(in.LogoPNG) == 0 {
		return in.PosterPNG, nil
	}
	prompt := assembleBrandCompositePrompt(in)
	if c.Image != nil {
		out, err := c.Image.Generate(ctx, GenerateRequest{
			Prompt:   prompt,
			RefImage: in.PosterPNG,
			RefMIME:  "image/png",
			ExtraRefs: []ImageRef{{
				Data: in.LogoPNG,
				MIME: firstNonEmpty(in.LogoMIME, "image/png"),
			}},
		})
		if err == nil && len(out) > 0 {
			return out, nil
		}
	}
	return overlayLogoLocal(in.PosterPNG, in.LogoPNG, in.LogoMeta)
}

// LocalBrandCompositor only alpha-composites the logo (offline / tests).
type LocalBrandCompositor struct{}

func (LocalBrandCompositor) Composite(_ context.Context, in BrandCompositeInput) ([]byte, error) {
	if len(in.PosterPNG) == 0 {
		return nil, fmt.Errorf("poster image is required")
	}
	if len(in.LogoPNG) == 0 {
		return in.PosterPNG, nil
	}
	return overlayLogoLocal(in.PosterPNG, in.LogoPNG, in.LogoMeta)
}

func assembleBrandCompositePrompt(in BrandCompositeInput) string {
	name := strings.TrimSpace(in.BrandName)
	if name == "" {
		name = strings.TrimSpace(in.BrandID)
	}
	if name == "" {
		name = "品牌"
	}
	c := in.Colors
	primary := firstNonEmpty(c.Primary, "#1A120C")
	sec1 := firstNonEmpty(c.Secondary1, "#C4A574")
	sec2 := firstNonEmpty(c.Secondary2, "#F5F0E8")

	marginHint := "右下角安全区内"
	if in.LogoMeta.SafeMarginRatio > 0 {
		marginHint = fmt.Sprintf("距边缘约 %.0f%% 安全边距的右下角", in.LogoMeta.SafeMarginRatio*100)
	} else if in.LogoMeta.SafeMarginPx > 0 {
		marginHint = fmt.Sprintf("距边缘约 %dpx 的右下角", in.LogoMeta.SafeMarginPx)
	}

	return strings.TrimSpace(fmt.Sprintf(`你是资深品牌视觉设计师。输入有两张图：
1) 第一张：已生成的节日营销海报（主视觉是饮品，无品牌 Logo）
2) 第二张：品牌固定元素（Logo），必须忠实保留其图形与配色，不要重绘成别的标志

任务：把品牌固定元素自然整合进海报，输出一张完整竖版 3:4 成片。

品牌：%s
节日：%s
产品：%s
品牌色：主色 %s，辅色 %s / %s

审美与整合规则（必须遵守）：
- 构图均衡：Logo 体量约为画面短边的 10%%–14%%，放在 %s；不得压住杯体主视觉，不得与月饼/道具重叠抢戏
- 色调融合：用品牌色做极克制的氛围呼应（光边、地面反射或角落色温微调），饱和度低于产品主体；禁止铺满品牌色背景重绘整张图
- 光影一致：Logo 受光方向、对比度与海报一致，可有轻微投影/边缘融入，避免硬贴贴纸感
- 保留第一张海报的产品外观、构图比例与节日氛围；不要新增任何文字、口号、水印、角标文案
- 输出纯视觉成片，画面上除 Logo 图形外不要出现可读文字

硬性禁止：改变饮品品类、放大 Logo 至喧宾夺主、在画面中央贴 Logo、添加 sensenova/稿定 等水印。`,
		name, firstNonEmpty(in.Festival, "节日"), firstNonEmpty(in.Product, "饮品"),
		primary, sec1, sec2, marginHint,
	))
}

func overlayLogoLocal(posterPNG, logoPNG []byte, meta assets.LogoMeta) ([]byte, error) {
	poster, err := decodeImage(posterPNG)
	if err != nil {
		return nil, err
	}
	logo, err := decodeImage(logoPNG)
	if err != nil {
		return nil, err
	}
	pb := poster.Bounds()
	lb := logo.Bounds()
	short := pb.Dx()
	if pb.Dy() < short {
		short = pb.Dy()
	}
	targetW := short * 12 / 100
	if targetW < 48 {
		targetW = 48
	}
	scale := float64(targetW) / float64(lb.Dx())
	targetH := int(float64(lb.Dy()) * scale)
	if targetH < 1 {
		targetH = 1
	}
	resized := resizeNearest(logo, targetW, targetH)

	margin := meta.SafeMarginPx
	if margin <= 0 && meta.SafeMarginRatio > 0 {
		margin = int(float64(short) * meta.SafeMarginRatio)
	}
	if margin <= 0 {
		margin = short * 6 / 100
	}
	x := pb.Dx() - targetW - margin
	y := pb.Dy() - targetH - margin
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	dst := image.NewRGBA(pb)
	draw.Draw(dst, pb, poster, pb.Min, draw.Src)
	draw.Draw(dst, image.Rect(x, y, x+targetW, y+targetH), resized, image.Point{}, draw.Over)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func resizeNearest(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	for y := 0; y < h; y++ {
		sy := sb.Min.Y + y*sh/h
		for x := 0; x < w; x++ {
			sx := sb.Min.X + x*sw/w
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
