package handler

import (
	"movieagent/pkg/response"

	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	AppName string
}

func NewHealthHandler(appName string) *HealthHandler {
	return &HealthHandler{AppName: appName}
}

func (h *HealthHandler) Ping(c *gin.Context) {
	response.OK(c, gin.H{
		"app":    h.AppName,
		"status": "up",
	})
}
