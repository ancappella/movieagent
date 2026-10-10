package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	agentctx "movieagent/internal/context"
	"movieagent/internal/service/agentruntime"
	"movieagent/internal/service/agents/brandagent"
	"movieagent/internal/service/chathub"
	"movieagent/internal/service/claw/engine"
	"movieagent/internal/service/claw/schema"
	clawtools "movieagent/internal/service/claw/tools"
	svcconfig "movieagent/internal/service/config"
	"movieagent/internal/service/dispatch"
	"movieagent/internal/service/providers/deepseek"
	"movieagent/internal/service/sandbox"
	"movieagent/internal/service/storage"
	"movieagent/internal/service/task"
	svctools "movieagent/internal/service/tools"
	"movieagent/pkg/response"

	"github.com/gin-gonic/gin"
)

var internalLeakRe = regexp.MustCompile(`(?i)(/mnt/[^\s)\]，。；;]+|sandbox[^\n]*|tool[_ ]?call[^\n]*|picturebook_illustrate|picturebook_animate|output_path|wrote\s+/[^\n]*|\d+\s*bytes?|seed\s*[:=]\s*\d+)`)

// ChatHandler serves the claw-style chat API (tool registry + Thinking/Action loop).
type ChatHandler struct {
	Client      *deepseek.Client
	Sandboxes   *sandbox.LocalProvider
	Paths       *svcconfig.Paths
	ProjectRoot string
	Dispatch    *dispatch.Dispatcher
	Hub         *chathub.Hub
	// Objects uploads runtime images (OSS). Nil / disabled → local /media/generated.
	Objects storage.ObjectStore
}

// NewChatHandler builds a chat handler backed by DeepSeek.
func NewChatHandler(apiKey, baseURL, model string) *ChatHandler {
	root, _ := os.Getwd()
	return &ChatHandler{
		Client:      deepseek.New(deepseek.Config{APIKey: apiKey, BaseURL: baseURL, Model: model}),
		Sandboxes:   sandbox.NewLocalProvider(svcconfig.GetPaths()),
		Paths:       svcconfig.GetPaths(),
		ProjectRoot: root,
		Dispatch:    dispatch.Default(),
		Hub:         chathub.New(),
		Objects:     storage.NewFromEnv(),
	}
}

func (h *ChatHandler) hub() *chathub.Hub {
	if h.Hub == nil {
		h.Hub = chathub.New()
	}
	return h.Hub
}

type chatMessageIn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequestBody struct {
	Message        string          `json:"message"`
	Messages       []chatMessageIn `json:"messages"`
	EnableThinking *bool           `json:"enable_thinking,omitempty"`
	MaxTurns       int             `json:"max_turns,omitempty"`
}

type chatStartData struct {
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Events  string `json:"events"` // SSE subscribe path
}

// Chat only enqueues a task and returns task_id. Progress streams via TaskEvents (SSE).
func (h *ChatHandler) Chat(c *gin.Context) {
	var body chatRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, http.StatusBadRequest, 1, "invalid json")
		return
	}

	userMsg := strings.TrimSpace(body.Message)
	if userMsg == "" {
		for i := len(body.Messages) - 1; i >= 0; i-- {
			if strings.EqualFold(body.Messages[i].Role, "user") {
				userMsg = strings.TrimSpace(body.Messages[i].Content)
				break
			}
		}
	}
	if userMsg == "" && len(body.Messages) == 0 {
		response.Fail(c, http.StatusBadRequest, 1, "message required")
		return
	}

	if h.Client == nil {
		h.Client = deepseek.New(deepseek.Config{})
	}
	if !h.Client.Configured() {
		response.Fail(c, http.StatusServiceUnavailable, 1, "DEEPSEEK_API_KEY 未配置")
		return
	}

	cfg := engine.DefaultConfig()
	if body.EnableThinking != nil {
		cfg.EnableThinking = *body.EnableThinking
	}
	if body.MaxTurns > 0 {
		cfg.MaxTurns = body.MaxTurns
	}

	disp := h.Dispatch
	if disp == nil {
		disp = dispatch.Default()
	}

	home, _ := svcconfig.RuntimeHome()
	if home == "" {
		home = filepath.Join(h.ProjectRoot, ".deer-flow")
	}
	sched := task.NewScheduler(task.NewFileStore(home))
	agentTask, err := sched.Start(c.Request.Context(), task.StartRequest{
		ThreadID:  "chat",
		SessionID: "chat",
		Goal:      userMsg,
		Meta: map[string]any{
			"route":            "/api/v1/chat",
			"dispatch_max":     disp.Config().MaxTasks,
			"dispatch_max_llm": disp.Config().MaxLLM,
		},
	})
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, 1, "task start failed")
		return
	}

	hub := h.hub()
	hub.Publish(agentTask.ID, chathub.Event{Type: "task", Status: string(task.StatusPending)})

	// Detach from HTTP: work continues under dispatch even after POST returns.
	jobBody := body
	jobMsg := userMsg
	taskID := agentTask.ID
	go h.runChatTask(context.Background(), taskID, jobMsg, jobBody, cfg)

	response.OK(c, chatStartData{
		TaskID: taskID,
		Status: string(task.StatusPending),
		Events: "/api/v1/chat/tasks/" + taskID + "/events",
	})
}

