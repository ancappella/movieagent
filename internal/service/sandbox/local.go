package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const commandCaptureLimit = 1024 * 1024

// PathMapping translates one virtual container path to a host directory.
type PathMapping struct {
	ContainerPath string
	LocalPath     string
	ReadOnly      bool
}

// LocalSandbox runs commands and file ops on the host, with virtual path mappings.
// Each ExecuteCommand starts a fresh shell; no shell state survives between calls.
type LocalSandbox struct {
	id             string
	mappings       []PathMapping
	written        map[string]struct{}
	commandTimeout time.Duration
}

func NewLocalSandbox(id string, mappings []PathMapping) *LocalSandbox {
	copied := append([]PathMapping(nil), mappings...)
	sort.SliceStable(copied, func(i, j int) bool {
		return len(copied[i].ContainerPath) > len(copied[j].ContainerPath)
	})
	return &LocalSandbox{
		id:             id,
		mappings:       copied,
		written:        map[string]struct{}{},
		commandTimeout: DefaultCommandTimeoutSec * time.Second,
	}
}

func (s *LocalSandbox) ID() string { return s.id }

func (s *LocalSandbox) Mappings() []PathMapping {
	return append([]PathMapping(nil), s.mappings...)
}

func (s *LocalSandbox) ExecuteCommand(ctx context.Context, command string, env map[string]string) (string, error) {
	if err := ValidateExtraEnv(env); err != nil {
		return "", err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := s.commandTimeout
	if timeout <= 0 {
		timeout = DefaultCommandTimeoutSec * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	shell := findShell()
	cmd := exec.CommandContext(ctx, shell, "-c", s.resolveCommand(command))
	cmd.Env = envList(BuildSandboxEnv(env))
	cmd.Stdin = nil

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &stdout, limit: commandCaptureLimit}
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: commandCaptureLimit}
	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\nStd Error:\n"
		}
		output += stderr.String()
	}
	if ctx.Err() == context.DeadlineExceeded {
		if output != "" {
			output += "\n"
		}
		output += fmt.Sprintf("Command timed out after %s. Background long-lived processes instead of running them in the foreground.", timeout)
		output += "\nExit Code: 124"
	} else if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			output += fmt.Sprintf("\nExit Code: %d", exitErr.ExitCode())
		} else if output == "" {
			return "", ErrCommand(err.Error(), command, -1)
		}
	}
	if output == "" {
		output = "(no output)"
	}
	return s.reverseOutput(output), nil
}

func (s *LocalSandbox) ReadFile(path string, startLine, endLine int) (string, error) {
	resolved, _, err := s.resolve(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", ErrFile(err.Error(), path, "read")
	}
	content := string(data)
	if startLine > 0 || endLine > 0 {
		lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
		start := startLine
		if start < 1 {
			start = 1
		}
		end := endLine
		if end <= 0 || end > len(lines) {
			end = len(lines)
		}
		if start > len(lines) {
			content = ""
		} else {
			content = strings.Join(lines[start-1:end], "\n")
		}
	}
	if _, ok := s.written[resolved]; ok {
		content = s.reverseOutput(content)
	}
	return content, nil
}

