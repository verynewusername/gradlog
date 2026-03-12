# Build stage
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

# Populated automatically by Buildx
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Install build dependencies.
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for better caching.
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code.
COPY . .

# Build the binary with embedded migrations.
# CGO is disabled for a fully static binary.
# GOOS/GOARCH are set from Buildx TARGETOS/TARGETARCH for native cross-compilation.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-w -s" -o /gradlog ./cmd/gradlog

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
