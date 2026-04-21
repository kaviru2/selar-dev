# SELAR API Dockerfile — Multi-stage Go build
# Produces a minimal Alpine-based image for the Go API server.

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /selar-api ./cmd/server

# --- Runtime ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /selar-api /usr/local/bin/selar-api

EXPOSE 8080

ENTRYPOINT ["selar-api"]
