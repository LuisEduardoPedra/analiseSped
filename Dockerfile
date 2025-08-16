# --- Estágio 1: Build ---
# Usa a imagem oficial do Go para compilar a aplicação
FROM golang:1.25-alpine AS builder

# Define o diretório de trabalho dentro do container
WORKDIR /app

# Copia os arquivos de gerenciamento de dependências
COPY go.mod go.sum ./

# Baixa as dependências
RUN go mod download

# Copia todo o código fonte da aplicação
COPY . .

# Compila a aplicação
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /server ./cmd/web/main.go


# --- Estágio 2: Final (SEÇÃO CORRIGIDA) ---
# Usa a imagem Alpine, que é mínima mas contém os certificados SSL necessários.
FROM alpine:latest

# Define o diretório de trabalho
WORKDIR /root/

# Copia o binário compilado do estágio 'builder'
COPY --from=builder /server .

# Expõe a porta que a nossa aplicação usa
EXPOSE 8080

# Comando para executar a aplicação quando o container iniciar
CMD ["./server"]