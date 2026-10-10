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
	PictureBookToolName   = "picturebook_illustrate"
	PictureBookDefaultOut = "/mnt/user-data/outputs/picturebook.png"
)

// NewPictureBookIllustrator builds the Canvas 2D picture-book illustration tool.
// Prefer binding via skill picturebook-illustrate (scripts/draw.mjs).
func NewPictureBookIllustrator(opts Options) herness.Tool {
	root := strings.TrimSpace(opts.ProjectRoot)
	sb := opts.Sandbox
	skillScript := strings.TrimSpace(opts.SkillScript)
	skillDir := strings.TrimSpace(opts.SkillDir)

	return herness.ToolFunc{
		ToolName: PictureBookToolName,
		ToolDesc: "Skill picturebook-illustrate: run its scripts/draw.mjs to paint a children's watercolor/colored-pencil style illustration with Canvas 2D (no SD/LoRA). Prefer aspect_ratio for size; saves a PNG into the sandbox.",
		Params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Scene / character description (Chinese or English). Canvas picks a theme from keywords (sea/fox/flowers/...).",
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Sandbox virtual path for PNG; default /mnt/user-data/outputs/picturebook.png",
				},
				"aspect_ratio": map[string]any{
					"type": "string",
					"enum": []string{"1:1", "3:4", "4:3", "16:9", "9:16", "2:3", "21:9"},
					"description": "Image size preset by scene. " +
						"1:1 头像/图标/方形海报; " +
						"3:4 竖版人像/手机壁纸/书籍封面; " +
						"4:3 传统照片/PPT; " +
						"16:9 横版视频封面/桌面壁纸; " +
						"9:16 短视频/Story; " +
						"2:3 摄影/明信片/绘本插页(默认); " +
						"21:9 电影宽银幕.",
				},
				"width": map[string]any{
					"type":        "integer",
					"description": "Optional width px override; must be set together with height (overrides aspect_ratio)",
				},
				"height": map[string]any{
					"type":        "integer",
					"description": "Optional height px override; must be set together with width (overrides aspect_ratio)",
				},
				"seed": map[string]any{"type": "integer", "description": "Optional RNG seed for Canvas painting"},
			},
			"required": []string{"prompt"},
		},
		Fn: func(ctx context.Context, args json.RawMessage) (herness.ToolResult, error) {
			var in struct {
				Prompt      string `json:"prompt"`
				OutputPath  string `json:"output_path"`
				AspectRatio string `json:"aspect_ratio"`
				Width       int    `json:"width"`
				Height      int    `json:"height"`
				Seed        *int   `json:"seed"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError(PictureBookToolName, err)}, nil
			}
			prompt := strings.TrimSpace(in.Prompt)
			if prompt == "" {
				return herness.ToolResult{Error: "empty prompt", Content: "prompt is required"}, nil
			}
			outVirt := strings.TrimSpace(in.OutputPath)
			if outVirt == "" {
				outVirt = PictureBookDefaultOut
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
				script = resolvePictureBookScript(projRoot)
			}
			if script == "" {
				return herness.ToolResult{
					Error:   "picturebook skill script missing",
					Content: herness.FormatToolError(PictureBookToolName, fmt.Errorf("skill picturebook-illustrate scripts/draw.mjs missing")),
				}, nil
			}

			node := resolvePictureBookNode(projRoot, skillDir)
			tmpDir, err := os.MkdirTemp("", "picturebook-*")
			if err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError(PictureBookToolName, err)}, nil
			}
			defer os.RemoveAll(tmpDir)
			tmpOut := filepath.Join(tmpDir, "out.png")

			cmdArgs := []string{
				script,
				"--prompt", prompt,
				"--output", tmpOut,
			}
			if ar := strings.TrimSpace(in.AspectRatio); ar != "" {
				cmdArgs = append(cmdArgs, "--aspect-ratio", ar)
			}
			if in.Width > 0 && in.Height > 0 {
				cmdArgs = append(cmdArgs, "--width", strconv.Itoa(in.Width), "--height", strconv.Itoa(in.Height))
			} else if in.Width > 0 || in.Height > 0 {
				return herness.ToolResult{
					Error:   "width and height must both be set",
					Content: "set both width and height, or use aspect_ratio alone",
				}, nil
			}
			if in.Seed != nil {
				cmdArgs = append(cmdArgs, "--seed", strconv.Itoa(*in.Seed))
			}
			if os.Getenv("PICTUREBOOK_DRY_RUN") == "1" {
				cmdArgs = append(cmdArgs, "--dry-run")
			}

			runCtx := ctx
			if _, hasDeadline := ctx.Deadline(); !hasDeadline {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithTimeout(ctx, 2*time.Minute)
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
					Content: herness.FormatToolError(PictureBookToolName, fmt.Errorf("%s", msg)),
				}, nil
			}
			if parseErr != nil {
				return herness.ToolResult{
					Error:   parseErr.Error(),
					Content: herness.FormatToolError(PictureBookToolName, parseErr),
				}, nil
			}
			if ok, _ := payload["ok"].(bool); !ok {
				msg := fmt.Sprint(payload["error"])
				if msg == "" || msg == "<nil>" {
					msg = "picturebook generation failed"
				}
				return herness.ToolResult{Error: msg, Content: herness.FormatToolError(PictureBookToolName, fmt.Errorf("%s", msg))}, nil
			}

			hostPath, _ := payload["path"].(string)
			if hostPath == "" {
				hostPath = tmpOut
			}
			png, err := os.ReadFile(hostPath)
			if err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError(PictureBookToolName, err)}, nil
			}
			if err := writeSandboxBytes(sb, outVirt, png); err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError(PictureBookToolName, err)}, nil
			}

			data := map[string]any{
				"path":         outVirt,
				"bytes":        len(png),
				"seed":         payload["seed"],
				"model":        payload["model"],
				"device":       payload["device"],
				"theme":        payload["theme"],
				"aspect_ratio": payload["aspect_ratio"],
				"width":        payload["width"],
				"height":       payload["height"],
				"skill":        "picturebook-illustrate",
				"script":       script,
			}
			return herness.ToolResult{
				Content: fmt.Sprintf("ok: skill picturebook-illustrate ran %s → %s (%dx%d, %v) (%d bytes)",
					script, outVirt, toInt(payload["width"]), toInt(payload["height"]), payload["aspect_ratio"], len(png)),
				Data: data,
			}, nil
		},
	}
}

func pictureBookSkillDir(root string) string {
	return filepath.Join(root, "skills", "public", "picturebook-illustrate")
}

func resolvePictureBookScript(root string) string {
	candidates := []string{
		filepath.Join(pictureBookSkillDir(root), "scripts", "draw.mjs"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func resolvePictureBookNode(root, skillDir string) string {
	skillScripts := filepath.Join(pictureBookSkillDir(root), "scripts")
	if strings.TrimSpace(skillDir) != "" {
		skillScripts = filepath.Join(skillDir, "scripts")
	}
	candidates := []string{
		os.Getenv("PICTUREBOOK_NODE"),
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

func parsePictureBookJSON(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	line := raw
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "{") {
			line = l
			break
		}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		return nil, fmt.Errorf("parse picturebook json: %w (raw=%q)", err, truncate(line, 240))
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}
