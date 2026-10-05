package brandagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"movieagent/internal/service/herness"
	"movieagent/internal/service/sandbox"
)

func sandboxTools(sb sandbox.Sandbox) []herness.Tool {
	return []herness.Tool{
		herness.BashTool{Sandbox: sb},
		readFileTool{sb: sb},
		writeFileTool{sb: sb},
		listDirTool{sb: sb},
	}
}

type readFileTool struct{ sb sandbox.Sandbox }

func (readFileTool) Name() string { return "read_file" }

func (t readFileTool) Schema() herness.ToolSchema {
	return herness.ToolSchema{
		Name:        "read_file",
		Description: "Read a file from the LocalSandbox using a virtual path (e.g. /mnt/user-data/workspace/...).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       map[string]any{"type": "string"},
				"start_line": map[string]any{"type": "integer", "description": "1-based start line; 0 = from beginning"},
				"end_line":   map[string]any{"type": "integer", "description": "1-based end line; 0 = to end"},
			},
			"required": []string{"path"},
		},
	}
}

func (t readFileTool) Run(_ context.Context, args json.RawMessage) (herness.ToolResult, error) {
	var in struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("read_file", err)}, nil
	}
	body, err := t.sb.ReadFile(in.Path, in.StartLine, in.EndLine)
	if err != nil {
		return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("read_file", err)}, nil
	}
	return herness.ToolResult{Content: body}, nil
}

type writeFileTool struct{ sb sandbox.Sandbox }

func (writeFileTool) Name() string { return "write_file" }

func (t writeFileTool) Schema() herness.ToolSchema {
	return herness.ToolSchema{
		Name:        "write_file",
		Description: "Write text into LocalSandbox. Use virtual paths under /mnt/user-data/workspace or /mnt/user-data/outputs.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
				"append":  map[string]any{"type": "boolean"},
			},
			"required": []string{"path", "content"},
		},
	}
}

func (t writeFileTool) Run(_ context.Context, args json.RawMessage) (herness.ToolResult, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Append  bool   `json:"append"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("write_file", err)}, nil
	}
	if err := t.sb.WriteFile(in.Path, in.Content, in.Append); err != nil {
		return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("write_file", err)}, nil
	}
	return herness.ToolResult{Content: "ok: wrote " + in.Path}, nil
}

type listDirTool struct{ sb sandbox.Sandbox }

func (listDirTool) Name() string { return "list_dir" }

func (t listDirTool) Schema() herness.ToolSchema {
	return herness.ToolSchema{
		Name:        "list_dir",
		Description: "List files under a LocalSandbox virtual path.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":      map[string]any{"type": "string"},
				"max_depth": map[string]any{"type": "integer"},
			},
			"required": []string{"path"},
		},
	}
}

func (t listDirTool) Run(_ context.Context, args json.RawMessage) (herness.ToolResult, error) {
	var in struct {
		Path     string `json:"path"`
		MaxDepth int    `json:"max_depth"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("list_dir", err)}, nil
	}
	entries, err := t.sb.ListDir(in.Path, in.MaxDepth)
	if err != nil {
		return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("list_dir", err)}, nil
	}
	return herness.ToolResult{Content: strings.Join(entries, "\n")}, nil
}

