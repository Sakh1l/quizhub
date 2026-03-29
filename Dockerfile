# ---- Build stage ----
FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /quizhub ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /quizhub /app/quizhub

ENV QUIZHUB_PORT=8080
ENV QUIZHUB_DB=/app/data/quizhub.db
ENV QUIZHUB_ADMIN_PIN=1234

VOLUME ["/app/data"]
EXPOSE 8080

ENTRYPOINT ["/app/quizhub"]
