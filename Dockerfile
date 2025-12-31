# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies (including gcc for CGO/SQLite)
RUN apk add --no-cache git ca-certificates gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with CGO enabled for SQLite support
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o /hub ./cmd/hub

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 hub && \
    adduser -u 1000 -G hub -s /bin/sh -D hub

# Copy binary from builder
COPY --from=builder /hub /app/hub

# Set ownership
RUN chown -R hub:hub /app

USER hub

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

# Run the binary
ENTRYPOINT ["/app/hub"]
CMD ["serve"]