// TaskEvents streams task progress as Server-Sent Events (SSE).
//
//	event: message
//	data: {"type":"thinking_delta","delta":"..."}
//
//	data: {"type":"delta","delta":"..."}
//	data: {"type":"done","model":"..."}
func (h *ChatHandler) TaskEvents(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.Fail(c, http.StatusBadRequest, 1, "task id required")
		return
	}

	home, _ := svcconfig.RuntimeHome()
	if home == "" {
		home = filepath.Join(h.ProjectRoot, ".deer-flow")
	}
	store := task.NewFileStore(home)
	if _, err := store.GetTask(c.Request.Context(), taskID); err != nil && !h.hub().Has(taskID) {
		response.Fail(c, http.StatusNotFound, 1, "task not found")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, canFlush := c.Writer.(http.Flusher)
	writeSSE := func(ev chathub.Event) bool {
		raw, err := json.Marshal(ev)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", raw); err != nil {
			return false
		}
		if canFlush {
			flusher.Flush()
		}
		return true
	}

	history, live, unsub := h.hub().Subscribe(taskID)
	defer unsub()

	for _, ev := range history {
		if !writeSSE(ev) {
			return
		}
		if ev.Type == "done" || ev.Type == "error" {
			return
		}
	}

	notify := c.Request.Context().Done()
	for {
		select {
		case <-notify:
			return
		case ev, ok := <-live:
			if !ok {
				return
			}
			if !writeSSE(ev) {
				return
			}
			if ev.Type == "done" || ev.Type == "error" {
				return
			}
		}
	}
}

func (h *ChatHandler) runtimeHome() string {
	home, _ := svcconfig.RuntimeHome()
	if home == "" {
		home = filepath.Join(h.ProjectRoot, ".deer-flow")
	}
	return home
}

