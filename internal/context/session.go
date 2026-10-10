package context

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Session holds ordered conversation history for one agent chat.
// SystemMessage sections are patchable; Replay restores Agent config in order.
type Session struct {
	mu       sync.RWMutex
	ID       string
	WorkDir  string
	Messages []Message

	seq int
}

// NewSession creates an empty session with a blank SystemMessage ready for patches.
func NewSession(id, workDir string) *Session {
	s := &Session{
		ID:      strings.TrimSpace(id),
		WorkDir: strings.TrimSpace(workDir),
	}
	s.Messages = []Message{{
		ID:        s.nextID("sys"),
		Kind:      KindSystem,
		Sections:  map[string]string{},
		CreatedAt: time.Now(),
	}}
	if s.WorkDir != "" {
		s.Messages[0].Sections[SectionWorkDir] = fmt.Sprintf("### 工作目录\n`%s`", s.WorkDir)
	}
	return s
}

func (s *Session) nextID(prefix string) string {
	s.seq++
	return prefix + "-" + strconv.Itoa(s.seq)
}

func (s *Session) systemMsg() *Message {
	if len(s.Messages) == 0 || s.Messages[0].Kind != KindSystem {
		s.Messages = append([]Message{{
			ID:        s.nextID("sys"),
			Kind:      KindSystem,
			Sections:  map[string]string{},
			CreatedAt: time.Now(),
		}}, s.Messages...)
	}
	if s.Messages[0].Sections == nil {
		s.Messages[0].Sections = map[string]string{}
	}
	return &s.Messages[0]
}

// SetSection patches a SystemMessage section (create or replace).
func (s *Session) SetSection(name, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	sys := s.systemMsg()
	body = strings.TrimSpace(body)
	if body == "" {
		delete(sys.Sections, name)
	} else {
		sys.Sections[name] = body
	}
	sys.Content = RenderSystem(sys.Sections)
	sys.CreatedAt = time.Now()
}

// DeleteSection removes a SystemMessage section.
func (s *Session) DeleteSection(name string) {
	s.SetSection(name, "")
}

// SetTools replaces the tools section from a tool list.
func (s *Session) SetTools(tools []ToolInfo) {
	s.SetSection(SectionTools, formatTools(tools))
}

// ApplyPhase loads Builder output into system sections for Thinking or Action.
func (s *Session) ApplyPhase(b *Builder, phase Phase) {
	if b == nil {
		return
	}
	s.SetSection(SectionInstructions, b.loadSystemPrompt())
	if s.WorkDir != "" {
		s.SetSection(SectionWorkDir, fmt.Sprintf("### 工作目录\n`%s`", s.WorkDir))
	}
	s.SetSection(SectionSchedule, ImageSkillSchedule())

	if phase == PhaseAction {
		s.SetSection(SectionSkills, b.loadSkills())
		s.SetTools(b.tools)
	} else {
		// First Thinking: lean — drop skills/tools so the model plans without tool noise.
		s.DeleteSection(SectionSkills)
		s.DeleteSection(SectionTools)
	}
	if len(b.extras) > 0 {
		s.SetSection(SectionExtra, strings.Join(b.extras, "\n\n"))
	}
}

// AppendUser adds a UserMessage (plain text and/or multimodal parts).
func (s *Session) AppendUser(text string, parts ...ContentPart) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := Message{
		ID:        s.nextID("user"),
		Kind:      KindUser,
		Content:   strings.TrimSpace(text),
		Parts:     append([]ContentPart(nil), parts...),
		CreatedAt: time.Now(),
	}
	s.Messages = append(s.Messages, m)
}

// AppendAssistant adds an AssistantMessage (reply text + optional thinking + toolCalls).
func (s *Session) AppendAssistant(content, thinking string, calls ...ToolCallReq) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, Message{
		ID:        s.nextID("asst"),
		Kind:      KindAssistant,
		Content:   strings.TrimSpace(content),
		Thinking:  strings.TrimSpace(thinking),
		ToolCalls: append([]ToolCallReq(nil), calls...),
		CreatedAt: time.Now(),
	})
}

// AppendToolResult adds a ToolResultMessage for one toolCall.
func (s *Session) AppendToolResult(toolCallID, toolName, result string, isError bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, Message{
		ID:         s.nextID("tool"),
		Kind:       KindToolResult,
		ToolCallID: strings.TrimSpace(toolCallID),
		ToolName:   strings.TrimSpace(toolName),
		Content:    result,
		IsError:    isError,
		CreatedAt:  time.Now(),
	})
}

// Replay returns a deep copy of messages in order (for restoring Agent config).
func (s *Session) Replay() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Message, len(s.Messages))
	for i, m := range s.Messages {
		out[i] = cloneMessage(m)
	}
	return out
}

