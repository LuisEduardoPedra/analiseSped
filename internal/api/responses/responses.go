// internal/api/responses/responses.go
package responses

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var logger *zap.Logger

// APIResponse defines the standard envelope for API responses.
type APIResponse struct {
	Status  string      `json:"status"` // "success" or "error"
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}

// InitLogger initializes the structured logger for API responses.
func InitLogger() {
	logger, _ = zap.NewProduction()
}

// Success sends a successful response with the provided data and message.
func Success(c *gin.Context, data interface{}, message string) {
	resp := APIResponse{Status: "success", Data: data, Message: message}
	c.JSON(http.StatusOK, resp)
	logger.Info("API success", zap.String("path", c.Request.URL.Path), zap.Int("status", http.StatusOK))
}

// Error sends an error response with the provided code, message, and optional errors.
func Error(c *gin.Context, code int, message string, errs ...string) {
	resp := APIResponse{Status: "error", Message: message, Errors: errs}
	c.JSON(code, resp)
	logger.Error("API error", zap.String("path", c.Request.URL.Path), zap.Int("status", code), zap.Strings("errors", errs))
}
