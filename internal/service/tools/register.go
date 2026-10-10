// Package tools registers callable agent capabilities for movieagent.
package tools

import (
	"strings"

	agentctx "movieagent/internal/context"
	"movieagent/internal/service/config"
	"movieagent/internal/service/herness"
	"movieagent/internal/service/sandbox"
)

// Options configure Register.
type Options struct {
	ProjectRoot string
	Python      string // optional; defaults to PICTUREBOOK_PYTHON / VOICE_PYTHON / python3
	Sandbox     sandbox.Sandbox
	// SkillDir / SkillScript bind a tool to a concrete skill package (scripts/).
	SkillDir    string
	SkillScript string
}

// Register discovers tools from skills (SKILL.md tool/script frontmatter) and
// returns herness.Tool list. Picture-book tools are registered when their skills
// expose the declared entry scripts (Canvas draw.mjs / render.mjs).
func Register(opts Options) []herness.Tool {
	root := strings.TrimSpace(opts.ProjectRoot)
	if root == "" {
		if r, err := config.ProjectRoot(); err == nil {
			root = r
		}
	}
	opts.ProjectRoot = root

	var out []herness.Tool
	if root != "" {
		for _, sk := range agentctx.NewSkillLoader(root).List() {
			toolName := strings.TrimSpace(sk.Tool)
			script := sk.ScriptPath()
			if toolName == "" || script == "" {
				continue
			}
			switch toolName {
			case PictureBookToolName:
				o := opts
				o.SkillDir = sk.Dir
				o.SkillScript = script
				out = append(out, NewPictureBookIllustrator(o))
			case PictureBookAnimateToolName:
				o := opts
				o.SkillDir = sk.Dir
				o.SkillScript = script
				out = append(out, NewPictureBookAnimator(o))
			}
		}
	}

	// Fallback when skill metadata is missing but legacy layout still exists.
	if !hasTool(out, PictureBookToolName) {
		out = append(out, NewPictureBookIllustrator(opts))
	}
	if !hasTool(out, PictureBookAnimateToolName) {
		out = append(out, NewPictureBookAnimator(opts))
	}
	return out
}

func hasTool(list []herness.Tool, name string) bool {
	for _, t := range list {
		if t != nil && t.Name() == name {
			return true
		}
	}
	return false
}