func (h *ChatHandler) runChatTask(ctx context.Context, taskID, userMsg string, body chatRequestBody, cfg engine.Config) {
	hub := h.hub()
	publish := func(ev chathub.Event) {
		hub.Publish(taskID, ev)
	}
	fail := func(msg string) {
		publish(chathub.Event{Type: "error", Message: scrubUserText(msg)})
	}

	disp := h.Dispatch
	if disp == nil {
		disp = dispatch.Default()
	}

	home := h.runtimeHome()
	taskStore := task.NewFileStore(home)
	sched := task.NewScheduler(taskStore)

	rt, rerr := agentruntime.New(agentruntime.Options{
		Root:      home,
		SessionID: "chat",
		ThreadID:  "chat",
		TaskID:    taskID,
		Model:     h.Client.Model(),
		WorkDir:   h.ProjectRoot,
	})
	if rerr != nil {
		_, _ = sched.Finish(ctx, taskID, nil, rerr)
		fail(rerr.Error())
		return
	}
	if cp, ok := rt.Checkpoints.(*agentruntime.FileCheckpointer); ok {
		cp.WithTaskStore(taskStore)
	}
	_ = rt.InjectMemory(ctx)

	sb := h.chatSandbox()
	reg := clawtools.NewRegistry()
	clawtools.RegisterBuiltins(reg)
	clawtools.RegisterHarness(reg, svctools.SessionTools(sb)...)
	reg.Gate = dispatch.ToolGate(disp, dispatch.SkillNames{
		svctools.PictureBookToolName:        {},
		svctools.PictureBookAnimateToolName: {},
	})
	llm := dispatch.WrapProvider(h.Client, disp)

	toolInfos := make([]agentctx.ToolInfo, 0)
	for _, def := range reg.GetAvailableTools() {
		toolInfos = append(toolInfos, agentctx.ToolInfo{
			Name:        def.Name,
			Description: def.Description,
		})
	}
	ctxBuilder := agentctx.NewBuilder(h.ProjectRoot).WithTools(toolInfos...)

	sess := rt.Context
	cfg.ThinkingSystemPrompt = systemForPhase(sess, ctxBuilder, agentctx.PhaseThinking)
	cfg.ActionSystemPrompt = systemForPhase(sess, ctxBuilder, agentctx.PhaseAction)

	startPhase := agentctx.PhaseAction
	if cfg.EnableThinking {
		startPhase = agentctx.PhaseThinking
	}
	sess.ApplyPhase(ctxBuilder, startPhase)

	for _, m := range body.Messages {
		sess.IngestClientHistory(m.Role, m.Content)
	}
	if userMsg != "" {
		sess.AppendUser(userMsg)
	}

	msgs := wireToSchema(sess.WireMessages())
	if len(msgs) < 2 {
		_, _ = sched.Finish(ctx, taskID, nil, fmt.Errorf("message required"))
		fail("message required")
		return
	}
	_ = rt.SeedInfer(ctx)

	eng := engine.New(llm, reg, cfg)

	runErr := disp.Run(ctx, func(jobCtx context.Context) error {
		if _, aerr := sched.Activate(jobCtx, taskID); aerr != nil {
			_, _ = sched.Finish(jobCtx, taskID, nil, aerr)
			publish(chathub.Event{Type: "error", Message: scrubUserText(aerr.Error())})
			return aerr
		}
		publish(chathub.Event{Type: "task", Status: string(task.StatusRunning)})

		turn, terr := sched.BeginTurn(jobCtx, taskID, userMsg)
		if terr != nil {
			_, _ = sched.Finish(jobCtx, taskID, nil, terr)
			publish(chathub.Event{Type: "error", Message: scrubUserText(terr.Error())})
			return terr
		}
		rt.SetTurn(turn.ID)
		_, _ = rt.Barrier(jobCtx, agentruntime.PhaseCPThinking, agentruntime.Continuation{Kind: "need_action"}, 0, nil, nil)

		openSkills := map[string]string{}
		pendingTools := []agentruntime.ToolCallState{}
		var thinkingBuf strings.Builder
		var assistantBuf strings.Builder
		var settled atomic.Bool
		var thinkingAnnounced bool
		var replyAnnounced bool

		publish(chathub.Event{Type: "progress", Message: "开始本轮"})

		history, engErr := eng.Run(jobCtx, msgs, func(ev engine.Event) {
			switch ev.Type {
			case "thinking_delta":
				thinkingBuf.WriteString(ev.Delta)
				if !thinkingAnnounced && strings.TrimSpace(ev.Delta) != "" {
					thinkingAnnounced = true
					publish(chathub.Event{Type: "progress", Message: "正在思考…"})
					_, _ = rt.Barrier(jobCtx, agentruntime.PhaseCPAction, agentruntime.Continuation{Kind: "need_action"}, 0, pendingTools, nil)
				}
			case "tool_start":
				kind := task.KindTool
				label := toolProgressLabel(ev.ToolName)
				switch ev.ToolName {
				case svctools.PictureBookToolName:
					kind = task.KindSkill
					publish(chathub.Event{Type: "status", Message: "正在生成图片"})
				case svctools.PictureBookAnimateToolName:
					kind = task.KindSkill
					publish(chathub.Event{Type: "status", Message: "正在生成动画"})
				}
				publish(chathub.Event{Type: "progress", Message: "调用 " + label})
				var args any
				if strings.TrimSpace(ev.Delta) != "" {
					args = json.RawMessage(ev.Delta)
				}
				sk, serr := sched.BeginSkill(jobCtx, turn.ID, ev.ToolName, kind, args)
				if serr == nil && sk != nil {
					openSkills[ev.ToolCallID] = sk.ID
				}
				pendingTools = append(pendingTools, agentruntime.ToolCallState{
					ToolCallID: ev.ToolCallID,
					Name:       ev.ToolName,
					Arguments:  ev.Delta,
					Status:     "running",
					Replay:     agentruntime.DefaultReplayForTool(ev.ToolName),
				})
				phase := agentruntime.PhaseCPToolBatch
				cont := agentruntime.Continuation{Kind: "need_tools"}
				if agentruntime.DefaultReplayForTool(ev.ToolName) == "never" {
					phase = agentruntime.PhaseCPAwaitAsync
					cont = agentruntime.Continuation{Kind: "await_async"}
				}
				_, _ = rt.Barrier(jobCtx, phase, cont, 0, pendingTools, nil)
			case "tool_end":
				label := toolProgressLabel(ev.ToolName)
				if sid, ok := openSkills[ev.ToolCallID]; ok {
					var skillErr error
					if ev.IsError {
						skillErr = fmt.Errorf("%s", ev.Delta)
					}
					_, _ = sched.EndSkill(jobCtx, sid, ev.Delta, ev.Data, skillErr)
					delete(openSkills, ev.ToolCallID)
				}
				for i := range pendingTools {
					if pendingTools[i].ToolCallID == ev.ToolCallID {
						pendingTools[i].Status = "completed"
						if ev.IsError {
							pendingTools[i].Status = "failed"
						}
						pendingTools[i].Result = ev.Delta
						pendingTools[i].IsError = ev.IsError
						if ev.Data != nil {
							pendingTools[i].Memo = map[string]any{"data": ev.Data}
						}
					}
				}
				_ = rt.CaptureSchemaHistory(jobCtx, agentruntime.PhaseTool, []agentctx.Message{{
					Kind:       agentctx.KindToolResult,
					ToolCallID: ev.ToolCallID,
					ToolName:   ev.ToolName,
					Content:    ev.Delta,
					IsError:    ev.IsError,
				}}, true)
				_, _ = rt.Barrier(jobCtx, agentruntime.PhaseCPAction, agentruntime.Continuation{Kind: "need_action"}, 0, pendingTools, nil)
				if ev.IsError {
					publish(chathub.Event{Type: "progress", Message: label + " 失败"})
				} else {
					publish(chathub.Event{Type: "progress", Message: label + " 完成"})
				}
				switch ev.ToolName {
				case svctools.PictureBookToolName:
					if ev.IsError {
						publish(chathub.Event{Type: "status", Message: "插画生成失败，请稍后重试或换个描述再试。"})
						return
					}
					url, title := h.publishPicturebook(ev.Data)
					if url == "" {
						publish(chathub.Event{Type: "status", Message: "插画已生成，但发布到聊天失败，请重试。"})
						return
					}
					publish(chathub.Event{Type: "image", URL: url, Title: title})
				case svctools.PictureBookAnimateToolName:
					if ev.IsError {
						publish(chathub.Event{Type: "status", Message: "动画生成失败，请稍后重试或换个效果再试。"})
						return
					}
					url, title := h.publishPicturebookVideo(ev.Data)
					if url == "" {
						publish(chathub.Event{Type: "status", Message: "动画已生成，但发布到聊天失败，请重试。"})
						return
					}
					publish(chathub.Event{Type: "video", URL: url, Title: title})
				}
			case "delta":
				text := scrubUserText(ev.Delta)
				if text == "" {
					return
				}
				if !replyAnnounced {
					replyAnnounced = true
					publish(chathub.Event{Type: "progress", Message: "正在回复…"})
				}
				assistantBuf.WriteString(text)
				publish(chathub.Event{Type: "delta", Delta: text})
			case "done":
				if settled.CompareAndSwap(false, true) {
					publish(chathub.Event{Type: "progress", Message: "本轮完成"})
					publish(chathub.Event{Type: "done", Model: ev.Model})
				}
			case "error":
				if settled.CompareAndSwap(false, true) {
					publish(chathub.Event{Type: "error", Message: scrubUserText(ev.Message)})
				}
			}
		})

		if len(history) > 0 {
			_ = rt.CaptureSchemaHistory(jobCtx, agentruntime.PhaseFinal, agentruntime.FromSchemaHistory(history), false)
		}
		finalPhase := agentruntime.PhaseCPDone
		finalCont := agentruntime.Continuation{Kind: "may_finish"}
		if engErr != nil {
			finalPhase = agentruntime.PhaseCPAction
			finalCont = agentruntime.Continuation{Kind: "need_action"}
		}
		_, _ = rt.Barrier(jobCtx, finalPhase, finalCont, 0, pendingTools, nil)

		_, _ = sched.EndTurn(jobCtx, turn.ID, thinkingBuf.String(), assistantBuf.String(), engErr)
		_, _ = sched.Finish(jobCtx, taskID, map[string]any{
			"model": h.Client.Model(),
		}, engErr)

		if engErr != nil {
			if settled.CompareAndSwap(false, true) {
				publish(chathub.Event{Type: "error", Message: scrubUserText(engErr.Error())})
			}
		} else if settled.CompareAndSwap(false, true) {
			publish(chathub.Event{Type: "done", Model: h.Client.Model()})
		}
		return engErr
	})

	if runErr != nil {
		got, gerr := sched.Store.GetTask(context.Background(), taskID)
		if gerr == nil && got != nil && got.Status == task.StatusPending {
			_, _ = sched.Finish(context.Background(), taskID, nil, runErr)
			fail("task cancelled while queued")
		} else if gerr == nil && got != nil && !got.Status.Terminal() {
			_, _ = sched.Finish(context.Background(), taskID, nil, runErr)
			fail(runErr.Error())
		}
	}
}

