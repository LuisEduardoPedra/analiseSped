// internal/core/auth/service.go
package auth

import (
	"context"
	"errors"
	"log"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/iterator"
)

// Defina uma chave secreta para assinar os tokens.
// Em produção, isso DEVE vir de uma variável de ambiente, nunca hardcoded.
var jwtSecret = []byte("SUA_CHAVE_SUPER_SECRETA_MUDE_ISSO")

type Service interface {
	Login(ctx context.Context, username, password string) (string, error)
}

type service struct {
	db *firestore.Client
}

func NewService(db *firestore.Client) Service {
	return &service{db: db}
}

// User representa a estrutura de um usuário no Firestore.
type User struct {
	Username     string `firestore:"username"`
	PasswordHash string `firestore:"passwordHash"`
}

func (s *service) Login(ctx context.Context, username, password string) (string, error) {
	// 1. Encontrar o usuário no Firestore.
	query := s.db.Collection("users").Where("username", "==", username).Limit(1).Documents(ctx)
	defer query.Stop()

	doc, err := query.Next()
	if err == iterator.Done {
		return "", errors.New("usuário ou senha inválidos")
	}
	if err != nil {
		log.Printf("Erro detalhado do Firestore: %v", err)

		// 3. A mensagem para o usuário continua a mesma
		return "", errors.New("erro ao consultar o banco de dados")
	}

	var user User
	if err := doc.DataTo(&user); err != nil {
		return "", errors.New("erro ao ler dados do usuário")
	}

	// 2. Comparar a senha fornecida com o hash armazenado.
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		// Se err != nil, a senha está incorreta.
		return "", errors.New("usuário ou senha inválidos")
	}

	// 3. Gerar o Token JWT.
	claims := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": user.Username,
		"exp":      time.Now().Add(time.Hour * 24).Unix(), // Token expira em 24 horas
	})

	tokenString, err := claims.SignedString(jwtSecret)
	if err != nil {
		return "", errors.New("erro ao gerar token de acesso")
	}

	return tokenString, nil
}