func generateImageTool(gen ImageGenerator, sb sandbox.Sandbox) herness.Tool {
	return herness.ToolFunc{
		ToolName: "generate_image",
		ToolDesc: "Generate a marketing poster from a prompt and optional beverage reference image. Saves to LocalSandbox outputs (default /mnt/user-data/outputs/brand-campaign.png).",
		Params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt":         map[string]any{"type": "string"},
				"ref_image_path": map[string]any{"type": "string", "description": "Virtual path to beverage reference image"},
				"output_path":    map[string]any{"type": "string"},
			},
			"required": []string{"prompt"},
		},
		Fn: func(ctx context.Context, args json.RawMessage) (herness.ToolResult, error) {
			var in struct {
				Prompt       string `json:"prompt"`
				RefImagePath string `json:"ref_image_path"`
				OutputPath   string `json:"output_path"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("generate_image", err)}, nil
			}
			if strings.TrimSpace(in.Prompt) == "" {
				return herness.ToolResult{Error: "empty prompt", Content: "prompt is required"}, nil
			}
			outPath := in.OutputPath
			if outPath == "" {
				outPath = BrandToneOutputPath
			}
			req := GenerateRequest{Prompt: in.Prompt}
			if p := strings.TrimSpace(in.RefImagePath); p != "" {
				data, err := readSandboxBytes(sb, p)
				if err != nil {
					return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("generate_image", err)}, nil
				}
				req.RefImage = data
				req.RefMIME = sniffMIME(data)
			}
			png, err := gen.Generate(ctx, req)
			if err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("generate_image", err)}, nil
			}
			if err := writeSandboxBytes(sb, outPath, png); err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("generate_image", err)}, nil
			}
			return herness.ToolResult{
				Content: fmt.Sprintf("ok: wrote %s (%s bytes)", outPath, strconv.Itoa(len(png))),
				Data:    map[string]any{"path": outPath, "bytes": len(png)},
			}, nil
		},
	}
}

func imageResizeTool(sb sandbox.Sandbox) herness.Tool {
	return herness.ToolFunc{
		ToolName: "image_resize",
		ToolDesc: "Resize an image to the specified width and height. Reads/writes LocalSandbox virtual paths; output is PNG.",
		Params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"image_path": map[string]any{
					"type":        "string",
					"description": "Virtual path to source image (PNG/JPEG)",
				},
				"width": map[string]any{
					"type":        "integer",
					"description": "Target width in pixels (>0)",
				},
				"height": map[string]any{
					"type":        "integer",
					"description": "Target height in pixels (>0)",
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Virtual path for output PNG; default /mnt/user-data/outputs/resized.png",
				},
			},
			"required": []string{"image_path", "width", "height"},
		},
		Fn: func(_ context.Context, args json.RawMessage) (herness.ToolResult, error) {
			var in struct {
				ImagePath  string `json:"image_path"`
				Width      int    `json:"width"`
				Height     int    `json:"height"`
				OutputPath string `json:"output_path"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("image_resize", err)}, nil
			}
			srcPath := strings.TrimSpace(in.ImagePath)
			if srcPath == "" {
				return herness.ToolResult{Error: "empty image_path", Content: "image_path is required"}, nil
			}
			if in.Width <= 0 || in.Height <= 0 {
				return herness.ToolResult{Error: "invalid size", Content: "width and height must be > 0"}, nil
			}
			outPath := strings.TrimSpace(in.OutputPath)
			if outPath == "" {
				outPath = "/mnt/user-data/outputs/resized.png"
			}

			raw, err := readSandboxBytes(sb, srcPath)
			if err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("image_resize", err)}, nil
			}
			src, err := decodeImage(raw)
			if err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("image_resize", err)}, nil
			}
			pngBytes, err := encodePNG(resizeNearest(src, in.Width, in.Height))
			if err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("image_resize", err)}, nil
			}
			if err := writeSandboxBytes(sb, outPath, pngBytes); err != nil {
				return herness.ToolResult{Error: err.Error(), Content: herness.FormatToolError("image_resize", err)}, nil
			}
			b := src.Bounds()
			return herness.ToolResult{
				Content: fmt.Sprintf("ok: resized %dx%d → %dx%d, wrote %s (%s bytes)",
					b.Dx(), b.Dy(), in.Width, in.Height, outPath, strconv.Itoa(len(pngBytes))),
				Data: map[string]any{
					"path":          outPath,
					"bytes":         len(pngBytes),
					"width":         in.Width,
					"height":        in.Height,
					"source_width":  b.Dx(),
					"source_height": b.Dy(),
				},
			}, nil
		},
	}
}
