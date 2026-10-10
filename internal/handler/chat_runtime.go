package handler

import (
	"net/http"
	"strings"

	"movieagent/internal/service/agentruntime"
	"movieagent/internal/service/chathub"
	"movieagent/internal/service/task"
	"movieagent/pkg/response"

	"github.com/gin-gonic/gin"
)

// PauseTask checkpoints a running chat task for later resume.
func (h *ChatHandler) PauseTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.Fail(c, http.StatusBadRequest, 1, "task id required")
		return
	}
	home := h.runtimeHome()
	sched := task.NewScheduler(task.NewFileStore(home))
	got, err := sched.Pause(c.Request.Context(), taskID)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, 1, err.Error())
		return
	}
	cpStore := agentruntime.NewFileCheckpointer(home).WithTaskStore(sched.Store)
	full, _ := cpStore.Load(c.Request.Context(), taskID)
	if full != nil {
		full.Phase = agentruntime.PhaseCPAction
		if full.Continuation.Kind == "" || full.Continuation.Kind == "may_finish" {
			full.Continuation = agentruntime.Continuation{Kind: "need_action"}
		}
		_ = cpStore.Save(c.Request.Context(), full)
	}
	h.hub().Publish(taskID, chathub.Event{Type: "task", Status: string(task.StatusPaused)})
	response.OK(c, gin.H{
		"task_id":    got.ID,
		"status":     got.Status,
		"checkpoint": full,
	})
}

// ResumeTask loads the durable checkpoint plan and marks the task running again.
// Full engine replay of incomplete tools is left to a subsequent worker; this
// endpoint exposes the resume plan and settles already-done checkpoints.
func (h *ChatHandler) ResumeTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.Fail(c, http.StatusBadRequest, 1, "task id required")
		return
	}
	home := h.runtimeHome()
	sched := task.NewScheduler(task.NewFileStore(home))
	got, light, err := sched.Resume(c.Request.Context(), taskID)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, 1, err.Error())
		return
	}
	cpStore := agentruntime.NewFileCheckpointer(home).WithTaskStore(sched.Store)
	full, _ := cpStore.Load(c.Request.Context(), taskID)
	plan := agentruntime.PlanResume(full)

	if plan.SkipDone {
		_, _ = sched.Finish(c.Request.Context(), taskID, map[string]any{"resumed": true, "already_done": true}, nil)
		h.hub().Publish(taskID, chathub.Event{Type: "task", Status: string(task.StatusSucceeded)})
		response.OK(c, gin.H{
			"task_id":    got.ID,
			"status":     task.StatusSucceeded,
			"plan":       "already_done",
			"checkpoint": full,
			"light":      light,
		})
		return
	}

	h.hub().Publish(taskID, chathub.Event{Type: "task", Status: string(task.StatusRunning)})
	response.OK(c, gin.H{
		"task_id":     got.ID,
		"status":      got.Status,
		"plan":        "continue",
		"replay_safe": plan.ReplaySafe,
		"blocked":     plan.Blocked,
		"checkpoint":  full,
		"light":       light,
		"events":      "/api/v1/chat/tasks/" + taskID + "/events",
	})
}

// GetCheckpoint returns the durable AgentCheckpoint for a chat task.
func (h *ChatHandler) GetCheckpoint(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.Fail(c, http.StatusBadRequest, 1, "task id required")
		return
	}
	home := h.runtimeHome()
	cp, err := agentruntime.NewFileCheckpointer(home).Load(c.Request.Context(), taskID)
	if err != nil {
		response.Fail(c, http.StatusNotFound, 1, "checkpoint not found")
		return
	}
	response.OK(c, gin.H{
		"checkpoint": cp,
		"plan":       agentruntime.PlanResume(cp),
	})
}

// GetRuntimeSnapshot returns context/memory/infer/checkpoint for a task.
func (h *ChatHandler) GetRuntimeSnapshot(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.Fail(c, http.StatusBadRequest, 1, "task id required")
		return
	}
	home := h.runtimeHome()
	rt, err := agentruntime.New(agentruntime.Options{
		Root:      home,
		SessionID: "chat",
		ThreadID:  "chat",
		TaskID:    taskID,
		WorkDir:   h.ProjectRoot,
	})
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	snap, err := rt.Snapshot(c.Request.Context())
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	response.OK(c, snap)
}
