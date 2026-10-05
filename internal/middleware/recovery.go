package middleware

import (
	"net/http"

	"movieagent/pkg/response"

	"github.com/gin-gonic/gin"
)

func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		response.Fail(c, http.StatusInternalServerError, 500, "internal server error")
		c.Abort()
	})
}
