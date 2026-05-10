FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o autoscaler .

FROM alpine:latest
# ca-certificates нужны для работы с API
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/autoscaler /autoscaler
CMD ["/autoscaler"]
