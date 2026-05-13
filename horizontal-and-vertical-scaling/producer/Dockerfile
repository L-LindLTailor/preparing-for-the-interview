# ==========================================
# ЭТАП СБОРКИ / BUILD STAGE
# ==========================================

# Используем официальный образ Go на базе Alpine Linux версии 1.24.3 в качестве базового слоя для сборки. Стейдж называется "builder".
# Use the official Go image based on Alpine Linux 1.24.3 as the base layer for building. The stage is named "builder".
FROM golang:1.24.3-alpine AS builder

# Устанавливаем системные утилиты компиляции (gcc, pkgconf) и заголовочные файлы (musl, librdkafka), необходимые для сборки Си-зависимостей Kafka.
# --no-cache отключает сохранение индексов пакетов, снижая объем дискового пространства.
# Install system compilation tools (gcc, pkgconf) and header files (musl, librdkafka) required to build Kafka's C dependencies.
# --no-cache prevents storing package indexes, saving disk space.
RUN apk add --no-cache gcc musl-dev libc-dev librdkafka-dev pkgconf

# Указываем рабочую папку в контейнере, внутри которой будут выполняться все последующие инструкции.
# Specify the working directory inside the container where all subsequent instructions will run.
WORKDIR /app

# Копируем только файлы манифеста зависимостей проекта для оптимизации кэширования слоев Docker.
# Copy only the project dependency manifest files to optimize Docker layer caching.
COPY go.mod go.sum ./

# Загружаем внешние Go-модули, описанные в манифесте. Шаг закэшируется и выполнится повторно только при изменении go.mod/go.sum.
# Download external Go modules defined in the manifest. This step is cached and will rerun only if go.mod/go.sum changes.
RUN go mod download

# Копируем весь оставшийся исходный код проекта с локального диска хоста в рабочую директорию контейнера.
# Copy all remaining project source code from the host's local disk to the container's working directory.
COPY . .

# Компилируем приложение:
# -tags musl — явно указываем линковаться со стандартной Си-библиотекой Alpine (musl) вместо glibc.
# -o producer . — собираем текущую папку в исполняемый бинарный файл с именем "producer".
# Compile the application:
# -tags musl — explicitly instruct to link against Alpine's standard C library (musl) instead of glibc.
# -o producer . — compile the current directory into an executable binary named "producer".
RUN go build -tags musl -o producer .


# ==========================================
# ЭТАП ЗАПУСКА / RUNTIME STAGE
# ==========================================

# Переключаемся на чистый, минимальный образ Alpine Linux. Это позволяет полностью исключить тяжелый Go-компилятор и Си-утилиты сборки из финального контейнера.
# Switch to a clean, minimal Alpine Linux image. This allows completely excluding the heavy Go compiler and C build tools from the final container.
FROM alpine:latest

# Устанавливаем динамическую Си-библиотеку librdkafka (необходима для работы бинарника) и корневые SSL-сертификаты (ca-certificates).
# Install the dynamic C library librdkafka (required for the binary to execute) and root SSL certificates (ca-certificates).
RUN apk add --no-cache librdkafka ca-certificates

# Устанавливаем домашнюю директорию суперпользователя в качестве рабочей зоны этапа запуска.
# Set the root user's home directory as the working area for the runtime stage.
WORKDIR /root/

# Извлекаем только скомпилированный на первом этапе бинарный файл "producer" и переносим его в текущую папку финального слоя.
# Extract only the compiled "producer" binary from the first stage (builder) and transfer it to the current directory of the final layer.
COPY --from=builder /app/producer .

# Настраиваем команду по умолчанию, которая автоматически запустит наш продюсер при старте контейнера.
# Configure the default command that will automatically run our producer when the container starts.
CMD ["./producer"]
