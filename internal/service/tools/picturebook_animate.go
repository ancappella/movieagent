package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"movieagent/internal/service/config"
	"movieagent/internal/service/herness"
)

const (
	PictureBookAnimateToolName   = "picturebook_animate"
	PictureBookAnimateDefaultIn  = PictureBookDefaultOut
	PictureBookAnimateDefaultOut = "/mnt/user-data/outputs/picturebook.mp4"
)

// NewPictureBookAnimator builds the LLM-code → Playwright frames → ffmpeg MP4 tool.
// Prefer binding via skill picturebook-animate (scripts/render.mjs).
func NewPictureBookAnimator(opts Options) herness.Tool {
	root := strings.TrimSpace(opts.ProjectRoot)
	sb := opts.Sandbox
	skillScript := strings.TrimSpace(opts.SkillScript)
	skillDir := strings.TrimSpace(opts.SkillDir)

	return herness.ToolFunc{
		ToolName: PictureBookAnimateToolName,
		ToolDesc: "Skill picturebook-animate: run scripts/render.mjs. Pass animation JS with draw(t,ctx,canvas,assets) (+ optional seek/setup). Playwright screenshots each frame; ffmpeg encodes H.264 MP4. Optional image_path becomes assets.image. If code is empty, template character_spin|character_idle is used.",
		Params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{
					"type": "string",
					"description": "Runnable browser JS defining draw(t, ctx, canvas, assets) where t is seconds. " +
						"Optional seek(t) and async setup({canvas,ctx,width,height,assets}). " +
						"This is the LLM product (text code), not a video.",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "User animation request (for logs / fallback templates), e.g. 玩偶原地旋转 10 秒",
				},
				"image_path": map[string]any{
					"type":        "string",
					"description": "Optional sandbox PNG/JPG exposed as assets.image; default /mnt/user-data/outputs/picturebook.png when present",
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Sandbox virtual path for MP4; default /mnt/user-data/outputs/picturebook.mp4",
				},
				"template": map[string]any{
					"type":        "string",
					"enum":        []string{"character_spin", "character_idle"},
					"description": "Fallback bundled program when code is empty. Default character_spin.",
				},
				"duration_sec": map[string]any{
					"type":        "number",
					"description": "Clip length in seconds (default 4, max 30)",
				},
				"fps": map[string]any{
					"type":        "integer",
					"description": "Frames per second (default 24)",
				},
				"width": map[string]any{
					"type":        "integer",
					"description": "Output width px (default 512)",
				},
				"height": map[string]any{
					"type":        "integer",
					"description": "Output height px (default 768)",
				},
			},
			"required": []string{},
		},
		Fn: func(ctx context.Context, args json.RawMessage) (herness.ToolResult, error) {
			var in struct {
				Code        string  `json:"code"`
				Prompt      string  `json:"prompt"`
				ImagePath   string  `json:"image_path"`
				OutputPath  string  `json:"output_path"`
				Template    string  `json:"template"`
				DurationSec float64 `json:"duration_sec"`
				FPS         int     `json:"fps"`
				Width       int     `json:"width"`
				Height      int     `json:"height"`
			}
			if len(bytes.TrimSpace(args)) > 0 {
				if err := json.Unmarshal(args, &in); err != nil {
					return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError(PictureBookAnimateToolName, err)}, nil
				}
			}

			outVirt := strings.TrimSpace(in.OutputPath)
			if outVirt == "" {
				outVirt = PictureBookAnimateDefaultOut
			}
			code := strings.TrimSpace(in.Code)
			template := strings.TrimSpace(in.Template)
			if template == "" {
				template = "character_spin"
			}
			if code == "" && strings.Contains(strings.ToLower(in.Prompt+template), "idle") {
				template = "character_idle"
			}

			projRoot := root
			if projRoot == "" {
				if r, err := config.ProjectRoot(); err == nil {
					projRoot = r
				} else {
					projRoot, _ = os.Getwd()
				}
			}
			script := strings.TrimSpace(skillScript)
			if script == "" {
				script = resolvePictureBookAnimateScript(projRoot)
			}
			if script == "" {
				return herness.ToolResult{
					Error:   "picturebook-animate skill script missing",
					Content: herness.FormatToolError(PictureBookAnimateToolName, fmt.Errorf("skill picturebook-animate scripts/render.mjs missing")),
				}, nil
			}

			node := resolvePictureBookAnimateNode(projRoot, skillDir)
			tmpDir, err := os.MkdirTemp("", "picturebook-anim-*")
			if err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError(PictureBookAnimateToolName, err)}, nil
			}
			defer os.RemoveAll(tmpDir)
			tmpOut := filepath.Join(tmpDir, "out.mp4")
			tmpCode := filepath.Join(tmpDir, "animation.js")

			cmdArgs := []string{
				script,
				"--output", tmpOut,
			}

			if code != "" {
				if err := os.WriteFile(tmpCode, []byte(code), 0o644); err != nil {
					return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError(PictureBookAnimateToolName, err)}, nil
				}
				cmdArgs = append(cmdArgs, "--code-file", tmpCode)
			} else {
				cmdArgs = append(cmdArgs, "--template", template)
			}

			// Optional still asset for assets.image
			inVirt := strings.TrimSpace(in.ImagePath)
			if inVirt == "" {
				inVirt = PictureBookAnimateDefaultIn
			}
			if png, rerr := readSandboxBytes(sb, inVirt); rerr == nil && len(png) >= 32 {
				tmpIn := filepath.Join(tmpDir, "asset.png")
				if err := os.WriteFile(tmpIn, png, 0o644); err == nil {
					cmdArgs = append(cmdArgs, "--asset", tmpIn)
				}
			} else if in.ImagePath != "" {
				return herness.ToolResult{
					Error:   "empty or invalid source image",
					Content: "image_path must point to an existing image in the sandbox when provided",
				}, nil
			}

			if in.DurationSec > 0 {
				cmdArgs = append(cmdArgs, "--duration", strconv.FormatFloat(in.DurationSec, 'f', -1, 64))
			}
			if in.FPS > 0 {
				cmdArgs = append(cmdArgs, "--fps", strconv.Itoa(in.FPS))
			}
			if in.Width > 0 {
				cmdArgs = append(cmdArgs, "--width", strconv.Itoa(in.Width))
			}
			if in.Height > 0 {
				cmdArgs = append(cmdArgs, "--height", strconv.Itoa(in.Height))
			}
			if os.Getenv("PICTUREBOOK_ANIMATE_DRY_RUN") == "1" {
				cmdArgs = append(cmdArgs, "--dry-run")
			}

			runCtx := ctx
			if _, hasDeadline := ctx.Deadline(); !hasDeadline {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithTimeout(ctx, 15*time.Minute)
				defer cancel()
			}

			cmd := exec.CommandContext(runCtx, node, cmdArgs...)
			cmd.Dir = filepath.Dir(script)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			runErr := cmd.Run()

			payload, parseErr := parsePictureBookJSON(stdout.String())
			if runErr != nil {
				msg := strings.TrimSpace(stderr.String())
				if payload != nil && payload["error"] != nil {
					msg = fmt.Sprint(payload["error"])
				}
				if msg == "" {
					msg = runErr.Error()
				}
				return herness.ToolResult{
					Error:   msg,
					Content: herness.FormatToolError(PictureBookAnimateToolName, fmt.Errorf("%s", msg)),
				}, nil
			}
			if parseErr != nil {
				return herness.ToolResult{
					Error:   parseErr.Error(),
					Content: herness.FormatToolError(PictureBookAnimateToolName, parseErr),
				}, nil
			}
			if ok, _ := payload["ok"].(bool); !ok {
				msg := fmt.Sprint(payload["error"])
				if msg == "" || msg == "<nil>" {
					msg = "picturebook animation failed"
				}
				return herness.ToolResult{Error: msg, Content: herness.FormatToolError(PictureBookAnimateToolName, fmt.Errorf("%s", msg))}, nil
			}

			hostPath, _ := payload["path"].(string)
			if hostPath == "" {
				hostPath = tmpOut
			}
			mp4, err := os.ReadFile(hostPath)
			if err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError(PictureBookAnimateToolName, err)}, nil
			}
			if err := writeSandboxBytes(sb, outVirt, mp4); err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError(PictureBookAnimateToolName, err)}, nil
			}

			label := "code"
			tplOut := ""
			if code == "" {
				label = "template:" + template
				tplOut = template
			}
			data := map[string]any{
				"path":         outVirt,
				"image_path":   inVirt,
				"bytes":        len(mp4),
				"prompt":       strings.TrimSpace(in.Prompt),
				"template":     tplOut,
				"has_code":     code != "",
				"duration_sec": payload["duration_sec"],
				"fps":          payload["fps"],
				"frames":       payload["frames"],
				"width":        payload["width"],
				"height":       payload["height"],
				"renderer":     payload["renderer"],
				"skill":        "picturebook-animate",
				"script":       script,
				"kind":         "video",
			}
			return herness.ToolResult{
				Content: fmt.Sprintf("ok: skill picturebook-animate ran %s → %s (%s, %dx%d, %vs) (%d bytes)",
					script, outVirt, label, toInt(payload["width"]), toInt(payload["height"]), payload["duration_sec"], len(mp4)),
				Data: data,
			}, nil
		},
	}
}

func pictureBookAnimateSkillDir(root string) string {
	return filepath.Join(root, "skills", "public", "picturebook-animate")
}

func resolvePictureBookAnimateScript(root string) string {
	candidates := []string{
		filepath.Join(pictureBookAnimateSkillDir(root), "scripts", "render.mjs"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func resolvePictureBookAnimateNode(root, skillDir string) string {
	skillScripts := filepath.Join(pictureBookAnimateSkillDir(root), "scripts")
	if strings.TrimSpace(skillDir) != "" {
		skillScripts = filepath.Join(skillDir, "scripts")
	}
	candidates := []string{
		os.Getenv("PICTUREBOOK_ANIMATE_NODE"),
		os.Getenv("NODE"),
		filepath.Join(skillScripts, ".node", "bin", "node"),
		"node",
	}
	for _, p := range candidates {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return "node"
}