func toolProgressLabel(name string) string {
	switch strings.TrimSpace(name) {
	case svctools.PictureBookToolName:
		return "绘本插画"
	case svctools.PictureBookAnimateToolName:
		return "插画动画"
	case "":
		return "工具"
	default:
		// Avoid leaking raw tool ids; keep a short readable label.
		if strings.Contains(name, "_") {
			parts := strings.Split(name, "_")
			return parts[len(parts)-1]
		}
		return name
	}
}

func scrubUserText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = internalLeakRe.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		if strings.Contains(line, "/mnt/") || strings.Contains(strings.ToLower(line), "sandbox") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func (h *ChatHandler) publishPicturebook(data map[string]any) (url, title string) {
	if data == nil {
		return "", ""
	}
	virt, _ := data["path"].(string)
	virt = strings.TrimSpace(virt)
	if virt == "" {
		virt = svctools.PictureBookDefaultOut
	}
	if h.Paths == nil {
		h.Paths = svcconfig.GetPaths()
	}
	userID := brandagent.DefaultUserID
	hostPath, err := h.Paths.ResolveVirtualPath("chat", virt, &userID)
	if err != nil {
		return "", ""
	}
	png, err := os.ReadFile(hostPath)
	if err != nil || len(png) == 0 {
		return "", ""
	}
	root := h.ProjectRoot
	if root == "" {
		root, _ = os.Getwd()
	}
	outDir := filepath.Join(root, "tmp", "generated")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", ""
	}
	name := fmt.Sprintf("chat-%s-%d.png", time.Now().Format("20060102-150405"), time.Now().UnixNano()%1_000_000)
	if err := os.WriteFile(filepath.Join(outDir, name), png, 0o644); err != nil {
		return "", ""
	}
	title = "插画"
	if w := toIntAny(data["width"]); w > 0 {
		if ht := toIntAny(data["height"]); ht > 0 {
			title = fmt.Sprintf("插画 %dx%d", w, ht)
		}
	}

	localURL := "/media/generated/" + name
	store := h.Objects
	if store == nil {
		store = storage.Nop{}
	}
	if store.Enabled() {
		keyPrefix := storage.LoadConfig().KeyPrefix
		key := storage.ChatImageKey(keyPrefix, name)
		if remote, uerr := store.Put(context.Background(), key, png, "image/png"); uerr == nil && strings.TrimSpace(remote) != "" {
			return remote, title
		}
	}
	return localURL, title
}

