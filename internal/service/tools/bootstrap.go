package tools

import (
	"sync"

	"movieagent/internal/service/herness"
	"movieagent/internal/service/sandbox"
)

var (
	bootMu     sync.RWMutex
	bootRoot   string
	bootPython string
)

// Bootstrap registers built-in tools at process start (schemas available immediately).
// Call from server/router / StudioHandler construction. Sandbox is bound later via SessionTools.
func Bootstrap(projectRoot, python string) {
	bootMu.Lock()
	if projectRoot != "" {
		bootRoot = projectRoot
	}
	if python != "" {
		bootPython = python
	}
	bootMu.Unlock()
	// Touch schemas so tool catalog is non-empty right after startup.
	_ = SessionTools(nil)
}

// ProjectRoot returns the root captured at Bootstrap (may be empty).
func ProjectRoot() string {
	bootMu.RLock()
	defer bootMu.RUnlock()
	return bootRoot
}

// SessionTools builds tools bound to a sandbox for one agent turn.
func SessionTools(sb sandbox.Sandbox) []herness.Tool {
	bootMu.RLock()
	root := bootRoot
	python := bootPython
	bootMu.RUnlock()
	return Register(Options{
		ProjectRoot: root,
		Python:      python,
		Sandbox:     sb,
	})
}

// ListSchemas returns registered tool schemas (startup catalog).
func ListSchemas() []herness.ToolSchema {
	return herness.NewRegistry(SessionTools(nil)...).Schemas()
}
