# ==========================================
# ESTÁGIO 1: Builder (Construção do binário Go)
# ==========================================
FROM golang:alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o scraper cmd/scraper/main.go

# ==========================================
# ESTÁGIO 2: Final (Imagem de Produção Segura K8s Compliant)
# ==========================================
FROM alpine:latest
RUN apk --no-cache add ca-certificates chromium
# ca-certificates para chamadas de rede estritas; para compatibilidade

# Criação estática do usuário não-root (Segurança corporativa K8s Compliant)
RUN addgroup -g 1000 appgroup && \
    adduser -D -u 1000 -G appgroup appuser

WORKDIR /app
COPY --from=builder /app/scraper .

# Criação de pastas com permissões restritas e donos estáticos de UID/GID
RUN mkdir -p data && chown -R appuser:appgroup /app/data
USER appuser

# Exposição da porta de rede para Health Check (Exigência formal do edital)
EXPOSE 8080

CMD ["./scraper"]
