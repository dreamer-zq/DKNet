# Updated with latest signing functionality - 2025/06/10
# Multi-stage build for DKNet
FROM golang:1.24-alpine AS builder

# Install dependencies for building
RUN apk add --no-cache git gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Verify source files are present
RUN ls -la ./cmd/tss-server/ && echo "Source files copied successfully"

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o tss-server ./cmd/tss-server

# Verify binary was created
RUN ls -la tss-server && echo "Binary built successfully"

# Final stage
FROM alpine:latest

# Install required packages
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 tss && \
    adduser -D -u 1001 -G tss tss

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/tss-server .

# Create necessary directories
RUN mkdir -p /app/data && \
    chown -R tss:tss /app

# Switch to non-root user
USER tss

# Expose ports
EXPOSE 8080 9090 4001

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the binary
ENTRYPOINT ["./tss-server"]
CMD ["start"] 