package skills

import (
	"os"
	"path/filepath"
	"testing"

	"movieagent/internal/service/config"
)

func TestLoadCatalog(t *testing.T) {
	base := t.TempDir()
	p, err := config.NewPaths(&base)
	if err != nil {
		t.Fatal(err)
	}

	writeSkill(t, filepath.Join(base, "skills_view", "public", "public-skill"), "public-skill", "public desc", "Apache-2.0")
	writeSkill(t, filepath.Join(base, "integrations", "skills", "github", "issues"), "issues", "integration desc", "")
	writeSkill(t, filepath.Join(base, "users", "alice", "skills", "custom", "my-skill"), "my-skill", "custom desc", "MIT")
	writeSkill(t, filepath.Join(base, "users", "alice", "skills_view", "legacy", "old-skill"), "old-skill", "legacy desc", "")
	// custom skill with the same name replaces public
	writeSkill(t, filepath.Join(base, "users", "alice", "skills", "custom", "public-skill"), "public-skill", "override", "MIT")

	got, err := LoadCatalog(p, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, skills = %#v", len(got), got)
	}

	byName := map[string]CatalogSkill{}
	for _, s := range got {
		byName[s.Name] = s
	}
	if s := byName["public-skill"]; s.Category != CategoryCustom || !s.Editable || s.Description != "override" {
		t.Fatalf("override = %#v", s)
	}
	if s := byName["issues"]; s.Category != CategoryIntegrations || s.Editable {
		t.Fatalf("integration = %#v", s)
	}
	if s := byName["old-skill"]; s.Category != CategoryLegacy || s.Editable {
		t.Fatalf("legacy = %#v", s)
	}
	if s := byName["my-skill"]; s.License != "MIT" || !s.Enabled {
		t.Fatalf("custom = %#v", s)
	}
}

func writeSkill(t *testing.T, dir, name, desc, license string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + desc + "\n"
	if license != "" {
		body += "license: " + license + "\n"
	}
	body += "---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
