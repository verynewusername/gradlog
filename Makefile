.PHONY: all build run test clean docker-build docker-up docker-down deps

# Default target
all: build

# Install dependencies
deps:
	cd gradlog && go mod download && go mod tidy

# Build the backend binary
build:
	cd gradlog && go build -o bin/gradlog ./cmd/gradlog

# Run the backend locally (requires PostgreSQL)
run:
	cd gradlog && go run ./cmd/gradlog

# Run tests
test:
	cd gradlog && go test -v ./...

# Clean build artifacts
clean:
	rm -rf gradlog/bin

# Docker operations
docker-build:
	docker-compose build

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

# Development setup
dev-setup:
	cp gradlog/.env.example gradlog/.env
	@echo "Edit gradlog/.env with your configuration"
	@echo "Then run: make docker-up"

# Database operations (requires running PostgreSQL)
db-migrate:
	cd gradlog && go run ./cmd/gradlog migrate

# SDK operations
sdk-install:
	cd sdk/python && pip install -e ".[dev]"

sdk-test:
	cd sdk/python && pytest

sdk-build:
	cd sdk/python && python -m build

# Lint
lint:
	cd gradlog && go vet ./...
	cd sdk/python && ruff check .

# Format
fmt:
	cd gradlog && go fmt ./...
	cd sdk/python && black .
