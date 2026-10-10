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

func TestRegisterIncludesPictureBook(t *testing.T) {
	root, err := config.ProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	list := tools.Register(tools.Options{ProjectRoot: root})
	if len(list) == 0 {
		t.Fatal("expected at least one tool")
	}
	found := false
	for _, tool := range list {
		if tool.Name() == tools.PictureBookToolName {
			found = true
			sch := tool.Schema()
			if sch.Description == "" {
				t.Fatal("empty description")
			}
			if !strings.Contains(sch.Description, "picturebook-illustrate") {
				t.Fatalf("tool should bind skill picturebook-illustrate, got %q", sch.Description)
			}
			if !strings.Contains(sch.Description, "Canvas") && !strings.Contains(sch.Description, "draw.mjs") {
				t.Fatalf("tool should mention Canvas/draw.mjs, got %q", sch.Description)
			}
		}
	}
	if !found {
		t.Fatalf("missing %s", tools.PictureBookToolName)
	}
}

func TestBootstrapListSchemas(t *testing.T) {
	tools.Bootstrap(t.TempDir(), "")
	schemas := tools.ListSchemas()
	found := false
	for _, s := range schemas {
		if s.Name == tools.PictureBookToolName {
			found = true
		}
	}
	if !found {
		t.Fatalf("Bootstrap missing %s", tools.PictureBookToolName)
	}
}

func TestPictureBookIllustratorDryRun(t *testing.T) {
	root, err := config.ProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "skills", "public", "picturebook-illustrate", "scripts", "draw.mjs")
	if _, err := os.Stat(script); err != nil {
		t.Skip("draw.mjs not present")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(script), "node_modules")); err != nil {
		t.Skip("picturebook-illustrate node_modules missing; run npm install in scripts/")
	}

	t.Setenv("PICTUREBOOK_DRY_RUN", "1")
	t.Setenv("DEER_FLOW_HOME", t.TempDir())

	paths, err := config.NewPaths(nil)
	if err != nil {
		t.Fatal(err)
	}
	provider := sandbox.NewLocalProvider(paths)
	id, err := provider.Acquire("pb-test", "default")
	if err != nil {
		t.Fatal(err)
	}
	sb, ok := provider.Get(id)
	if !ok {
		t.Fatal("sandbox missing")
	}

	tool := tools.NewPictureBookIllustrator(tools.Options{
		ProjectRoot: root,
		Sandbox:     sb,
	})
	args, _ := json.Marshal(map[string]any{
		"prompt": "a tiny rabbit holding a lantern in the forest",
		"seed":   7,
	})
	res, err := tool.Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s / %s", res.Error, res.Content)
	}
	path, _ := res.Data["path"].(string)
	if path != tools.PictureBookDefaultOut {
		t.Fatalf("path = %q", path)
	}
	bytesN, _ := res.Data["bytes"].(int)
	if bytesN < 32 {
		t.Fatalf("expected png bytes in sandbox, got %d (%s)", bytesN, res.Content)
	}
	usedScript, _ := res.Data["script"].(string)
	if !strings.Contains(usedScript, filepath.Join("skills", "public", "picturebook-illustrate", "scripts", "draw.mjs")) {
		t.Fatalf("expected skill script path, got %q (%s)", usedScript, res.Content)
	}
	if model, _ := res.Data["model"].(string); model != "" && model != "canvas-2d" {
		// dry-run still reports canvas-2d
		if model != "canvas-2d" {
			t.Fatalf("model=%q", model)
		}
	}
}