func (s *LocalSandbox) WriteFile(path, content string, appendFile bool) error {
	resolved, mapping, err := s.resolve(path)
	if err != nil {
		return err
	}
	if mapping.ReadOnly {
		return ErrFile("read-only file system", path, "write")
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return ErrFile(err.Error(), path, "write")
	}
	flag := os.O_CREATE | os.O_WRONLY
	if appendFile {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(resolved, flag, 0o644)
	if err != nil {
		return ErrFile(err.Error(), path, "write")
	}
	defer f.Close()
	if _, err := io.WriteString(f, s.resolveCommand(content)); err != nil {
		return ErrFile(err.Error(), path, "write")
	}
	s.written[resolved] = struct{}{}
	return nil
}

func (s *LocalSandbox) ListDir(path string, maxDepth int) ([]string, error) {
	if maxDepth <= 0 {
		maxDepth = 2
	}
	resolved, _, err := s.resolve(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFile("path not found", path, "list")
		}
		return nil, ErrFile(err.Error(), path, "list")
	}
	if !info.IsDir() {
		return nil, ErrFile("not a directory", path, "list")
	}

	var out []string
	err = filepath.WalkDir(resolved, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == resolved {
			return nil
		}
		rel, relErr := filepath.Rel(resolved, p)
		if relErr != nil {
			return relErr
		}
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		virtual := s.reversePath(p)
		if d.IsDir() && !strings.HasSuffix(virtual, "/") {
			virtual += "/"
		}
		out = append(out, virtual)
		return nil
	})
	if err != nil {
		return nil, ErrFile(err.Error(), path, "list")
	}

	container := strings.TrimRight(filepath.ToSlash(path), "/")
	existing := map[string]struct{}{}
	for _, entry := range out {
		existing[strings.TrimRight(entry, "/")] = struct{}{}
	}
	for _, mapping := range s.mappings {
		child := strings.TrimRight(mapping.ContainerPath, "/")
		if !strings.HasPrefix(child, container+"/") {
			continue
		}
		rel := strings.TrimPrefix(child, container+"/")
		if strings.Contains(rel, "/") {
			continue
		}
		if _, ok := existing[child]; ok {
			continue
		}
		if info, statErr := os.Stat(mapping.LocalPath); statErr == nil && info.IsDir() {
			out = append(out, child+"/")
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *LocalSandbox) resolve(virtual string) (string, PathMapping, error) {
	virtual = filepath.ToSlash(virtual)
	var match *PathMapping
	for i := range s.mappings {
		cp := strings.TrimRight(s.mappings[i].ContainerPath, "/")
		if virtual == cp || strings.HasPrefix(virtual, cp+"/") {
			if match == nil || len(cp) > len(strings.TrimRight(match.ContainerPath, "/")) {
				match = &s.mappings[i]
			}
		}
	}
	if match == nil {
		return "", PathMapping{}, ErrFile("path is not inside a sandbox mount", virtual, "resolve")
	}
	root, err := filepath.Abs(match.LocalPath)
	if err != nil {
		return "", PathMapping{}, err
	}
	rel := strings.TrimPrefix(virtual, strings.TrimRight(match.ContainerPath, "/"))
	rel = strings.TrimPrefix(rel, "/")
	joined := filepath.Join(root, filepath.FromSlash(rel))
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", PathMapping{}, err
	}
	back, err := filepath.Rel(root, abs)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
		return "", PathMapping{}, ErrFile("path escapes mounted directory", virtual, "resolve")
	}
	return abs, *match, nil
}

func (s *LocalSandbox) resolveCommand(text string) string {
	for _, mapping := range s.mappings {
		text = strings.ReplaceAll(text, strings.TrimRight(mapping.ContainerPath, "/"), mapping.LocalPath)
	}
	return text
}

func (s *LocalSandbox) reversePath(local string) string {
	abs, err := filepath.Abs(local)
	if err != nil {
		abs = local
	}
	bestLen := -1
	best := ""
	for _, mapping := range s.mappings {
		root, err := filepath.Abs(mapping.LocalPath)
		if err != nil {
			continue
		}
		if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
			continue
		}
		if len(root) <= bestLen {
			continue
		}
		rel := strings.TrimPrefix(abs, root)
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		rel = filepath.ToSlash(rel)
		container := strings.TrimRight(mapping.ContainerPath, "/")
		if rel == "" {
			best = container
		} else {
			best = container + "/" + rel
		}
		bestLen = len(root)
	}
	if best == "" {
		return filepath.ToSlash(abs)
	}
	return best
}

func (s *LocalSandbox) reverseOutput(output string) string {
	type pair struct{ local, container string }
	var pairs []pair
	for _, mapping := range s.mappings {
		root, err := filepath.Abs(mapping.LocalPath)
		if err != nil {
			continue
		}
		pairs = append(pairs, pair{local: root, container: strings.TrimRight(mapping.ContainerPath, "/")})
	}
	sort.Slice(pairs, func(i, j int) bool { return len(pairs[i].local) > len(pairs[j].local) })
	for _, p := range pairs {
		output = strings.ReplaceAll(output, p.local, p.container)
	}
	return output
}

func findShell() string {
	for _, candidate := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if path, err := exec.LookPath("sh"); err == nil {
		return path
	}
	return "/bin/sh"
}

type limitedWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remain := w.limit - w.buf.Len()
	if remain > 0 {
		if len(p) > remain {
			_, _ = w.buf.Write(p[:remain])
		} else {
			_, _ = w.buf.Write(p)
		}
	}
	return len(p), nil
}
