// cmd/web/main.go
package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/LuisEduardoPedra/analiseSped/internal/api/handlers"
	"github.com/LuisEduardoPedra/analiseSped/internal/api/middleware"
	"github.com/LuisEduardoPedra/analiseSped/internal/api/responses"
	"github.com/LuisEduardoPedra/analiseSped/internal/core/analysis"
	"github.com/LuisEduardoPedra/analiseSped/internal/core/auth"
	"github.com/LuisEduardoPedra/analiseSped/internal/core/converter"
	"github.com/gin-gonic/gin"
)

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

func loadEnv() {
	file, err := os.Open(".env")
	if err != nil {

		if os.IsNotExist(err) {
			log.Print("Arquivo .env não encontrado, prosseguindo com variáveis de ambiente existentes")
		} else {
			log.Printf("Erro ao carregar .env: %v", err)
		}

		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Erro ao ler .env: %v", err)
	} else {
		log.Print("Variáveis de ambiente carregadas de .env")
	}
}

func main() {
	loadEnv()

	if os.Getenv("JWT_SECRET") == "" {

		log.Fatal("FATAL: Variável de ambiente JWT_SECRET não está configurada.")
	}
	jwtSecretBytes := []byte(jwtSecret)

	responses.InitLogger()
	ctx := context.Background()
	firestoreClient := initFirestoreClient(ctx)
	defer firestoreClient.Close()

	analysisService := analysis.NewService()
	authService := auth.NewService(firestoreClient, jwtSecretBytes)
	converterService := converter.NewService()

	analysisHandler := handlers.NewAnalysisHandler(analysisService)
	authHandler := handlers.NewAuthHandler(authService)
	converterHandler := handlers.NewConverterHandler(converterService)

	allowedOriginsEnv := os.Getenv("ALLOWED_ORIGINS")
	if allowedOriginsEnv == "" {
		allowedOriginsEnv = "https://analise-sped-frontend.vercel.app"
	}
	allowedOrigins := strings.Split(allowedOriginsEnv, ",")

	router := gin.Default()
	router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" && (allowedOriginsEnv == "*" || containsOrigin(allowedOrigins, origin)) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}
		c.Writer.Header().Set("Vary", "Origin")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
		protected.Use(middleware.AuthMiddleware(jwtSecretBytes))
		{
			// Rotas de Análise
			protected.POST("/analyze/icms", middleware.PermissionMiddleware("analise-icms"), analysisHandler.HandleAnalysisIcms)
			protected.POST("/analyze/ipi-st", middleware.PermissionMiddleware("analise-ipi-st"), analysisHandler.HandleAnalysisIpiSt)

			// Rotas de Conversão
			protected.POST("/convert/francesinha", middleware.PermissionMiddleware("converter-francesinha"), converterHandler.HandleSicrediConversion)
			protected.POST("/convert/receitas-acisa", middleware.PermissionMiddleware("converter-receitas-acisa"), converterHandler.HandleReceitasAcisaConversion)
			protected.POST("/convert/atolini-pagamentos", middleware.PermissionMiddleware("converter-atolini-pagamentos"), converterHandler.HandleAtoliniPagamentosConversion)
			protected.POST("/convert/atolini-recebimentos", middleware.PermissionMiddleware("converter-atolini-recebimentos"), converterHandler.HandleAtoliniRecebimentosConversion)
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

func containsOrigin(origins []string, origin string) bool {
	for _, o := range origins {
		if strings.TrimSpace(o) == origin {
			return true
		}
	}
	return false
}
