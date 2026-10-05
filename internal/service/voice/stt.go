package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Options struct {
	ProjectRoot string
	Python      string
	Model       string
	Language    string
	Timeout     time.Duration
}

type Line struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	MS        int64     `json:"ms,omitempty"`
}

type Service struct {
	opts   Options
	mu     sync.Mutex
	bySess map[string][]Line
}

type sttResult struct {
	OK    bool   `json:"ok"`
	Text  string `json:"text"`
	Error string `json:"error"`
}

func New(opts Options) *Service {
	if opts.Model == "" {
		opts.Model = "tiny"
	}
	if opts.Language == "" {
		opts.Language = "zh"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 90 * time.Second
	}
	if opts.Python == "" {
		opts.Python = resolvePython(opts.ProjectRoot)
	}
	return &Service{
		opts:   opts,
		bySess: map[string][]Line{},
	}
}

func (s *Service) Python() string { return s.opts.Python }
func (s *Service) Model() string  { return s.opts.Model }

func (s *Service) ScriptPath() string {
	return filepath.Join(s.opts.ProjectRoot, "internal", "service", "voice", "whisper_stt.py")
}

func (s *Service) Ready() error {
	if _, err := os.Stat(s.ScriptPath()); err != nil {
		return fmt.Errorf("whisper script missing: %w", err)
	}
	if s.opts.Python == "" {
		return fmt.Errorf("VOICE_PYTHON is empty")
	}
	if _, err := os.Stat(s.opts.Python); err != nil && !strings.Contains(s.opts.Python, string(os.PathSeparator)) {
		if _, lookErr := exec.LookPath(s.opts.Python); lookErr != nil {
			return fmt.Errorf("python not found: %s", s.opts.Python)
		}
		return nil
	}
	return nil
}

func (s *Service) TranscribeFile(path string) (string, error) {
	if err := s.Ready(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.opts.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.opts.Python, s.ScriptPath(), path, "--model", s.opts.Model, "--language", s.opts.Language)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	trimmed := lastJSONLine(string(out))
	var res sttResult
	if jsonErr := json.Unmarshal([]byte(trimmed), &res); jsonErr == nil {
		if res.OK {
			return strings.TrimSpace(res.Text), nil
		}
		msg := strings.TrimSpace(res.Error)
		if msg == "" {
			msg = "transcription failed"
		}
		return "", fmt.Errorf("whisper: %s", msg)
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("whisper: %s", msg)
	}
	return "", fmt.Errorf("whisper output: %s", trimmed)
}

func (s *Service) Append(sessionID, text string, elapsedMS int64) Line {
	line := Line{
		ID:        fmt.Sprintf("utt-%d", time.Now().UnixNano()),
		Text:      strings.TrimSpace(text),
		CreatedAt: time.Now(),
		MS:        elapsedMS,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list := append(s.bySess[sessionID], line)
	if len(list) > 200 {
		list = list[len(list)-200:]
	}
	s.bySess[sessionID] = list
	return line
}

func (s *Service) List(sessionID string) []Line {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.bySess[sessionID]
	out := make([]Line, len(src))
	copy(out, src)
	return out
}

func (s *Service) Clear(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bySess, sessionID)
}

func resolvePython(root string) string {
	candidates := []string{
		os.Getenv("VOICE_PYTHON"),
		filepath.Join(root, "..", "pi-agent-notebook", "chapter", "chapter07", ".venv", "bin", "python"),
		filepath.Join(root, "chapter07", ".venv", "bin", "python"),
	}
	for _, p := range candidates {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return "python3"
}

func lastJSONLine(raw string) string {
	raw = strings.TrimSpace(raw)
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") {
			return line
		}
	}
	return raw
}
