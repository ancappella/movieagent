package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"movieagent/internal/service/claw/provider"
	"movieagent/internal/service/claw/schema"
	"movieagent/internal/service/claw/tools"
)

// Tools that may take a long time; run in background so the model can reply first.
const (
	asyncToolPictureBook        = "picturebook_illustrate"
	asyncToolPictureBookAnimate = "picturebook_animate"
)

// Config controls the main loop.
type Config struct {
	MaxTurns       int
	EnableThinking bool
	// ThinkingSystemPrompt is used for the first Thinking phase only (no skills/tools).
	// Empty keeps the system message already present in history.
	ThinkingSystemPrompt string
	// ActionSystemPrompt replaces the system message before each Action phase
	// (system + skills + tools). Empty keeps the current system message.
	ActionSystemPrompt string
}

// DefaultConfig returns MaxTurns=10 with Thinking on (go-claw default).
func DefaultConfig() Config {
	return Config{MaxTurns: 10, EnableThinking: true}
}

// Event is emitted during Run for NDJSON streaming to the chat UI.
type Event struct {
	Type       string // thinking_delta | delta | tool_start | tool_end | image | done | error
	Delta      string
	ToolCallID string
	ToolName   string
	IsError    bool
	Model      string
	Message    string
	URL        string
	Title      string
	Data       map[string]any
}

// Emitter receives loop progress events. May be nil.
type Emitter func(Event)

// Engine runs Thinking → Action → tool exec → loop.
type Engine struct {
	provider provider.Provider
	tools    *tools.Registry
	cfg      Config
}

// New builds an Engine.
func New(p provider.Provider, reg *tools.Registry, cfg Config) *Engine {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = DefaultConfig().MaxTurns
	}
	return &Engine{provider: p, tools: reg, cfg: cfg}
}

type asyncJob struct {
	call schema.ToolCall
	ch   chan schema.ToolResult
}

