// cmd/web/main.go
package main

import (
	"context"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	"github.com/LuisEduardoPedra/analiseSped/internal/api/handlers"
	"github.com/LuisEduardoPedra/analiseSped/internal/api/middleware" // Importa o middleware
	"github.com/LuisEduardoPedra/analiseSped/internal/core/analysis"
	"github.com/LuisEduardoPedra/analiseSped/internal/core/auth" // Importa o serviço de auth
	"github.com/gin-gonic/gin"
)

func initFirestoreClient(ctx context.Context) *firestore.Client {
	// O projectID será detectado automaticamente do ambiente do Google Cloud.
	projectID := "analise-sped-db"

	// O databaseID é o nome que você deu ao seu novo banco de dados.
	databaseID := "analise-sped-db"

	// Usamos NewClientWithDatabase para conectar a um banco de dados nomeado.
	// Esta é a função correta.
	client, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		log.Fatalf("Erro ao inicializar cliente Firestore para o banco '%s': %v\n", databaseID, err)
	}

	log.Printf("Conectado com sucesso ao Firestore, banco de dados: %s", databaseID)
	return client
}

func main() {
	ctx := context.Background()
	firestoreClient := initFirestoreClient(ctx)
	defer firestoreClient.Close()

	// A partir daqui, o resto da função main continua exatamente igual.
	// ... (código inalterado) ...
	analysisService := analysis.NewService()
	// O serviço de autenticação precisa do firestoreClient, que agora está sendo
	// inicializado corretamente.
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
		protected.Use(middleware.AuthMiddleware())
		{
			protected.POST("/analyze", analysisHandler.HandleAnalysis)
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
