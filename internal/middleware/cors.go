package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS allows the Next.js chat-ui (dev on :3000) to call the Go API / SSE
// directly, bypassing Next rewrites that drop long-lived event streams.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if allowChatUIOrigin(origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization")
			c.Header("Access-Control-Expose-Headers", "Content-Type")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func allowChatUIOrigin(origin string) bool {
	switch origin {
	case "http://127.0.0.1:3000", "http://localhost:3000":
		return true
	default:
		return false
	}
}