// WireMessage is a provider-ready transcript row (no claw import).
type WireMessage struct {
	Role       string
	Content    string
	ToolCalls  []ToolCallReq
	ToolCallID string
	Name       string
}

// WireMessages flattens session history into provider chat messages for one turn.
// System sections are re-rendered so patches are always current.
func (s *Session) WireMessages() []WireMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]WireMessage, 0, len(s.Messages))
	for _, m := range s.Messages {
		switch m.Kind {
		case KindSystem:
			content := RenderSystem(m.Sections)
			if content == "" {
				content = strings.TrimSpace(m.Content)
			}
			if content == "" {
				continue
			}
			out = append(out, WireMessage{Role: "system", Content: content})
		case KindUser:
			text := m.UserText()
			if text == "" {
				continue
			}
			out = append(out, WireMessage{Role: "user", Content: text})
		case KindAssistant:
			out = append(out, WireMessage{
				Role:      "assistant",
				Content:   m.AssistantText(),
				ToolCalls: append([]ToolCallReq(nil), m.ToolCalls...),
			})
		case KindToolResult:
			out = append(out, WireMessage{
				Role:       "tool",
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
				Name:       m.ToolName,
			})
		}
	}
	return out
}

// IngestClientHistory maps prior client turns (user/assistant/system text) into the session.
// Does not replace the leading SystemMessage; client "system" lines become Extra patches.
func (s *Session) IngestClientHistory(role, content string) {
	role = strings.ToLower(strings.TrimSpace(role))
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	switch role {
	case "assistant":
		s.AppendAssistant(content, "")
	case "system":
		s.mu.Lock()
		sys := s.systemMsg()
		prev := strings.TrimSpace(sys.Sections[SectionExtra])
		if prev != "" {
			prev += "\n\n"
		}
		sys.Sections[SectionExtra] = prev + content
		sys.Content = RenderSystem(sys.Sections)
		s.mu.Unlock()
	default:
		s.AppendUser(content)
	}
}

func cloneMessage(m Message) Message {
	cp := m
	if m.Sections != nil {
		cp.Sections = make(map[string]string, len(m.Sections))
		for k, v := range m.Sections {
			cp.Sections[k] = v
		}
	}
	if m.Parts != nil {
		cp.Parts = append([]ContentPart(nil), m.Parts...)
	}
	if m.ToolCalls != nil {
		cp.ToolCalls = append([]ToolCallReq(nil), m.ToolCalls...)
	}
	return cp
}

// ImageSkillSchedule explains how to dispatch picturebook image/video skills each Action turn.
func ImageSkillSchedule() string {
	return strings.TrimSpace(`### 生图 / 动画 Skill 调度（每轮必读）

当用户要求画图 / 插画 / 绘本 / 出图 / 生成图片时，必须走 skill **picturebook-illustrate**：

1. **匹配**：用户意图命中绘本水彩彩铅、儿童插画、或明确要图 → 选用该 skill（不要只用文字假装已出图）。
2. **调用**：在本轮 Action 发出 toolCall：` + "`picturebook_illustrate`" + `。可同时写 content 方案描述；运行时会后台出图，不必等成图再写字。
3. **参数**：写清 ` + "`prompt`" + `（具体场景与角色）；比例用 ` + "`aspect_ratio`" + `（绘本插页默认 2:3；竖版封面常用 3:4；短视频竖图 9:16）。
4. **禁止**：不要改走其它出图通道；不要向用户暴露工具名与脚本路径；后台生成中不要谎称「图已画完」。
5. **回填**：若 ToolResult 提示生成已在后台开始，立刻输出简短方案描述；图片由系统稍后单独推送。

当用户要求把插画做成动画 / 动起来 / 出短视频 / 旋转 时，走 skill **picturebook-animate**：

1. **写程序**：先产出可运行的 Canvas JS（文本代码）：实现 ` + "`draw(t, ctx, canvas, assets)`" + `（` + "`t`" + ` 为秒）；可选 ` + "`seek(t)`" + ` / ` + "`setup`" + `。有静图时用 ` + "`assets.image`" + `。
2. **输入图**：优先 ` + "`/mnt/user-data/outputs/picturebook.png`" + `；没有图时先 ` + "`picturebook_illustrate`" + ` 再动画。
3. **调用**：toolCall ` + "`picturebook_animate`" + `，把代码放进 ` + "`code`" + `，并设 ` + "`duration_sec`" + `（从用户需求解析，默认 4）；可同时写动效说明。运行时 Playwright 逐帧求值 → ffmpeg 合成 MP4。
4. **兜底**：暂时写不出代码时可省略 ` + "`code`" + `，设 ` + "`template`" + ` 为 character_spin 或 character_idle。
5. **回填**：ToolResult 提示动画已在后台开始时，立刻写简短动效说明；视频由系统稍后单独推送。`)
}
