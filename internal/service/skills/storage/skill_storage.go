package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const skillMDFile = "SKILL.md"

type SkillStorage struct {
	ContainerPath string
}

func NewSkillStorage(containerPath string) *SkillStorage {
	return &SkillStorage{ContainerPath: containerPath}
}

func (s *SkillStorage) ValidateSkillName(name string) (string, error) {
	normalizedName := strings.TrimSpace(name)
	if normalizedName == "" {
		return "", fmt.Errorf("skill name is required")
	}
	if len(normalizedName) > 64 {
		return "", fmt.Errorf("skill name must be 64 characters or fewer")
	}
	return normalizedName, nil
}

func (s *SkillStorage) ValidateRelativePath(relativePath, basedir string) (string, error) {
	if relativePath == "" {
		return "", fmt.Errorf("relative_path must not be empty")
	}

	resolvedBase, err := filepath.EvalSymlinks(basedir)
	if err != nil {
		resolvedBase, err = filepath.Abs(basedir)
		if err != nil {
			return "", err
		}
		resolvedBase = filepath.Clean(resolvedBase)
	}

	joined := filepath.Join(resolvedBase, relativePath)
	target, err := filepath.EvalSymlinks(joined)
	if err != nil {
		target, err = filepath.Abs(joined)
		if err != nil {
			return "", err
		}
		target = filepath.Clean(target)
	}

	rel, err := filepath.Rel(resolvedBase, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("relative_path is not a subpath of basedir")
	}
	return target, nil
}

// ValidateSkillMarkdownContent checks basic skill name + writable SKILL.md layout.
// Frontmatter parsing can be plugged in later.
func (s *SkillStorage) ValidateSkillMarkdownContent(name, content string) error {
	normalized, err := s.ValidateSkillName(name)
	if err != nil {
		return err
	}
	tmpdir, err := os.MkdirTemp("", "skill-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	tempSkillDir := filepath.Join(tmpdir, normalized)
	if err := os.MkdirAll(tempSkillDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tempSkillDir, skillMDFile), []byte(content), 0o644); err != nil {
		return err
	}
	return nil
}
