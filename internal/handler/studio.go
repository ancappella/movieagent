package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	appcfg "movieagent/internal/config"
	"movieagent/internal/service/agent_loop"
	"movieagent/internal/service/agents/brandagent"
	"movieagent/internal/service/assets"
	svcconfig "movieagent/internal/service/config"
	svctools "movieagent/internal/service/tools"
	"movieagent/pkg/response"

	"github.com/gin-gonic/gin"
)

// StudioHandler serves asset upload, tasks, and robot session APIs.
type StudioHandler struct {
	ProjectRoot string
	AssetsYAML  string
	BrandAgent  *brandagent.BrandAgent
	Paths       *svcconfig.Paths
	Sessions    *agent_loop.Hub
}

func NewStudioHandler(projectRoot, assetsYAML string) *StudioHandler {
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	if assetsYAML == "" {
		assetsYAML = filepath.Join(projectRoot, assets.DefaultCatalogPath())
	}
	// Ensure GEMINI_API_KEY from .env is in process env before BrandAgent / sandbox use it.
	_ = appcfg.LoadDotEnv(filepath.Join(projectRoot, ".env"))

	// Register local tools (picturebook_illustrate, …) at service startup.
	svctools.Bootstrap(projectRoot, os.Getenv("PICTUREBOOK_PYTHON"))

	paths := svcconfig.GetPaths()
	ba, err := brandagent.New(brandagent.Options{
		Paths:    paths,
		UserID:   brandagent.DefaultUserID,
		Image:    brandagent.DefaultImageGenerator(),
		Creative: brandagent.DefaultCreativeDesigner(),
	})
	if err != nil {
		// Keep studio up even if agent init fails; CreateTask will surface the error.
		ba = nil
	}
	return &StudioHandler{
		ProjectRoot: projectRoot,
		AssetsYAML:  assetsYAML,
		BrandAgent:  ba,
		Paths:       paths,
		Sessions:    agent_loop.NewHub(),
	}
}

func (h *StudioHandler) UploadAsset(c *gin.Context) {
	kind := c.PostForm("kind")
	brandID := c.PostForm("brand_id")
	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, http.StatusBadRequest, 1, "missing file")
		return
	}
	src, err := file.Open()
	if err != nil {
		response.Fail(c, http.StatusBadRequest, 1, err.Error())
		return
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, 64<<20)) // 64MB
	if err != nil {
		response.Fail(c, http.StatusBadRequest, 1, err.Error())
		return
	}
	rec, err := assets.SaveUpload(h.ProjectRoot, brandID, kind, file.Filename, data)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	rec.ContentType = file.Header.Get("Content-Type")
	response.OK(c, rec)
}

