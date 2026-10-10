package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"movieagent/internal/service/sandbox"
)

func writeSandboxBytes(sb sandbox.Sandbox, virtualPath string, data []byte) error {
	if sb == nil {
		return fmt.Errorf("sandbox is nil")
	}
	ls, ok := sb.(*sandbox.LocalSandbox)
	if !ok {
		// Fallback: Go string can hold arbitrary PNG bytes.
		return sb.WriteFile(virtualPath, string(data), false)
	}
	host, err := resolveHostPath(ls, virtualPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(host), 0o755); err != nil {
		return err
	}
	return os.WriteFile(host, data, 0o644)
}

func readSandboxBytes(sb sandbox.Sandbox, virtualPath string) ([]byte, error) {
	if sb == nil {
		return nil, fmt.Errorf("sandbox is nil")
	}
	ls, ok := sb.(*sandbox.LocalSandbox)
	if !ok {
		s, err := sb.ReadFile(virtualPath, 0, 0)
		if err != nil {
			return nil, err
		}
		return []byte(s), nil
	}
	host, err := resolveHostPath(ls, virtualPath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(host)
}

func resolveHostPath(ls *sandbox.LocalSandbox, virtual string) (string, error) {
	virtual = filepath.ToSlash(virtual)
	var best sandbox.PathMapping
	bestLen := -1
	for _, m := range ls.Mappings() {
		prefix := strings.TrimRight(m.ContainerPath, "/")
		if virtual != prefix && !strings.HasPrefix(virtual, prefix+"/") {
			continue
		}
		if len(prefix) <= bestLen {
			continue
		}
		best = m
		bestLen = len(prefix)
	}
	if bestLen < 0 {
		return "", fmt.Errorf("path is not inside a sandbox mount: %s", virtual)
	}
	prefix := strings.TrimRight(best.ContainerPath, "/")
	rel := strings.TrimPrefix(virtual, prefix)
	rel = strings.TrimPrefix(rel, "/")
	host := filepath.Join(best.LocalPath, filepath.FromSlash(rel))
	cleanHost := filepath.Clean(host)
	cleanRoot := filepath.Clean(best.LocalPath)
	if cleanHost != cleanRoot && !strings.HasPrefix(cleanHost, cleanRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes mounted directory: %s", virtual)
	}
	return cleanHost, nil
}
