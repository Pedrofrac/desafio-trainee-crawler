FROM golang:alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o scraper main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates su-exec
WORKDIR /app
COPY --from=builder /app/scraper .
COPY entrypoint.sh .
RUN chmod +x entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["./scraper"]
