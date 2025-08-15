# --- Estágio 1: Build ---
# Usa a imagem oficial do Go para compilar a aplicação
FROM golang:1.22-alpine AS builder

# Define o diretório de trabalho dentro do container
WORKDIR /app

# Copia os arquivos de gerenciamento de dependências
COPY go.mod go.sum ./

# Baixa as dependências. Este passo é separado para aproveitar o cache do Docker.
RUN go mod download

# Copia todo o código fonte da aplicação
COPY . .

# Compila a aplicação. CGO_ENABLED=0 cria um binário estático.
# -o /server especifica o nome e local do arquivo de saída.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /server ./cmd/web/main.go


# --- Estágio 2: Final ---
# Usa uma imagem base mínima (scratch) para a imagem final
FROM scratch

# Define o diretório de trabalho
WORKDIR /root/

# Copia o binário compilado do estágio 'builder'
COPY --from=builder /server .

# Copia o arquivo de credenciais do Firestore.
# O Dockerfile DEVE estar no mesmo diretório que o credentials.json durante o build.
COPY credentials.json .

# Expõe a porta que a nossa aplicação usa
EXPOSE 8080

# Comando para executar a aplicação quando o container iniciar
CMD ["./server"]