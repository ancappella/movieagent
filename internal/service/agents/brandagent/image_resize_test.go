package brandagent

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"testing"

	"movieagent/internal/service/config"
	"movieagent/internal/service/sandbox"
)

func TestImageResizeTool(t *testing.T) {
	base := t.TempDir()
	paths, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}
	provider := sandbox.NewLocalProvider(paths)
	sbID, err := provider.Acquire("t-resize", "alice")
	if err != nil {
		t.Fatal(err)
	}
	sb, ok := provider.Get(sbID)
	if !ok {
		t.Fatal("sandbox not cached")
	}

	src := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			src.Set(x, y, color.RGBA{R: 200, G: 80, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	inPath := "/mnt/user-data/uploads/images/src.png"
	outPath := "/mnt/user-data/outputs/out.png"
	if err := writeSandboxBytes(sb, inPath, buf.Bytes()); err != nil {
		t.Fatal(err)
	}

	tool := imageResizeTool(sb)
	args, _ := json.Marshal(map[string]any{
		"image_path":  inPath,
		"width":       10,
		"height":      8,
		"output_path": outPath,
	})
	res, err := tool.Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}
	if res.Data["width"] != 10 || res.Data["height"] != 8 {
		t.Fatalf("data = %#v", res.Data)
	}

	raw, err := readSandboxBytes(sb, outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	img, err := decodeImage(raw)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 10 || b.Dy() != 8 {
		t.Fatalf("got size %dx%d", b.Dx(), b.Dy())
	}
}
