package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const DefaultSkillsContainerPath = "/mnt/skills"

// SkillsConfig mirrors deerflow.config.skills_config.SkillsConfig.
type SkillsConfig struct {
	// Class path of the SkillStorage implementation (Python reflection target).
	Use string `yaml:"use" json:"use"`

	// Path to skills directory. Empty = use env / project-root defaults.
	Path *string `yaml:"path,omitempty" json:"path,omitempty"`

	// Path where skills are mounted in the sandbox container.
	ContainerPath string `yaml:"container_path" json:"container_path"`

	// When true, only skill names go into <skill_index>; details via describe_skill.
	DeferredDiscovery bool `yaml:"deferred_discovery" json:"deferred_discovery"`
}

func DefaultSkillsConfig() SkillsConfig {
	return SkillsConfig{
		Use:               "deerflow.skills.storage.local_skill_storage:LocalSkillStorage",
		Path:              nil,
		ContainerPath:     DefaultSkillsContainerPath,
		DeferredDiscovery: false,
	}
}

// legacySkillsCandidates returns source-tree skills locations for monorepo compatibility.
func legacySkillsCandidates() []string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil
	}
	backendDir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	repoRoot := filepath.Dir(backendDir)
	return []string{filepath.Join(repoRoot, "skills")}
}

func (c SkillsConfig) GetSkillsPath(projectRoot string, resolvePath func(string) (string, error)) (string, error) {
	if resolvePath == nil {
		resolvePath = func(p string) (string, error) {
			if filepath.IsAbs(p) {
				return filepath.Clean(p), nil
			}
			return filepath.Abs(filepath.Join(projectRoot, p))
		}
	}

	if c.Path != nil && *c.Path != "" {
		return resolvePath(*c.Path)
	}
	if envPath := os.Getenv("DEER_FLOW_SKILLS_PATH"); envPath != "" {
		return resolvePath(envPath)
	}

	projectDefault := filepath.Join(projectRoot, "skills")
	if fi, err := os.Stat(projectDefault); err == nil && fi.IsDir() {
		return projectDefault, nil
	}

	for _, candidate := range legacySkillsCandidates() {
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate, nil
		}
	}

	return projectDefault, nil
}

// GetSkillContainerPath returns the full container path for a skill.
func (c SkillsConfig) GetSkillContainerPath(skillName, category string) string {
	container := c.ContainerPath
	if container == "" {
		container = DefaultSkillsContainerPath
	}
	if category == "" {
		return filepath.ToSlash(filepath.Join(container, skillName))
	}
	return filepath.ToSlash(filepath.Join(container, category, skillName))
}
