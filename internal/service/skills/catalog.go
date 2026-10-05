package skills

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"movieagent/internal/service/config"

	"gopkg.in/yaml.v3"
)

const (
	CategoryPublic       = "public"
	CategoryCustom       = "custom"
	CategoryLegacy       = "legacy"
	CategoryIntegrations = "integrations"
)

// CatalogSkill is one skill discovered from the paths.go layout.
type CatalogSkill struct {
	Name        string
	Description string
	License     string
	Category    string
	Enabled     bool
	Editable    bool
	Dir         string
}

// LoadCatalog reads skills from the host layout in config.Paths.
//
//	skills_view/public/                         public, read-only
//	integrations/skills/{provider}/{skill}/     integrations, read-only
//	users/{userID}/skills/custom/               custom, editable
//	users/{userID}/skills_view/legacy/          legacy, read-only
//
// A later category with the same name replaces an earlier one, so a user's
// custom skill wins over a public skill of the same name.
func LoadCatalog(paths *config.Paths, userID string) ([]CatalogSkill, error) {
	if paths == nil {
		return nil, os.ErrInvalid
	}

	byName := map[string]CatalogSkill{}
	public, err := paths.PublicSkillsViewDir()
	if err != nil {
		return nil, err
	}
	if err := scanSkillDirs(public, CategoryPublic, false, byName); err != nil {
		return nil, err
	}

	if userID != "" {
		legacy, err := paths.UserLegacySkillsViewDir(userID)
		if err != nil {
			return nil, err
		}
		if err := scanSkillDirs(legacy, CategoryLegacy, false, byName); err != nil {
			return nil, err
		}
	}

	integrations, err := paths.IntegrationSkillsDir()
	if err != nil {
		return nil, err
	}
	if err := scanIntegrationSkills(integrations, byName); err != nil {
		return nil, err
	}

	if userID != "" {
		custom, err := paths.UserCustomSkillsDir(userID)
		if err != nil {
			return nil, err
		}
		if err := scanSkillDirs(custom, CategoryCustom, true, byName); err != nil {
			return nil, err
		}
	}

	out := make([]CatalogSkill, 0, len(byName))
	for _, skill := range byName {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func scanSkillDirs(root, category string, editable bool, byName map[string]CatalogSkill) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		skill, ok, err := readSkillDir(dir, entry.Name(), category, editable)
		if err != nil {
			return err
		}
		if ok {
			byName[skill.Name] = skill
		}
	}
	return nil
}

func scanIntegrationSkills(root string, byName map[string]CatalogSkill) error {
	providers, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, provider := range providers {
		if !provider.IsDir() || strings.HasPrefix(provider.Name(), ".") {
			continue
		}
		providerDir := filepath.Join(root, provider.Name())
		skills, err := os.ReadDir(providerDir)
		if err != nil {
			return err
		}
		for _, skillDir := range skills {
			if !skillDir.IsDir() || strings.HasPrefix(skillDir.Name(), ".") {
				continue
			}
			dir := filepath.Join(providerDir, skillDir.Name())
			skill, ok, err := readSkillDir(dir, skillDir.Name(), CategoryIntegrations, false)
			if err != nil {
				return err
			}
			if ok {
				byName[skill.Name] = skill
			}
		}
	}
	return nil
}

func readSkillDir(dir, fallbackName, category string, editable bool) (CatalogSkill, bool, error) {
	raw, err := os.ReadFile(filepath.Join(dir, skillMDFile))
	if err != nil {
		if os.IsNotExist(err) {
			return CatalogSkill{}, false, nil
		}
		return CatalogSkill{}, false, err
	}
	meta := parseFrontmatter(string(raw))
	name := strings.TrimSpace(meta["name"])
	if name == "" {
		name = fallbackName
	}
	return CatalogSkill{
		Name:        name,
		Description: strings.TrimSpace(meta["description"]),
		License:     strings.TrimSpace(meta["license"]),
		Category:    category,
		Enabled:     true,
		Editable:    editable,
		Dir:         dir,
	}, true, nil
}

func parseFrontmatter(content string) map[string]string {
	content = strings.TrimPrefix(content, "\uFEFF")
	if !strings.HasPrefix(content, "---") {
		return map[string]string{}
	}
	rest := content[3:]
	rest = strings.TrimPrefix(rest, "\r\n")
	rest = strings.TrimPrefix(rest, "\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return map[string]string{}
	}
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(rest[:end]), &raw); err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
