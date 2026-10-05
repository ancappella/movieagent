package skills

import (
	"fmt"

	"movieagent/internal/service/herness"
)

const skillMDFile = "SKILL.md"

type Skill struct {
	Name               string
	Description        string
	License            string
	SkillDir           string
	SkillFile          string
	RelativePath       string
	Category           string
	AllowedTools       []string
	Enable             bool
	RequiredSecrets    []string
	SecretsAutonomous  bool
}

func (s *Skill) SkillPath() string {
	path := s.RelativePath
	if path == "." {
		return ""
	}
	return path
}

func (s *Skill) GetContainerPath(containerPath string) string {
	if containerPath == "" {
		containerPath = herness.DefaultSkillsContainerPath
	}
	categoryBase := fmt.Sprintf("%s/%s", containerPath, s.Category)
	skillPath := s.SkillPath()
	if skillPath == "" {
		return fmt.Sprintf("%s/%s", categoryBase, s.Name)
	}
	return fmt.Sprintf("%s/%s", categoryBase, skillPath)
}

func (s *Skill) GetContainerFilePath(containerPath string) string {
	if containerPath == "" {
		containerPath = herness.DefaultSkillsContainerPath
	}
	return fmt.Sprintf("%s/%s/%s", containerPath, s.Category, skillMDFile)
}

func (s Skill) String() string {
	return fmt.Sprintf(
		"Skill(name=%q, description=%q, category=%q)",
		s.Name,
		s.Description,
		s.Category,
	)
}
