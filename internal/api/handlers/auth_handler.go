// internal/api/handlers/auth_handler.go
package handlers

import (
	"net/http"

	"github.com/LuisEduardoPedra/analiseSped/internal/core/auth"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	service auth.Service
}

func NewAuthHandler(service auth.Service) *AuthHandler {
	return &AuthHandler{service: service}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Requisição inválida"})
		return
	}

	token, err := h.service.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}
