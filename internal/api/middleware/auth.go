// internal/api/middleware/auth.go
package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddleware verifica se o token JWT é válido.
func AuthMiddleware(jwtSecret []byte) gin.HandlerFunc {
	if len(jwtSecret) == 0 {
		if env := os.Getenv("JWT_SECRET"); env != "" {
			jwtSecret = []byte(env)
		}
	}
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token de autorização não fornecido"})
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Formato do token inválido"})
			return
		}

		tokenString := parts[1]
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("método de assinatura inesperado: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token inválido ou expirado"})
			return
		}

		// Armazena os claims no contexto para uso posterior, se necessário
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			c.Set("user_claims", claims)
		}

		c.Next()
	}
}

// PermissionMiddleware verifica se o usuário tem uma permissão específica.
func PermissionMiddleware(requiredPermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Pega os claims do token que foram validados pelo AuthMiddleware
		claims, exists := c.Get("user_claims")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Claims do usuário não encontrados"})
			return
		}

		mapClaims := claims.(jwt.MapClaims)
		roles, ok := mapClaims["roles"].([]interface{})
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Permissões não encontradas no token"})
			return
		}

		// Verifica se a permissão necessária está na lista de permissões do usuário
		for _, role := range roles {
			if roleStr, ok := role.(string); ok && roleStr == requiredPermission {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Acesso negado: permissão necessária ausente"})
	}
}
