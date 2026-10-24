# syntax=docker/dockerfile:1

# Stage 1: Build binary statically
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compile static binary with zero CGO
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o /app/bin/manhua-api \
    ./cmd/server

# Stage 2: Distroless production container
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

# Copy timezone data and CA certs
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy compiled binary
COPY --from=builder /app/bin/manhua-api /manhua-api

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/manhua-api"]
