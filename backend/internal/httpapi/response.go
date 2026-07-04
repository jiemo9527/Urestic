package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type response struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
	Error   *err   `json:"error,omitempty"`
}

type err struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ok(c *gin.Context, data any) {
	okStatus(c, http.StatusOK, data)
}

func okStatus(c *gin.Context, status int, data any) {
	c.JSON(status, response{Success: true, Data: data, Message: "ok"})
}

func fail(c *gin.Context, status int, code string, message string) {
	c.JSON(status, response{Success: false, Error: &err{Code: code, Message: message}})
}
