# Этап сборки
FROM golang:1.24.3-alpine AS builder

# 1. Добавляем инструменты для сборки C-зависимостей (нужно для Kafka)
RUN apk add --no-cache gcc musl-dev libc-dev librdkafka-dev pkgconf

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 2. Включаем CGO и добавляем тег musl для совместимости с Alpine
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o topic-horizontal-and-vertical-scaling .

# Этап запуска
FROM alpine:latest

# 3. В финальном образе ОБЯЗАТЕЛЬНО должна быть библиотека librdkafka и curl для healthcheck
RUN apk --no-cache add ca-certificates librdkafka curl

WORKDIR /root/
COPY --from=builder /app/topic-horizontal-and-vertical-scaling .

EXPOSE 8080
CMD ["./topic-horizontal-and-vertical-scaling"]
