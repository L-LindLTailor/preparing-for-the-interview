FROM golang:1.23.1-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o topic-horizontal-and-vertical-scaling .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/topic-horizontal-and-vertical-scaling .
EXPOSE 8080
CMD ["./topic-horizontal-and-vertical-scaling"]
