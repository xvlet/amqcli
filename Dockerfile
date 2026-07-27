# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy the entire project
COPY . .

# Build the binary statically
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-w -s" -o build/amqcli cmd/main.go

# Final stage
FROM alpine:latest

WORKDIR /app

# Install CA certificates for potential TLS connections
RUN apk --no-cache add ca-certificates

# Copy the binary and default config from the builder stage
COPY --from=builder /app/build/amqcli /usr/local/bin/amqcli
COPY --from=builder /app/config.yml /app/config.yml

# Default entrypoint
ENTRYPOINT ["amqcli"]