// Run executes the claw main loop and returns the full history.
func (e *Engine) Run(ctx context.Context, messages []schema.Message, emit Emitter) ([]schema.Message, error) {
	if e.provider == nil {
		return nil, fmt.Errorf("provider is nil")
	}
	if e.tools == nil {
		return nil, fmt.Errorf("tool registry is nil")
	}
	fire := emit
	if fire == nil {
		fire = func(Event) {}
	}

	history := append([]schema.Message(nil), messages...)
	availableTools := e.tools.GetAvailableTools()
	hasPicturebook := toolRegistered(availableTools, asyncToolPictureBook)
	hasAnimate := toolRegistered(availableTools, asyncToolPictureBookAnimate)
	var pending []asyncJob
	var startedPicturebook, startedAnimate bool

	flushAsync := func() {
		for _, job := range pending {
			res := <-job.ch
			fire(Event{
				Type:       "tool_end",
				ToolCallID: res.ToolCallID,
				ToolName:   res.Name,
				Delta:      res.Content,
				IsError:    res.IsError,
				Data:       res.Data,
			})
			// History already has a placeholder tool message; leave it.
		}
		pending = nil
	}
	defer flushAsync()

	enqueueAsync := func(tc schema.ToolCall) {
		fire(Event{
			Type:       "tool_start",
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Delta:      tc.Arguments,
		})
		switch strings.TrimSpace(tc.Name) {
		case asyncToolPictureBook:
			startedPicturebook = true
		case asyncToolPictureBookAnimate:
			startedAnimate = true
		}
		job := e.startAsyncTool(ctx, tc)
		pending = append(pending, job)
		history = append(history, schema.Message{
			Role:       schema.RoleTool,
			Content:    asyncToolPlaceholder(tc.Name),
			ToolCallID: tc.ID,
			Name:       tc.Name,
		})
	}

	for turn := 0; turn < e.cfg.MaxTurns; turn++ {
		turnNo := turn + 1

		if e.cfg.EnableThinking {
			// First Thinking: lean system prompt (no skills / tools sections).
			if turn == 0 && strings.TrimSpace(e.cfg.ThinkingSystemPrompt) != "" {
				history = replaceSystemMessage(history, e.cfg.ThinkingSystemPrompt)
			}
			thinkResp, err := e.provider.Chat(ctx, schema.ChatRequest{
				Messages: history,
				Tools:    nil,
			})
			if err != nil {
				fire(Event{Type: "error", Message: err.Error()})
				return history, fmt.Errorf("thinking (turn %d): %w", turnNo, err)
			}
			if thinkResp == nil {
				return history, fmt.Errorf("thinking returned nil (turn %d)", turnNo)
			}
			if thinkResp.Message.Content != "" {
				fire(Event{Type: "thinking_delta", Delta: thinkResp.Message.Content})
				history = append(history, thinkResp.Message)
			}
		}

		// Action: load skills + tools into system context before tool schemas.
		if strings.TrimSpace(e.cfg.ActionSystemPrompt) != "" {
			history = replaceSystemMessage(history, e.cfg.ActionSystemPrompt)
		}

		actionResp, err := e.provider.Chat(ctx, schema.ChatRequest{
			Messages: history,
			Tools:    availableTools,
		})
		if err != nil {
			fire(Event{Type: "error", Message: err.Error()})
			return history, fmt.Errorf("action (turn %d): %w", turnNo, err)
		}
		if actionResp == nil {
			return history, fmt.Errorf("action returned nil (turn %d)", turnNo)
		}

		history = append(history, actionResp.Message)
		nCalls := len(actionResp.Message.ToolCalls)

		if nCalls == 0 {
			if actionResp.Message.Content != "" {
				fire(Event{Type: "delta", Delta: actionResp.Message.Content})
			}
			// Model often writes「正在出图/出动画」without a toolCall — force the
			// matching skill so the turn cannot finish before generation starts.
			userText := lastUserText(history)
			asstText := actionResp.Message.Content
			wantAnim := wantsAnimation(userText, asstText)
			if !startedAnimate && hasAnimate && wantAnim {
				enqueueAsync(autoAnimateCall(userText))
			} else if !startedPicturebook && hasPicturebook && wantsIllustration(userText, asstText) {
				enqueueAsync(autoPicturebookCall(userText))
			}
			flushAsync()
			fire(Event{Type: "done", Model: e.provider.Model()})
			return history, nil
		}

		// Stream any prose that arrived with the tool call(s) before waiting on tools.
		if content := strings.TrimSpace(actionResp.Message.Content); content != "" {
			fire(Event{Type: "delta", Delta: content})
		}

		var syncCalls []schema.ToolCall
		for _, tc := range actionResp.Message.ToolCalls {
			if isAsyncTool(tc.Name) {
				enqueueAsync(tc)
			} else {
				fire(Event{
					Type:       "tool_start",
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Delta:      tc.Arguments,
				})
				syncCalls = append(syncCalls, tc)
			}
		}

		for _, r := range e.tools.ExecuteAll(ctx, syncCalls) {
			fire(Event{
				Type:       "tool_end",
				ToolCallID: r.ToolCallID,
				ToolName:   r.Name,
				Delta:      r.Content,
				IsError:    r.IsError,
				Data:       r.Data,
			})
			history = append(history, schema.Message{
				Role:       schema.RoleTool,
				Content:    r.Content,
				ToolCallID: r.ToolCallID,
				Name:       r.Name,
			})
		}
	}

	flushAsync()
	err := fmt.Errorf("max turns (%d) exceeded", e.cfg.MaxTurns)
	fire(Event{Type: "error", Message: err.Error()})
	return history, err
}

func (e *Engine) startAsyncTool(ctx context.Context, tc schema.ToolCall) asyncJob {
	ch := make(chan schema.ToolResult, 1)
	job := asyncJob{call: tc, ch: ch}
	go func() {
		// Detach from parent cancel only for the generation work if needed?
		// Keep ctx so client abort still stops diffusion when possible.
		res := e.tools.Execute(ctx, tc)
		select {
		case ch <- res:
		default:
		}
	}()
	return job
}

func isAsyncTool(name string) bool {
	switch strings.TrimSpace(name) {
	case asyncToolPictureBook, asyncToolPictureBookAnimate:
		return true
	default:
		return false
	}
}

func asyncToolPlaceholder(name string) string {
	if strings.TrimSpace(name) == asyncToolPictureBookAnimate {
		return "ok: animation started in background. " +
			"Write a short description of the motion for the user now; " +
			"the system will display the video separately when ready. " +
			"Do not claim the video is finished until a later tool result confirms it; " +
			"do not expose paths or tool names."
	}
	return "ok: image generation started in background. " +
		"Write a short scheme description for the user now; " +
		"the system will display the image separately when ready. " +
		"Do not claim the image is finished until a later tool result confirms it; " +
		"do not expose paths or tool names."
}

