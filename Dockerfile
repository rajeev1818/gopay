# Build stage — full Go toolchain
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gopay ./cmd/server

# Run stage — minimal image (13MB)
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/gopay /usr/local/bin/gopay
EXPOSE 8080
CMD ["gopay"]