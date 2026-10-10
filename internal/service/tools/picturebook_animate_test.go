package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"movieagent/internal/service/config"
	"movieagent/internal/service/sandbox"
	"movieagent/internal/service/tools"
)

func TestRegisterIncludesPictureBookAnimate(t *testing.T) {
	root, err := config.ProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	list := tools.Register(tools.Options{ProjectRoot: root})
	found := false
	for _, tool := range list {
		if tool.Name() == tools.PictureBookAnimateToolName {
			found = true
			sch := tool.Schema()
			if !strings.Contains(sch.Description, "picturebook-animate") {
				t.Fatalf("tool should bind skill picturebook-animate, got %q", sch.Description)
			}
			if !strings.Contains(sch.Description, "render.mjs") {
				t.Fatalf("tool should mention render.mjs, got %q", sch.Description)
			}
		}
	}
	if !found {
		t.Fatalf("missing %s", tools.PictureBookAnimateToolName)
	}
}

func TestPictureBookAnimatorDryRun(t *testing.T) {
	root, err := config.ProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "skills", "public", "picturebook-animate", "scripts", "render.mjs")
	if _, err := os.Stat(script); err != nil {
		t.Skip("render script not present")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(script), "node_modules")); err != nil {
		t.Skip("picturebook-animate node_modules missing; run npm install in scripts/")
	}

	t.Setenv("PICTUREBOOK_ANIMATE_DRY_RUN", "1")
	t.Setenv("DEER_FLOW_HOME", t.TempDir())

	paths, err := config.NewPaths(nil)
	if err != nil {
		t.Fatal(err)
	}
	provider := sandbox.NewLocalProvider(paths)
	id, err := provider.Acquire("pb-anim-test", "default")
	if err != nil {
		t.Fatal(err)
	}
	sb, ok := provider.Get(id)
	if !ok {
		t.Fatal("sandbox missing")
	}

	// Minimal PNG so the tool accepts the optional asset.
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
	if err := sb.WriteFile(tools.PictureBookAnimateDefaultIn, string(png), false); err != nil {
		t.Fatal(err)
	}

	tool := tools.NewPictureBookAnimator(tools.Options{
		ProjectRoot: root,
		Sandbox:     sb,
	})
	args, _ := json.Marshal(map[string]any{
		"template":     "character_spin",
		"duration_sec": 2,
		"prompt":       "玩偶原地旋转",
	})
	res, err := tool.Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s / %s", res.Error, res.Content)
	}
	path, _ := res.Data["path"].(string)
	if path != tools.PictureBookAnimateDefaultOut {
		t.Fatalf("path = %q", path)
	}
	bytesN, _ := res.Data["bytes"].(int)
	if bytesN < 16 {
		t.Fatalf("expected mp4 bytes in sandbox, got %d (%s)", bytesN, res.Content)
	}
	usedScript, _ := res.Data["script"].(string)
	if !strings.Contains(usedScript, filepath.Join("skills", "public", "picturebook-animate", "scripts", "render.mjs")) {
		t.Fatalf("expected skill script path, got %q (%s)", usedScript, res.Content)
	}
}
