package context

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Skill 定义了从 SKILL.md 中解析出的标准化技能结构
type Skill struct {
	Name        string
	Description string
	Body        string // Markdown 正文指令
	Dir         string // skill 根目录（含 SKILL.md / scripts / references / assets）
	Tool        string // 可选：对外暴露的 tool 名（frontmatter tool:）
	Script      string // 可选：相对 Dir 的入口脚本（frontmatter script:），如 scripts/generate.py
}

// ScriptPath 返回 skill 入口脚本的绝对路径；未配置或文件不存在时返回空串。
func (sk Skill) ScriptPath() string {
	rel := strings.TrimSpace(sk.Script)
	if rel == "" || strings.TrimSpace(sk.Dir) == "" {
		return ""
	}
	p := filepath.Join(sk.Dir, filepath.FromSlash(rel))
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// SkillLoader 负责从本地文件系统中加载并解析符合规范的技能模板
type SkillLoader struct {
	workDir string
}

func NewSkillLoader(workDir string) *SkillLoader {
	return &SkillLoader{workDir: workDir}
}

func (s *SkillLoader) defaultDirs() []string {
	return []string{
		filepath.Join(s.workDir, ".claw", "skills"),
		filepath.Join(s.workDir, "skills", "public"),
	}
}

// List 返回默认目录下的全部 skill（含 Dir / tool / script 元数据）。
func (s *SkillLoader) List() []Skill {
	return s.ListFromDirs(s.defaultDirs()...)
}

// Find 按 name 查找 skill。
func (s *SkillLoader) Find(name string) (Skill, bool) {
	name = strings.TrimSpace(name)
	for _, sk := range s.List() {
		if sk.Name == name {
			return sk, true
		}
	}
	return Skill{}, false
}

// ListFromDirs 扫描给定目录中的 SKILL.md。
func (s *SkillLoader) ListFromDirs(dirs ...string) []Skill {
	var out []Skill
	seen := map[string]bool{}
	for _, skillBaseDir := range dirs {
		skillBaseDir = strings.TrimSpace(skillBaseDir)
		if skillBaseDir == "" {
			continue
		}
		if _, err := os.Stat(skillBaseDir); os.IsNotExist(err) {
			continue
		}
		_ = filepath.WalkDir(skillBaseDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
				return err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			sk := parseSkillMD(string(raw))
			sk.Dir = filepath.Dir(path)
			if sk.Name == "" || sk.Name == "Unknown Skill" {
				sk.Name = filepath.Base(sk.Dir)
			}
			if seen[sk.Name] {
				return nil
			}
			seen[sk.Name] = true
			out = append(out, sk)
			return nil
		})
	}
	return out
}

// LoadAll 扫描默认技能目录，解析 SKILL.md 并格式化为可注入 Context 的文本
func (s *SkillLoader) LoadAll() string {
	return s.FormatSkills(s.List())
}

// LoadFromDirs 扫描给定目录中的 SKILL.md，并格式化为字符串准备注入 Context
func (s *SkillLoader) LoadFromDirs(dirs ...string) string {
	return s.FormatSkills(s.ListFromDirs(dirs...))
}

// FormatSkills 将 skill 列表格式化为可注入 system context 的 Markdown。
func (s *SkillLoader) FormatSkills(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### 可用专业技能 (Agent Skills)\n")
	b.WriteString("以下是你拥有的标准化外挂技能，请在符合 description 描述的场景下严格遵循其正文指令。\n")
	b.WriteString("需要执行能力时，调用技能声明的 tool；tool 会运行该 skill 目录下 scripts/ 中的脚本。\n\n")

	for _, skill := range skills {
		b.WriteString(fmt.Sprintf("#### 技能名称: %s\n", skill.Name))
		b.WriteString(fmt.Sprintf("**触发条件**: %s\n\n", skill.Description))
		if tool := strings.TrimSpace(skill.Tool); tool != "" {
			b.WriteString(fmt.Sprintf("**绑定工具**: `%s`\n", tool))
		}
		if script := strings.TrimSpace(skill.Script); script != "" {
			b.WriteString(fmt.Sprintf("**入口脚本**: `%s`\n", script))
		}
		if tool := strings.TrimSpace(skill.Tool); tool != "" || strings.TrimSpace(skill.Script) != "" {
			b.WriteString("\n")
		}
		b.WriteString("**执行指南**:\n")
		b.WriteString(skill.Body)
		b.WriteString("\n\n---\n")
	}
	return strings.TrimSpace(b.String())
}

// parseSkillMD 极简解析带有 YAML Frontmatter 的 Markdown 内容
func parseSkillMD(content string) Skill {
	skill := Skill{
		Name:        "Unknown Skill",
		Description: "No description provided.",
		Body:        content, // 默认将全量内容作为 body
	}

	// 简单解析 YAML Frontmatter (以 --- 包裹)
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) == 3 {
			frontmatter := parts[1]
			skill.Body = strings.TrimSpace(parts[2])

			lines := strings.Split(frontmatter, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				switch {
				case strings.HasPrefix(line, "name:"):
					skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				case strings.HasPrefix(line, "description:"):
					skill.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				case strings.HasPrefix(line, "tool:"):
					skill.Tool = strings.TrimSpace(strings.TrimPrefix(line, "tool:"))
				case strings.HasPrefix(line, "script:"):
					skill.Script = strings.TrimSpace(strings.TrimPrefix(line, "script:"))
				}
			}
		}
	}

	return skill
}
