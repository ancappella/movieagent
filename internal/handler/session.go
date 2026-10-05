package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"movieagent/internal/model"
	"movieagent/internal/service/agent_loop"
	"movieagent/internal/service/agents/brandagent"
	"movieagent/pkg/response"

	"github.com/gin-gonic/gin"
)

// CreateRobotSession opens a robot session for a conversation (pi-agent session).
func (h *StudioHandler) CreateRobotSession(c *gin.Context) {
	if h.Sessions == nil {
		h.Sessions = agent_loop.NewHub()
	}
	var body struct {
		AgentID string `json:"agent_id"`
		Skill   string `json:"skill"`
	}
	_ = c.ShouldBindJSON(&body)
	agentID := strings.TrimSpace(body.AgentID)
	skill := strings.TrimSpace(body.Skill)
	if skill == "" {
		skill = defaultSkillForAgent(agentID)
	}
	sess := h.Sessions.Open(agentID, skill)
	response.OK(c, gin.H{
		"session_id": sess.ID,
		"agent_id":   sess.AgentID,
		"skill_id":   sess.SkillID,
		"created_at": sess.CreatedAt,
	})
}

// GetRobotSession returns the transcript snapshot.
func (h *StudioHandler) GetRobotSession(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	sess, ok := h.Sessions.Get(id)
	if !ok {
		response.Fail(c, http.StatusNotFound, 1, "session not found")
		return
	}
	snap := sess.Snapshot()
	response.OK(c, gin.H{
		"session_id": snap.ID,
		"agent_id":   snap.AgentID,
		"skill_id":   snap.SkillID,
		"messages":   snap.Messages,
		"last_data":  snap.LastData,
		"updated_at": snap.UpdatedAt,
	})
}

// PromptRobotSession runs one pi-agent prompt() and streams NDJSON events.
func (h *StudioHandler) PromptRobotSession(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	sess, ok := h.Sessions.Get(id)
	if !ok {
		response.Fail(c, http.StatusNotFound, 1, "session not found")
		return
	}
	var body struct {
		Prompt   string        `json:"prompt"`
		Skill    string        `json:"skill"`
		BrandID  string        `json:"brand_id"`
		AssetIDs []string      `json:"asset_ids"`
		Elements []taskElement `json:"elements"`
		Mode     string        `json:"mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, http.StatusBadRequest, 1, "invalid json")
		return
	}
	body.Prompt = strings.TrimSpace(body.Prompt)
	if body.Prompt == "" {
		response.Fail(c, http.StatusBadRequest, 1, "prompt required")
		return
	}
	skillID := strings.TrimSpace(body.Skill)
	if skillID == "" {
		skillID = sess.SkillID
	}
	if skillID == "" {
		skillID = defaultSkillForAgent(sess.AgentID)
	}
	sess.SetSkill(skillID)
	kind := resolveSkillKind(skillID)
	skillLabel := skillDisplayName(skillID)

	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)
	enc := json.NewEncoder(c.Writer)
	emit := func(ev agent_loop.Event) {
		_ = enc.Encode(ev)
		if flusher != nil {
			flusher.Flush()
		}
	}

	ctx := c.Request.Context()
	work := func(ctx context.Context, s *agent_loop.Session, prompt string, emit agent_loop.Emit) (map[string]any, error) {
		if kind != "poster" {
			return nil, fmt.Errorf("unsupported skill: %s", skillID)
		}
		agent_loop.EmitSkillStart(emit, skillID, skillLabel)
		onNode := func(n brandagent.WorkflowNode) {
			switch n.ID {
			case "brief", "creative_design":
				if n.Status == "run" {
					agent_loop.EmitThinking(emit, "正在"+n.Name+"…")
					return
				}
				line := n.Name
				if n.Detail != "" {
					line = n.Name + "：" + n.Detail
				}
				agent_loop.EmitThinking(emit, line)
			default:
				if n.Status == "run" {
					agent_loop.EmitToolStart(emit, n.ID, n.Name, map[string]any{"model": n.Model})
					return
				}
				agent_loop.EmitToolEnd(emit, n.ID, n.Name, gin.H{
					"status": n.Status,
					"model":  n.Model,
					"detail": n.Detail,
					"name":   n.Name,
					"id":     n.ID,
				}, n.Status == "error")
			}
		}
		_, payload, err := h.executeBrandPoster(ctx, prompt, body.BrandID, body.AssetIDs, body.Elements, body.Mode, s.ID, onNode)
		if err != nil {
			agent_loop.EmitSkillEnd(emit, skillID, skillLabel, gin.H{"status": "error"}, true)
			return nil, err
		}
		if t, ok := payload["creative_text"].(string); ok && t != "" {
			agent_loop.EmitThinking(emit, t)
		}
		agent_loop.EmitSkillEnd(emit, skillID, skillLabel, gin.H{"status": "ok"}, false)
		return map[string]any(payload), nil
	}

	_ = agent_loop.RunPrompt(ctx, sess, body.Prompt, emit, work)
}

func defaultSkillForAgent(agentID string) string {
	card, err := model.Default.Lookup(agentID)
	if err == nil {
		if card.Extra != nil {
			if s, ok := card.Extra["skill"].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		if len(card.Skills) > 0 {
			return card.Skills[0].ID
		}
	}
	return "poster-workflow"
}

func resolveSkillKind(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "poster-workflow", "poster", "海报工作流", "海报工作室", "brand-tone", "品牌调性":
		return "poster"
	default:
		return ""
	}
}

func skillDisplayName(id string) string {
	switch strings.TrimSpace(id) {
	case "brand-tone", "品牌调性":
		return "品牌调性"
	case "poster-workflow", "poster", "海报工作流", "海报工作室":
		return "海报工作流"
	default:
		return id
	}
}
