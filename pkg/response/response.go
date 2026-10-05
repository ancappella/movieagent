package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Body struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Body{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

func Fail(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, Body{
		Code:    code,
		Message: message,
	})
}
