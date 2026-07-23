# Единый этап сборки для всех бинарников
FROM golang:1.26-alpine AS builder

WORKDIR /app
ENV GOINSECURE=*

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
COPY web/ ./web/

RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o worker ./cmd/worker

# Финальный образ для server
FROM alpine:latest AS api

RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -u 1000 -g 1000 appuser

WORKDIR /app
COPY --from=builder --chown=appuser:appuser /app/server .  

USER appuser
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --spider -q http://localhost:8080/health || exit 1
CMD ["/app/server"]

# Финальный образ для воркера
FROM alpine:latest AS worker

RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -u 1001 -g 1001 workeruser

WORKDIR /app
COPY --from=builder --chown=workeruser:workeruser /app/worker .

USER workeruser
CMD ["/app/worker"]

