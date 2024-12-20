# Build stage
FROM golang:1.23.3-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o mqtt-benchmark

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/mqtt-benchmark .

# Expose prometheus metrics port
EXPOSE 2112

# Set entrypoint
ENTRYPOINT ["/app/mqtt-benchmark"]
