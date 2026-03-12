# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app/backend

# Install build dependencies.
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for better caching.
COPY backend/go.mod backend/go.sum* ./
RUN go mod download

# Copy backend source code.
COPY backend/ ./

# Copy frontend static files into the embedded UI directory.
# These files are served by the Go binary via go:embed.
COPY frontend/ ./internal/ui/dist/

# Build the binary with embedded migrations.
# CGO is disabled for a fully static binary.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /gradlog ./cmd/gradlog

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install CA certificates for HTTPS requests (OAuth).
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user for security.
RUN adduser -D -g '' gradlog

# Create artifact storage directory.
RUN mkdir -p /data/artifacts && chown -R gradlog:gradlog /data

# Copy the binary from builder.
COPY --from=builder /gradlog /app/gradlog

# Use non-root user.
USER gradlog

# Expose the default port.
EXPOSE 8080

# Runtime configuration is done via environment variables.
# The .env file should be mounted or env vars set directly.

ENTRYPOINT ["/app/gradlog"]
