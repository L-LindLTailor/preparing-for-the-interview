# Этап сборки
FROM golang:1.25.0-alpine AS builder

# Обязательные зависимости для компиляции C-кода Kafka
RUN apk add --no-cache gcc musl-dev libc-dev librdkafka-dev pkgconf

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# ВКЛЮЧАЕМ CGO и добавляем тег musl
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o autoscaler .

# Этап запуска
FROM alpine:latest
# В финальном образе тоже нужна библиотека librdkafka
RUN apk --no-cache add ca-certificates librdkafka

COPY --from=builder /app/autoscaler /autoscaler
CMD ["/autoscaler"]