func (h *StudioHandler) CreateTask(c *gin.Context) {
	var body struct {
		AgentID  string        `json:"agent_id"`
		Mode     string        `json:"mode"`
		Prompt   string        `json:"prompt"`
		BrandID  string        `json:"brand_id"`
		AssetIDs []string      `json:"asset_ids"`
		Elements []taskElement `json:"elements"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, http.StatusBadRequest, 1, "invalid json")
		return
	}
	body.Prompt = strings.TrimSpace(body.Prompt)
	if body.Prompt == "" && len(body.AssetIDs) == 0 && len(body.Elements) == 0 {
		response.Fail(c, http.StatusBadRequest, 1, "prompt or assets required")
		return
	}
	if body.AgentID == "" {
		body.AgentID = "brand-agent"
	}

	switch body.AgentID {
	case "brand-agent", "brandagent", "品牌agent", "品牌 Agent", "海报工作室":
		h.runBrandPosterTask(c, body.Prompt, body.BrandID, body.AssetIDs, body.Elements, body.Mode)
	default:
		response.Fail(c, http.StatusBadRequest, 1, "unsupported agent_id: "+body.AgentID)
	}
}

type taskElement struct {
	AssetID string `json:"asset_id"`
	Note    string `json:"note"`
}

func (h *StudioHandler) runBrandPosterTask(c *gin.Context, prompt, brandID string, assetIDs []string, elements []taskElement, mode string) {
	_, payload, err := h.executeBrandPoster(c.Request.Context(), prompt, brandID, assetIDs, elements, mode, "", nil)
	if err != nil {
		code := http.StatusInternalServerError
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "missing") {
			code = http.StatusBadRequest
		}
		if strings.Contains(err.Error(), "brand-agent is not initialized") {
			response.Fail(c, http.StatusInternalServerError, 1, err.Error())
			return
		}
		response.Fail(c, code, 1, err.Error())
		return
	}
	response.OK(c, payload)
}

func (h *StudioHandler) executeBrandPoster(ctx context.Context, prompt, brandID string, assetIDs []string, elements []taskElement, mode, threadID string, onNode func(brandagent.WorkflowNode)) (*brandagent.PosterWorkflowResult, gin.H, error) {
	if h.BrandAgent == nil {
		return nil, nil, fmt.Errorf("brand-agent is not initialized")
	}

	strategy := prompt
	if strategy == "" {
		strategy = brandagent.ExampleNationalDayLemonBrief()
	}
	festival, product := brandagent.InferFestivalProduct(strategy)

	drinkPath, drinkMIME, err := h.resolveDrinkAsset(assetIDs)
	if err != nil {
		return nil, nil, err
	}
	var drinkBytes []byte
	if drinkPath == "" {
		drinkBytes = brandagent.ExampleDrinkPNG()
		drinkMIME = "image/png"
	}

	if threadID == "" {
		threadID = fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	if len(threadID) > 64 {
		threadID = threadID[:64]
	}

	posterElements, err := h.loadPosterElements(elements)
	if err != nil {
		return nil, nil, err
	}

	req := brandagent.PosterWorkflowRequest{
		ThreadID:       threadID,
		Festival:       festival,
		ProductHint:    product,
		Brief:          strategy,
		DrinkImage:     drinkBytes,
		DrinkImagePath: drinkPath,
		DrinkMIME:      drinkMIME,
		Elements:       posterElements,
		BrandID:        brandID,
		OnNode:         onNode,
		RequireGemini:  true,
	}
	if brandID != "" {
		if cat, cerr := assets.LoadCatalog(h.AssetsYAML); cerr == nil {
			if b, berr := cat.FindBrand(brandID); berr == nil {
				req.BrandName = b.Name
				req.BrandColors = b.Colors
				req.LogoMeta = b.Visuals.Logo.Meta
			}
			if _, logo, meta, lerr := cat.ResolveLogoPNG(h.ProjectRoot, brandID); lerr == nil {
				req.LogoPNG = logo
				req.LogoMIME = "image/png"
				req.LogoMeta = meta
			}
		}
	}

	res, err := h.BrandAgent.RunPosterWorkflow(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	creativeText := ""
	if res.Creative != nil {
		creativeText = strings.TrimSpace(res.Creative.String())
	}
	if creativeText == "" {
		if sb, serr := h.BrandAgent.SandboxForThread(threadID); serr == nil {
			if raw, rerr := sb.ReadFile(brandagent.PosterCreativePath, 0, 0); rerr == nil {
				creativeText = strings.TrimSpace(raw)
			}
		}
	}

	msg := "已提交生成这组画面，等出图后你确认一下细节和氛围，没问题就可以继续往下做。"
	if res.CompositePath != "" {
		msg = "已提交生成这组画面（含品牌元素整合），等出图后你确认一下细节和氛围，没问题就可以继续往下做。"
	}

	imageURL := sandboxFileURL(threadID, res.OutputPath)
	renderURL := sandboxFileURL(threadID, res.RenderPath)
	imageURLs := uniqueImageURLs(renderURL, imageURL)

	payload := gin.H{
		"accepted":       true,
		"agent_id":       "brand-agent",
		"mode":           mode,
		"brand_id":       brandID,
		"prompt":         prompt,
		"asset_ids":      assetIDs,
		"elements":       elements,
		"thread_id":      threadID,
		"sandbox_id":     res.SandboxID,
		"festival":       res.Festival,
		"brief":          res.Brief,
		"nodes":          res.Nodes,
		"output_path":    res.OutputPath,
		"render_path":    res.RenderPath,
		"composite_path": res.CompositePath,
		"image_url":      imageURL,
		"render_url":     renderURL,
		"image_urls":     imageURLs,
		"creative_url":   sandboxFileURL(threadID, res.CreativePath),
		"creative_text":  creativeText,
		"bytes":          res.Bytes,
		"message":        msg,
		"suggestions": []string{
			"把这组画面做成动态短视频",
			"再生成一版同风格画面",
			"给画面加更有氛围的场景背景",
		},
	}
	return res, payload, nil
}

func (h *StudioHandler) resolveDrinkAsset(assetIDs []string) (hostPath, mime string, err error) {
	for _, id := range assetIDs {
		path, ferr := assets.FindUploadByID(h.ProjectRoot, id)
		if ferr != nil {
			return "", "", ferr
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif":
			mime = "image/png"
			if ext == ".jpg" || ext == ".jpeg" {
				mime = "image/jpeg"
			} else if ext == ".webp" {
				mime = "image/webp"
			} else if ext == ".gif" {
				mime = "image/gif"
			}
			return path, mime, nil
		}
	}
	return "", "", nil
}

func (h *StudioHandler) loadPosterElements(in []taskElement) ([]brandagent.PosterElement, error) {
	const maxElements = 6
	if len(in) > maxElements {
		in = in[:maxElements]
	}
	out := make([]brandagent.PosterElement, 0, len(in))
	for _, el := range in {
		item := brandagent.PosterElement{Note: strings.TrimSpace(el.Note)}
		if id := strings.TrimSpace(el.AssetID); id != "" {
			path, err := assets.FindUploadByID(h.ProjectRoot, id)
			if err != nil {
				return nil, err
			}
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".mp4", ".mov", ".webm", ".m4v", ".avi", ".mkv":
				if item.Note == "" {
					item.Note = "参考视频 " + filepath.Base(path)
				}
				out = append(out, item)
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			item.Image = data
			item.MIME = mimeForExt(ext)
		}
		if len(item.Image) == 0 && item.Note == "" {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func mimeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/png"
	}
}

func sandboxFileURL(threadID, virtualPath string) string {
	if strings.TrimSpace(threadID) == "" || strings.TrimSpace(virtualPath) == "" {
		return ""
	}
	q := url.Values{}
	q.Set("thread_id", threadID)
	q.Set("path", virtualPath)
	return "/api/v1/sandbox/file?" + q.Encode()
}

func uniqueImageURLs(urls ...string) []string {
	out := make([]string, 0, len(urls))
	seen := map[string]struct{}{}
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

// ServeSandboxFile streams a LocalSandbox virtual path for the given thread.
func (h *StudioHandler) ServeSandboxFile(c *gin.Context) {
	threadID := strings.TrimSpace(c.Query("thread_id"))
	virtual := strings.TrimSpace(c.Query("path"))
	if threadID == "" || virtual == "" {
		response.Fail(c, http.StatusBadRequest, 1, "thread_id and path required")
		return
	}
	if h.Paths == nil {
		response.Fail(c, http.StatusInternalServerError, 1, "paths not configured")
		return
	}
	userID := brandagent.DefaultUserID
	hostPath, err := h.Paths.ResolveVirtualPath(threadID, virtual, &userID)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, 1, err.Error())
		return
	}
	if _, err := os.Stat(hostPath); err != nil {
		if os.IsNotExist(err) {
			response.Fail(c, http.StatusNotFound, 1, "file not found")
			return
		}
		response.Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	c.File(hostPath)
}
