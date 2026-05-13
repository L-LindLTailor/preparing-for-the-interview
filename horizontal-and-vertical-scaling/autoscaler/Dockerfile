# ==========================================
# ЭТАП СБОРКИ / BUILD STAGE
# ==========================================

# Используем официальный образ Go на базе Alpine Linux. Стейдж называется "builder".
# Use the official Go image based on Alpine Linux. The stage is named "builder".
FROM golang:1.25.0-alpine AS builder

# Устанавливаем системные пакеты, необходимые для компиляции Си-библиотек (требуется для Kafka).
# --no-cache уменьшает размер слоя, не сохраняя скачанные индексы пакетов.
# Install system packages required to compile C libraries (needed for Kafka).
# --no-cache minimizes layer size by not saving downloaded package indexes.
RUN apk add --no-cache gcc musl-dev libc-dev librdkafka-dev pkgconf

# Устанавливаем рабочую директорию внутри контейнера, где будут выполняться последующие команды.
# Set the working directory inside the container where subsequent commands will run.
WORKDIR /app

# Копируем файлы описания зависимостей проекта для их изолированного кэширования.
# Copy the project dependency definition files to enable isolated layering and caching.
COPY go.mod go.sum ./

# Скачиваем все зависимости, указанные в go.mod. Этот шаг закэшируется и не будет перезапускаться, если зависимости не менялись.
# Download all dependencies specified in go.mod. This step is cached and won't rerun unless dependencies change.
RUN go mod download

# Копируем весь исходный код проекта из локальной папки в рабочую директорию контейнера.
# Copy all project source code from the local directory to the container's working directory.
COPY . .

# Компилируем приложение:
# CGO_ENABLED=1 — принудительно включаем поддержку CGO для связывания Go с Си-библиотекой librdkafka.
# GOOS=linux — указываем целевую операционную систему Linux.
# -tags musl — указываем компилятору линковаться со специфичной для Alpine библиотекой мусл вместо glibc.
# -o autoscaler . — сохраняем скомпилированный бинарник под именем "autoscaler" в текущей папке.
# Compile the application:
# CGO_ENABLED=1 — force CGO support to link Go with the C-based librdkafka library.
# GOOS=linux — specify Linux as the target operating system.
# -tags musl — instruct the compiler to link against Alpine's musl library instead of glibc.
# -o autoscaler . — output the compiled binary as "autoscaler" in the current directory.
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o autoscaler .


# ==========================================
# ЭТАП ЗАПУСКА / RUNTIME STAGE
# ==========================================

# Начинаем чистый финальный этап на базе минимального образа Alpine Linux. Это исключает весь инструментарий разработки из боевого образа.
# Start a clean final stage based on a minimal Alpine Linux image. This excludes all build tools from the production image.
FROM alpine:latest

# Устанавливаем корневые сертификаты (для безопасных HTTPS запросов) и динамическую библиотеку librdkafka, которая необходима нашему бинарнику для работы с Kafka.
# Install root certificates (for secure HTTPS requests) and the dynamic librdkafka library, which our binary needs at runtime to interact with Kafka.
RUN apk --no-cache add ca-certificates librdkafka

# Копируем только скомпилированный бинарный файл из предыдущего этапа (builder), сохраняя его в корень финального образа.
# Copy only the compiled binary from the previous stage (builder), saving it into the root of the final image.
COPY --from=builder /app/autoscaler /autoscaler

# Определяем команду по умолчанию, которая запустится при старте контейнера.
# Define the default command to execute when the container starts.
CMD ["/autoscaler"]
