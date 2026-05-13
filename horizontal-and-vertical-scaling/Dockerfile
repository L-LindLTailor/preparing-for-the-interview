# ==========================================
# ЭТАП СБОРКИ / BUILD STAGE
# ==========================================

# Используем официальный образ Go на базе Alpine Linux версии 1.24.3 в качестве базового слоя для сборки. Стейдж называется "builder".
# Use the official Go image based on Alpine Linux 1.24.3 as the base layer for building. The stage is named "builder".
FROM golang:1.24.3-alpine AS builder

# 1. Устанавливаем системные утилиты компиляции (gcc, pkgconf) и заголовочные файлы (musl, librdkafka), необходимые для сборки Си-зависимостей Kafka.
# --no-cache отключает сохранение индексов пакетов, снижая объем дискового пространства.
# 1. Install system compilation tools (gcc, pkgconf) and header files (musl, librdkafka) required to build Kafka's C dependencies.
# --no-cache prevents storing package indexes, saving disk space.
RUN apk add --no-cache gcc musl-dev libc-dev librdkafka-dev pkgconf

# Указываем рабочую папку в контейнере, внутри которой будут выполняться все последующие инструкции.
# Specify the working directory inside the container where all subsequent instructions will run.
WORKDIR /app

# Копируем файлы описания зависимостей проекта для их изолированного кэширования в слоях Docker.
# Copy the project dependency definition files to enable isolated layering and caching in Docker.
COPY go.mod go.sum ./

# Загружаем внешние Go-модули, описанные в манифесте. Шаг закэшируется и выполнится повторно только при изменении go.mod/go.sum.
# Download external Go modules defined in the manifest. This step is cached and will rerun only if go.mod/go.sum changes.
RUN go mod download

# Копируем весь оставшийся исходный код проекта с локального диска хоста в рабочую директорию контейнера.
# Copy all remaining project source code from the host's local disk to the container's working directory.
COPY . .

# 2. Компилируем приложение:
# CGO_ENABLED=1 — принудительно включаем поддержку CGO для связывания Go с Си-библиотекой librdkafka.
# GOOS=linux — указываем целевую операционную систему Linux.
# -tags musl — явно указываем линковаться со стандартной Си-библиотекой Alpine (musl) вместо glibc.
# -o topic-horizontal-and-vertical-scaling . — собираем исполняемый бинарный файл с указанным именем в текущей папке.
# 2. Compile the application:
# CGO_ENABLED=1 — force CGO support to link Go with the C-based librdkafka library.
# GOOS=linux — specify Linux as the target operating system.
# -tags musl — explicitly instruct to link against Alpine's standard C library (musl) instead of glibc.
# -o topic-horizontal-and-vertical-scaling . — compile the current directory into an executable binary with the specified name.
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o topic-horizontal-and-vertical-scaling .


# ==========================================
# ЭТАП ЗАПУСКА / RUNTIME STAGE
# ==========================================

# Переключаемся на чистый, минимальный образ Alpine Linux. Это позволяет полностью исключить тяжелый Go-компилятор и Си-утилиты сборки из финального контейнера.
# Switch to a clean, minimal Alpine Linux image. This allows completely excluding the heavy Go compiler and C build tools from the final container.
FROM alpine:latest

# 3. В финальном образе ОБЯЗАТЕЛЬНО должна быть динамическая Си-библиотека librdkafka (необходима для работы бинарника), корневые SSL-сертификаты (ca-certificates) и утилита curl.
# 3. In the final image, the dynamic C library librdkafka (required for the binary to execute), root SSL certificates (ca-certificates), and curl utility are STRICTLY required.
RUN apk --no-cache add ca-certificates librdkafka curl

# Устанавливаем домашнюю директорию суперпользователя в качестве рабочей зоны этапа запуска.
# Set the root user's home directory as the working area for the runtime stage.
WORKDIR /root/

# Извлекаем только скомпилированный на первом этапе бинарный файл "topic-horizontal-and-vertical-scaling" и переносим его в текущую папку финального слоя.
# Extract only the compiled "topic-horizontal-and-vertical-scaling" binary from the first stage (builder) and transfer it to the current directory of the final layer.
COPY --from=builder /app/topic-horizontal-and-vertical-scaling .

# Информируем Docker, что контейнер слушает сетевой порт 8080 (инструкция носит документирующий характер).
# Inform Docker that the container listens on network port 8080 (this instruction is for documentation purposes only).
EXPOSE 8080

# Настраиваем команду по умолчанию, которая автоматически запустит наш веб-сервер при старте контейнера.
# Configure the default command that will automatically run our web server when the container starts.
CMD ["./topic-horizontal-and-vertical-scaling"]
