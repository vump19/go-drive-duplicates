# Go Drive Duplicates - Clean Architecture Makefile

.PHONY: build run test clean help dev prod migrate

# Default target
all: build

# Build the server
build:
	@echo "🔨 Building Go Drive Duplicates server..."
	go build -o server ./cmd/server
	@echo "✅ Build completed: ./server"

# Build migration tool
build-migrate:
	@echo "🔨 Building migration tool..."
	go build -o migrate ./cmd/migrate
	@echo "✅ Migration tool built: ./migrate"

# Run the server in development mode
run: build
	@echo "🚀 Starting Go Drive Duplicates server..."
	./server -config config/app.json

# Run with YAML config
run-yaml: build
	@echo "🚀 Starting Go Drive Duplicates server with YAML config..."
	./server -config config/app.yaml

# Run in development environment
dev: build
	@echo "🚀 Starting in development mode..."
	./server -config config/environments/development.yaml

# Run in production environment  
prod: build
	@echo "🚀 Starting in production mode..."
	./server -config config/environments/production.yaml

# Run database migration
migrate: build-migrate
	@echo "📦 Running database migration..."
	./migrate

# Run tests
test:
	@echo "🧪 Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "🧪 Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "📊 Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -f server migrate
	rm -f coverage.out coverage.html
	@echo "✅ Clean completed"

# Install dependencies
deps:
	@echo "📦 Installing dependencies..."
	go mod download
	go mod tidy
	@echo "✅ Dependencies installed"

# Update dependencies
update-deps:
	@echo "🔄 Updating dependencies..."
	go get -u ./...
	go mod tidy
	@echo "✅ Dependencies updated"

# Lint code
lint:
	@echo "🔍 Running linter..."
	golangci-lint run
	@echo "✅ Linting completed"

# Format code
fmt:
	@echo "✨ Formatting code..."
	go fmt ./...
	@echo "✅ Code formatted"

# Health check
health:
	@echo "🔍 Checking server health..."
	curl -f http://localhost:8080/health || echo "❌ Server is not running"

# Show help
help:
	@echo "🗂️  Go Drive Duplicates - Clean Architecture"
	@echo ""
	@echo "Available commands:"
	@echo "  build         Build the server binary"
	@echo "  build-migrate Build the migration tool"
	@echo "  run           Build and run server with JSON config"
	@echo "  run-yaml      Build and run server with YAML config"
	@echo "  dev           Run in development mode"
	@echo "  prod          Run in production mode"
	@echo "  migrate       Run database migration"
	@echo "  test          Run all tests"
	@echo "  test-coverage Run tests with coverage report"
	@echo "  clean         Clean build artifacts"
	@echo "  deps          Install dependencies"
	@echo "  update-deps   Update all dependencies"
	@echo "  lint          Run code linter"
	@echo "  fmt           Format all Go code"
	@echo "  health        Check server health"
	@echo "  help          Show this help message"
	@echo ""
	@echo "📚 Configuration files:"
	@echo "  config/app.json                    - Default JSON config"
	@echo "  config/app.yaml                    - Default YAML config" 
	@echo "  config/environments/development.yaml - Development config"
	@echo "  config/environments/production.yaml  - Production config"
	@echo "  config/environments/testing.yaml     - Testing config"
	@echo ""
	@echo "🚀 Quick start:"
	@echo "  make run      # Start server with default config"
	@echo "  make health   # Check if server is running"