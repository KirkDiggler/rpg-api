.PHONY: help
help: ## Display this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

.PHONY: pre-commit
pre-commit: fmt tidy lint test ## Run all pre-commit checks

.PHONY: fmt
fmt: ## Format Go code
	@echo "==> Formatting code..."
	@go fmt ./...

.PHONY: tidy
tidy: ## Tidy go.mod
	@echo "==> Tidying go.mod..."
	@go mod tidy

.PHONY: lint
lint: ## Run linter
	@echo "==> Running linter..."
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "golangci-lint not found. Installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	@golangci-lint run

.PHONY: test
test: ## Run tests
	@echo "==> Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...

.PHONY: test-coverage
test-coverage: test ## Run tests and display coverage
	@echo "==> Generating coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: proto
proto: ## Generate code from proto files
	@echo "==> Generating proto code..."
	@if ! command -v protoc &> /dev/null; then \
		echo "protoc not found. Please install protocol buffers compiler"; \
		exit 1; \
	fi
	@protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/v1alpha1/*.proto

.PHONY: run
run: ## Run the server
	@echo "==> Running server..."
	@go run cmd/server/main.go

.PHONY: build
build: ## Build the server binary
	@echo "==> Building server..."
	@go build -o bin/rpg-api cmd/server/main.go

.PHONY: clean
clean: ## Clean build artifacts
	@echo "==> Cleaning..."
	@rm -rf bin/ coverage.out coverage.html

.PHONY: deps
deps: ## Install development dependencies
	@echo "==> Installing dependencies..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/golang/mock/mockgen@latest

.PHONY: mocks
mocks: ## Generate mocks
	@echo "==> Generating mocks..."
	@go generate ./...

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "==> Building Docker image..."
	@docker build -t rpg-api:latest .

.PHONY: docker-run
docker-run: ## Run Docker container
	@echo "==> Running Docker container..."
	@docker run -p 50051:50051 rpg-api:latest