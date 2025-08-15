// cmd/web/main.go
package main

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"github.com/LuisEduardoPedra/analiseSped/internal/api/handlers"
	"github.com/LuisEduardoPedra/analiseSped/internal/api/middleware" // Importa o middleware
	"github.com/LuisEduardoPedra/analiseSped/internal/core/analysis"
	"github.com/LuisEduardoPedra/analiseSped/internal/core/auth" // Importa o serviço de auth
	"github.com/gin-gonic/gin"
	"google.golang.org/api/option"
)

// Função para iniciar o cliente do Firestore
func initFirestoreClient(ctx context.Context) *firestore.Client {
	sa := option.WithCredentialsFile("credentials.json")
	app, err := firebase.NewApp(ctx, nil, sa)
	if err != nil {
		log.Fatalf("Erro ao inicializar app Firebase: %v\n", err)
	}

	client, err := app.Firestore(ctx)
	if err != nil {
		log.Fatalf("Erro ao inicializar cliente Firestore: %v\n", err)
	}
	return client
}

func main() {
	ctx := context.Background()
	firestoreClient := initFirestoreClient(ctx)
	defer firestoreClient.Close() // Garante que a conexão será fechada ao sair.

	// --- 1. Inicialização das Dependências ---
	analysisService := analysis.NewService()
	authService := auth.NewService(firestoreClient) // Cria o serviço de auth

	analysisHandler := handlers.NewAnalysisHandler(analysisService)
	authHandler := handlers.NewAuthHandler(authService) // Cria o handler de auth

	// --- 2. Configuração do Servidor Web (Gin) ---
	router := gin.Default()
	router.Use(func(c *gin.Context) { // Middleware de CORS
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// --- 3. Definição das Rotas da API ---
	apiV1 := router.Group("/api/v1")
	{
		// Rota pública para login
		apiV1.POST("/login", authHandler.Login)

		// Cria um novo grupo de rotas que usa o middleware de autenticação
		protected := apiV1.Group("/")
		protected.Use(middleware.AuthMiddleware())
		{
			// A rota de análise agora está dentro do grupo protegido
			protected.POST("/analyze", analysisHandler.HandleAnalysis)
		}
	}

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "UP"})
	})

	// --- 4. Inicialização do Servidor ---
	port := ":8080"
	log.Printf("🚀 Servidor iniciado e escutando na porta %s", port)
	if err := router.Run(port); err != nil {
		log.Fatal("Falha ao iniciar o servidor: ", err)
	}
}
