# Этап сборки
FROM golang:1.24.3-alpine AS builder

# Устанавливаем зависимости для компиляции C-кода
RUN apk add --no-cache gcc musl-dev libc-dev librdkafka-dev pkgconf

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Собираем с поддержкой CGO и динамической линковкой к librdkafka
RUN go build -tags musl -o producer .

# Этап запуска
FROM alpine:latest

# В финальном образе тоже нужна librdkafka для работы бинарника
RUN apk add --no-cache librdkafka ca-certificates

WORKDIR /root/
COPY --from=builder /app/producer .

CMD ["./producer"]
