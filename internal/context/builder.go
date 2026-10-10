package context

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed systemprompt.md
var embeddedSystemPrompt string

// ToolInfo is a lightweight tool descriptor for prompt assembly.
// Keep this free of herness/claw imports so context stays a leaf package.
type ToolInfo struct {
	Name        string
	Description string
}

// Builder assembles system prompt + skills + tools into one context string.
type Builder struct {
	workDir          string
	systemPromptPath string
	systemPrompt     string
	skillDirs        []string
	tools            []ToolInfo
	extras           []string
}

// NewBuilder creates a context builder rooted at workDir (usually project root).
func NewBuilder(workDir string) *Builder {
	return &Builder{workDir: strings.TrimSpace(workDir)}
}

// WithSystemPromptPath overrides the on-disk system prompt file.
// Default: <workDir>/internal/context/systemprompt.md
func (b *Builder) WithSystemPromptPath(path string) *Builder {
	b.systemPromptPath = strings.TrimSpace(path)
	return b
}

// WithSystemPrompt sets the system prompt text directly (skips file load).
func (b *Builder) WithSystemPrompt(text string) *Builder {
	b.systemPrompt = text
	return b
}

// WithSkillDirs overrides skill scan roots (absolute or relative to workDir).
// Default: .claw/skills and skills/public under workDir.
func (b *Builder) WithSkillDirs(dirs ...string) *Builder {
	b.skillDirs = append([]string(nil), dirs...)
	return b
}

// WithTools attaches tool summaries for the Tools section.
func (b *Builder) WithTools(tools ...ToolInfo) *Builder {
	b.tools = append([]ToolInfo(nil), tools...)
	return b
}

// WithExtra appends extra markdown sections after skills/tools.
func (b *Builder) WithExtra(sections ...string) *Builder {
	b.extras = append(b.extras, sections...)
	return b
}

// Phase selects which sections the builder includes.
type Phase int

const (
	// PhaseThinking is slow-thinking context: system (+ extras) only — no skills/tools.
	PhaseThinking Phase = iota
	// PhaseAction is the action / tool-use context: system + skills + tools (+ extras).
	PhaseAction
)

// Build assembles the full Action-phase context (system + skills + tools).
func (b *Builder) Build() string {
	return b.BuildPhase(PhaseAction)
}

// BuildThinking assembles the first slow-thinking context without skills/tools.
func (b *Builder) BuildThinking() string {
	return b.BuildPhase(PhaseThinking)
}

// BuildAction assembles the action context with skills and tools.
func (b *Builder) BuildAction() string {
	return b.BuildPhase(PhaseAction)
}

// BuildPhase assembles context for the given loop phase.
func (b *Builder) BuildPhase(phase Phase) string {
	if b == nil {
		return strings.TrimSpace(embeddedSystemPrompt)
	}

	var out strings.Builder

	sys := b.loadSystemPrompt()
	if sys != "" {
		out.WriteString(sys)
		out.WriteString("\n")
	}

	// Skills / tools only enter the Action phase — first Thinking stays lean.
	if phase == PhaseAction {
		if skills := b.loadSkills(); skills != "" {
			out.WriteString("\n")
			out.WriteString(skills)
			out.WriteString("\n")
		}
		if tools := formatTools(b.tools); tools != "" {
			out.WriteString("\n")
			out.WriteString(tools)
			out.WriteString("\n")
		}
	}

	for _, extra := range b.extras {
		extra = strings.TrimSpace(extra)
		if extra == "" {
			continue
		}
		out.WriteString("\n")
		out.WriteString(extra)
		out.WriteString("\n")
	}

	return strings.TrimSpace(out.String())
}

func (b *Builder) loadSystemPrompt() string {
	if strings.TrimSpace(b.systemPrompt) != "" {
		return strings.TrimSpace(b.systemPrompt)
	}

	path := b.systemPromptPath
	if path == "" && b.workDir != "" {
		path = filepath.Join(b.workDir, "internal", "context", "systemprompt.md")
	}
	if path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			if text := strings.TrimSpace(string(raw)); text != "" {
				return text
			}
		}
	}
	return strings.TrimSpace(embeddedSystemPrompt)
}

func (b *Builder) loadSkills() string {
	dirs := b.skillDirs
	if len(dirs) == 0 && b.workDir != "" {
		dirs = []string{
			filepath.Join(b.workDir, ".claw", "skills"),
			filepath.Join(b.workDir, "skills", "public"),
		}
	}
	resolved := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if !filepath.IsAbs(d) && b.workDir != "" {
			d = filepath.Join(b.workDir, d)
		}
		resolved = append(resolved, d)
	}
	return NewSkillLoader(b.workDir).LoadFromDirs(resolved...)
}

func formatTools(tools []ToolInfo) string {
	if len(tools) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### 可用工具 (Tools)\n")
	b.WriteString("需要执行能力时按 description 选择并调用下列工具；对用户回复中不要暴露工具名与内部参数。\n\n")
	for _, t := range tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(t.Description)
		if desc == "" {
			desc = "(no description)"
		}
		b.WriteString(fmt.Sprintf("- `%s`: %s\n", name, desc))
	}
	return strings.TrimSpace(b.String())
}
