// cmd/web/main.go
package main

import (
	"context"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	"github.com/LuisEduardoPedra/analiseSped/internal/api/handlers"
	"github.com/LuisEduardoPedra/analiseSped/internal/api/middleware"
	"github.com/LuisEduardoPedra/analiseSped/internal/api/responses"
	"github.com/LuisEduardoPedra/analiseSped/internal/core/analysis"
	"github.com/LuisEduardoPedra/analiseSped/internal/core/auth"
	"github.com/gin-gonic/gin"
)

// initFirestoreClient initializes the Firestore client.
func initFirestoreClient(ctx context.Context) *firestore.Client {
	projectID := "analise-sped-db"
	databaseID := "analise-sped-db"
	client, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		log.Fatalf("Erro ao inicializar cliente Firestore para o banco '%s': %v\n", databaseID, err)
	}
	log.Printf("Conectado com sucesso ao Firestore, banco de dados: %s", databaseID)
	return client
}

func main() {
	// Verifica se a variável de ambiente JWT_SECRET está configurada
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("FATAL: Variável de ambiente JWT_SECRET não está configurada.")
	}

	responses.InitLogger()
	ctx := context.Background()
	firestoreClient := initFirestoreClient(ctx)
	defer firestoreClient.Close()
	analysisService := analysis.NewService()
	authService := auth.NewService(firestoreClient)
	analysisHandler := handlers.NewAnalysisHandler(analysisService)
	authHandler := handlers.NewAuthHandler(authService)

	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	apiV1 := router.Group("/api/v1")
	{
		apiV1.POST("/login", authHandler.Login)
		protected := apiV1.Group("/")
		// 1. Aplica a autenticação geral para todas as rotas do grupo
		protected.Use(middleware.AuthMiddleware())
		{
			// 2. Aplica a verificação de permissão específica para cada rota
			protected.POST("/analyze/icms", middleware.PermissionMiddleware("analise-icms"), analysisHandler.HandleAnalysisIcms)
			protected.POST("/analyze/ipi-st", middleware.PermissionMiddleware("analise-ipi-st"), analysisHandler.HandleAnalysisIpiSt)
		}
	}
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "UP"})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Servidor iniciado e escutando na porta %s", port)

	if err := router.Run(":" + port); err != nil {
		log.Fatal("Falha ao iniciar o servidor: ", err)
	}
}