var (
	imageIntentRe = regexp.MustCompile(`(?i)画|绘制|插画|出图|生图|海报|配图|illustrat|picture\s*book|画一`)
	// Do not treat bare「稍后展示」as still-image intent — animation replies use it too.
	imageClaimRe = regexp.MustCompile(`正在出图|下方展示|生成图片|开始出图|插画稍后`)
	animIntentRe = regexp.MustCompile(`(?i)做成动画|做成短片|动起来|ken\s*burns|生成\s*\d*\s*s?\s*的?动画|生成动画|出动画|转成视频|做成视频|秒的?动画|\d+\s*s的?动画|上述的图.*动画|用.*图.*动画`)
	animClaimRe  = regexp.MustCompile(`正在出动画|正在生成动画|成片稍后|稍后.*(?:视频|成片|动画)|动画稍后`)
	durationRe   = regexp.MustCompile(`(?i)(\d+)\s*s(?:ec(?:onds?)?)?|(\d+)\s*秒`)
)

func toolRegistered(defs []schema.ToolDefinition, name string) bool {
	for _, d := range defs {
		if d.Name == name {
			return true
		}
	}
	return false
}

func lastUserText(history []schema.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == schema.RoleUser {
			return strings.TrimSpace(history[i].Content)
		}
	}
	return ""
}

func wantsIllustration(userText, assistantText string) bool {
	if wantsAnimation(userText, assistantText) {
		return false
	}
	// Strip 动画 so the bare「画」token inside it does not look like a draw request.
	scrubbed := strings.ReplaceAll(userText, "动画片", "")
	scrubbed = strings.ReplaceAll(scrubbed, "动画", "")
	return imageIntentRe.MatchString(scrubbed) || imageClaimRe.MatchString(assistantText)
}

func wantsAnimation(userText, assistantText string) bool {
	return animIntentRe.MatchString(userText) || animClaimRe.MatchString(assistantText)
}

func parseDurationSec(text string, fallback float64) float64 {
	m := durationRe.FindStringSubmatch(text)
	if len(m) == 0 {
		return fallback
	}
	raw := m[1]
	if raw == "" {
		raw = m[2]
	}
	var n float64
	if _, err := fmt.Sscanf(raw, "%f", &n); err != nil || n <= 0 {
		return fallback
	}
	if n > 30 {
		n = 30
	}
	return n
}

func autoPicturebookCall(prompt string) schema.ToolCall {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "children's picture book illustration"
	}
	args, _ := json.Marshal(map[string]any{
		"prompt":       prompt,
		"aspect_ratio": "2:3",
	})
	return schema.ToolCall{
		ID:        fmt.Sprintf("auto-pb-%d", time.Now().UnixNano()),
		Name:      asyncToolPictureBook,
		Arguments: string(args),
	}
}

func autoAnimateCall(userText string) schema.ToolCall {
	template := "character_idle"
	lower := strings.ToLower(userText)
	if strings.Contains(userText, "旋转") || strings.Contains(userText, "转圈") ||
		strings.Contains(lower, "spin") || strings.Contains(userText, "原地转") {
		template = "character_spin"
	}
	args, _ := json.Marshal(map[string]any{
		"template":     template,
		"prompt":       strings.TrimSpace(userText),
		"duration_sec": parseDurationSec(userText, 4),
		"image_path":   "/mnt/user-data/outputs/picturebook.png",
	})
	return schema.ToolCall{
		ID:        fmt.Sprintf("auto-anim-%d", time.Now().UnixNano()),
		Name:      asyncToolPictureBookAnimate,
		Arguments: string(args),
	}
}

// replaceSystemMessage sets the first system message content, or prepends one if missing.
func replaceSystemMessage(msgs []schema.Message, content string) []schema.Message {
	content = strings.TrimSpace(content)
	if content == "" {
		return msgs
	}
	out := append([]schema.Message(nil), msgs...)
	for i := range out {
		if out[i].Role == schema.RoleSystem {
			out[i].Content = content
			return out
		}
	}
	return append([]schema.Message{{Role: schema.RoleSystem, Content: content}}, out...)
}