func (h *ChatHandler) publishPicturebookVideo(data map[string]any) (url, title string) {
	if data == nil {
		return "", ""
	}
	virt, _ := data["path"].(string)
	virt = strings.TrimSpace(virt)
	if virt == "" {
		virt = svctools.PictureBookAnimateDefaultOut
	}
	if h.Paths == nil {
		h.Paths = svcconfig.GetPaths()
	}
	userID := brandagent.DefaultUserID
	hostPath, err := h.Paths.ResolveVirtualPath("chat", virt, &userID)
	if err != nil {
		return "", ""
	}
	mp4, err := os.ReadFile(hostPath)
	if err != nil || len(mp4) == 0 {
		return "", ""
	}
	root := h.ProjectRoot
	if root == "" {
		root, _ = os.Getwd()
	}
	outDir := filepath.Join(root, "tmp", "generated")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", ""
	}
	name := fmt.Sprintf("chat-%s-%d.mp4", time.Now().Format("20060102-150405"), time.Now().UnixNano()%1_000_000)
	if err := os.WriteFile(filepath.Join(outDir, name), mp4, 0o644); err != nil {
		return "", ""
	}
	title = "动画"
	if tpl, _ := data["template"].(string); strings.TrimSpace(tpl) != "" {
		title = "动画 " + strings.TrimSpace(tpl)
	} else if hasCode, _ := data["has_code"].(bool); hasCode {
		title = "动画 code"
	}
	if w := toIntAny(data["width"]); w > 0 {
		if ht := toIntAny(data["height"]); ht > 0 {
			title = fmt.Sprintf("%s %dx%d", title, w, ht)
		}
	}

	localURL := "/media/generated/" + name
	store := h.Objects
	if store == nil {
		store = storage.NewFromEnv()
	}
	if store.Enabled() {
		keyPrefix := storage.LoadConfig().KeyPrefix
		key := storage.ChatImageKey(keyPrefix, name)
		remote, uerr := store.Put(context.Background(), key, mp4, "video/mp4")
		if uerr != nil || strings.TrimSpace(remote) == "" {
			// OSS is the source of truth for chat videos when configured.
			return "", ""
		}
		return remote, title
	}
	return localURL, title
}

func toIntAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func (h *ChatHandler) chatSandbox() sandbox.Sandbox {
	if h.Sandboxes == nil {
		h.Sandboxes = sandbox.NewLocalProvider(svcconfig.GetPaths())
	}
	id, err := h.Sandboxes.Acquire("chat", "default")
	if err != nil {
		return nil
	}
	sb, ok := h.Sandboxes.Get(id)
	if !ok {
		return nil
	}
	return sb
}

func systemForPhase(base *agentctx.Session, b *agentctx.Builder, phase agentctx.Phase) string {
	workDir := ""
	if base != nil {
		workDir = base.WorkDir
	}
	tmp := agentctx.NewSession("", workDir)
	tmp.ApplyPhase(b, phase)
	wire := tmp.WireMessages()
	if len(wire) == 0 {
		return ""
	}
	return wire[0].Content
}

func wireToSchema(wire []agentctx.WireMessage) []schema.Message {
	out := make([]schema.Message, 0, len(wire))
	for _, w := range wire {
		m := schema.Message{Content: w.Content}
		switch strings.ToLower(w.Role) {
		case "system":
			m.Role = schema.RoleSystem
		case "assistant":
			m.Role = schema.RoleAssistant
			if len(w.ToolCalls) > 0 {
				m.ToolCalls = make([]schema.ToolCall, 0, len(w.ToolCalls))
				for _, tc := range w.ToolCalls {
					m.ToolCalls = append(m.ToolCalls, schema.ToolCall{
						ID:        tc.ID,
						Name:      tc.Name,
						Arguments: tc.Arguments,
					})
				}
			}
		case "tool":
			m.Role = schema.RoleTool
			m.ToolCallID = w.ToolCallID
			m.Name = w.Name
		default:
			m.Role = schema.RoleUser
		}
		out = append(out, m)
	}
	return out
}
