// Package brandagent implements a DeerFlow-style minimal brand-agent loop:
// one agent, LocalSandbox, model ↔ tools, no multi-agent orchestration.
//
// Poster flow calls the LLM / image model directly (no skill file).
package brandagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"movieagent/internal/service/config"
	"movieagent/internal/service/herness"
	"movieagent/internal/service/sandbox"
	svctools "movieagent/internal/service/tools"
)

const (
	DefaultAgentName = "brand-agent"
	DefaultUserID    = "default"
)

// Options configures New.
type Options struct {
	Name         string
	UserID       string
	Paths        *config.Paths
	Model        herness.Model
	Image        ImageGenerator
	Creative     CreativeDesigner // poster workflow creative-design node; optional
	Compositor   BrandCompositor  // final brand-fixed-elements node; optional
	SkillsSource string           // host dir of repo skills/public; projected into skills_view/public
	SystemPrompt string
	MaxSteps     int
}

// BrandAgent is one DeerFlow-style agent bound to LocalSandbox.
type BrandAgent struct {
	name         string
	userID       string
	paths        *config.Paths
	provider     *sandbox.LocalProvider
	model        herness.Model
	image        ImageGenerator
	creative     CreativeDesigner
	compositor   BrandCompositor
	skillsSource string
	systemPrompt string
	maxSteps     int

	mu       sync.Mutex
	sessions map[string]*session // threadID → session
}

type session struct {
	sandboxID string
	sb        sandbox.Sandbox
	client    *herness.Client
}

// New builds a single agent. Model may be nil if you only call RunBrandTone.
func New(opts Options) (*BrandAgent, error) {
	paths := opts.Paths
	if paths == nil {
		paths = config.GetPaths()
	}
	userID := opts.UserID
	if userID == "" {
		userID = DefaultUserID
	}
	if _, err := config.ValidateUserID(userID); err != nil {
		return nil, err
	}
	name := opts.Name
	if name == "" {
		name = DefaultAgentName
	}
	skillsSrc := opts.SkillsSource
	if skillsSrc == "" {
		skillsSrc = defaultSkillsSource()
	}
	img := opts.Image
	if img == nil {
		img = PlaceholderImage{}
	}
	creative := opts.Creative
	if creative == nil {
		creative = HeuristicCreativeDesigner{}
	}
	compositor := opts.Compositor
	if compositor == nil {
		compositor = AestheticBrandCompositor{Image: img}
	}
	system := opts.SystemPrompt
	if system == "" {
		system = defaultSystemPrompt()
	}
	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = herness.DefaultMaxSteps
	}

	a := &BrandAgent{
		name:         name,
		userID:       userID,
		paths:        paths,
		provider:     sandbox.NewLocalProvider(paths),
		model:        opts.Model,
		image:        img,
		creative:     creative,
		compositor:   compositor,
		skillsSource: skillsSrc,
		systemPrompt: system,
		maxSteps:     maxSteps,
		sessions:     map[string]*session{},
	}
	// Skills projection is optional; poster pipeline does not depend on SKILL.md.
	_ = a.ensurePublicSkills()
	return a, nil
}

// Provider exposes the LocalProvider for tests and advanced wiring.
func (a *BrandAgent) Provider() *sandbox.LocalProvider { return a.provider }

// UserID returns the bound user id.
func (a *BrandAgent) UserID() string { return a.userID }

// AcquireSandbox gets or creates the LocalSandbox for a thread.
func (a *BrandAgent) AcquireSandbox(threadID string) (sandbox.Sandbox, string, error) {
	if _, err := config.ValidateThreadID(threadID); err != nil {
		return nil, "", err
	}
	_ = a.ensurePublicSkills()
	id, err := a.provider.Acquire(threadID, a.userID)
	if err != nil {
		return nil, "", err
	}
	sb, ok := a.provider.Get(id)
	if !ok {
		return nil, "", sandbox.ErrNotFound(id)
	}
	return sb, id, nil
}

// Chat runs one DeerFlow-style turn (model ↔ tools) inside LocalSandbox.
func (a *BrandAgent) Chat(ctx context.Context, threadID, message string) (string, error) {
	if a.model == nil {
		return "", fmt.Errorf("brandagent: Model is required for Chat; use RunBrandTone for deterministic brand-tone")
	}
	sess, err := a.session(ctx, threadID)
	if err != nil {
		return "", err
	}
	return sess.client.Chat(ctx, message, threadID)
}

// Stream is Chat with streaming events.
func (a *BrandAgent) Stream(ctx context.Context, threadID, message string) (<-chan herness.StreamEvent, error) {
	if a.model == nil {
		return nil, fmt.Errorf("brandagent: Model is required for Stream")
	}
	sess, err := a.session(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return sess.client.Stream(ctx, message, threadID)
}

func (a *BrandAgent) session(ctx context.Context, threadID string) (*session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if s, ok := a.sessions[threadID]; ok {
		return s, nil
	}
	sb, id, err := a.AcquireSandbox(threadID)
	if err != nil {
		return nil, err
	}
	userID := a.userID
	tools := sandboxTools(sb)
	tools = append(tools, generateImageTool(a.image, sb), imageResizeTool(sb), herness.AskClarificationTool{})
	tools = append(tools, svctools.SessionTools(sb)...)

	agent, err := herness.NewAgent(herness.Options{
		Name:         a.name,
		Model:        a.model,
		Tools:        tools,
		SystemPrompt: a.systemPrompt,
		MaxSteps:     a.maxSteps,
		Features: &herness.Features{
			Sandbox:       true,
			LoopDetection: true,
			BuiltinTools:  false, // sandbox + generate_image + image_resize + registered tools
		},
		SandboxAcquire: func(_ context.Context, run *herness.RunConfig) (string, error) {
			tid := run.ThreadID
			if tid == "" {
				tid = threadID
			}
			uid := run.UserID
			if uid == "" {
				uid = userID
			}
			return a.provider.Acquire(tid, uid)
		},
	})
	if err != nil {
		return nil, err
	}
	s := &session{
		sandboxID: id,
		sb:        sb,
		client:    herness.NewClient(agent),
	}
	a.sessions[threadID] = s
	_ = ctx
	return s, nil
}

// ensurePublicSkills projects repo skills/public into paths skills_view/public
// so LocalSandbox can mount /mnt/skills/public.
func (a *BrandAgent) ensurePublicSkills() error {
	dst, err := a.paths.PublicSkillsViewDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	src := a.skillsSource
	if src == "" {
		return nil
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "" || e.Name()[0] == '.' {
			continue
		}
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())
		if err := syncDir(from, to); err != nil {
			return fmt.Errorf("project skill %s: %w", e.Name(), err)
		}
	}
	return nil
}

func syncDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func defaultSkillsSource() string {
	_, file, _, ok := runtime.Caller(0)
	if ok {
		// .../internal/service/agents/brandagent/brandagent.go → repo root
		repo := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
		candidate := filepath.Join(repo, "skills", "public")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
	}
	if abs, err := filepath.Abs(filepath.Join("skills", "public")); err == nil {
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			return abs
		}
	}
	return filepath.Join("skills", "public")
}
