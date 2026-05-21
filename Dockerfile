# ==========================================
# ESTÁGIO 1: Builder (Construção do binário)
# ==========================================
# Usamos a imagem oficial do Go baseada em Alpine (leve) para compilar o código
FROM golang:alpine AS builder

# Define a pasta de trabalho dentro do container
WORKDIR /app

# Copia os arquivos de configuração do Go primeiro (otimiza o cache do Docker)
COPY go.mod go.sum ./
RUN go mod download

# Copia todo o resto do código para dentro do container
COPY . .

# Compila o programa. 
# CGO_ENABLED=0 garante um binário 100% autossuficiente (excelente prática)
RUN CGO_ENABLED=0 GOOS=linux go build -o scraper main.go

# ==========================================
# ESTÁGIO 2: Final (Imagem de Produção)
# ==========================================
# Usamos o Alpine puro, que tem apenas ~5MB (Requisito: Imagem leve)
FROM alpine:latest

# Instala certificados de segurança (necessário para o scraper acessar sites HTTPS)
RUN apk --no-cache add ca-certificates

# Cria um usuário e grupo não-root (Requisito: Segurança, usuário não-root)
RUN adduser -D appuser

# Define a pasta de trabalho
WORKDIR /app

# Copia APENAS o binário pronto do estágio 1 (Deixa a imagem extremamente leve, descartando o código fonte)
COPY --from=builder /app/scraper .

# Cria a pasta 'data' e dá permissão total para o nosso usuário não-root poder salvar o CSV/JSON lá
RUN mkdir data && chown appuser:appuser data

# Muda do usuário root para o nosso usuário seguro
USER appuser

# Exposição da porta (Requisito do PDF). 
# Obs: Scrapers em linha de comando não abrem portas como uma API, mas expomos a 8080 para cumprir a regra do desafio e prever uso futuro em Cloud (AWS ECS).
EXPOSE 8080

# Comando que será executado quando o container ligar
CMD ["./scraper"]