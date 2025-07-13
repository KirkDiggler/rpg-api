.PHONY: help
help: ## Display this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

.PHONY: pre-commit
pre-commit: fmt tidy buf-lint lint test ## Run all pre-commit checks

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
proto: buf-generate ## Generate code from proto files (alias for buf-generate)

.PHONY: buf-lint
buf-lint: ## Lint proto files with buf
	@echo "==> Linting proto files..."
	@if ! command -v buf &> /dev/null; then \
		echo "buf not found. Installing..."; \
		go install github.com/bufbuild/buf/cmd/buf@latest; \
	fi
	@buf lint

.PHONY: buf-generate
buf-generate: ## Generate code from proto files using buf
	@echo "==> Generating proto code with buf..."
	@if ! command -v buf &> /dev/null; then \
		echo "buf not found. Installing..."; \
		go install github.com/bufbuild/buf/cmd/buf@latest; \
	fi
	@buf generate

.PHONY: buf-breaking
buf-breaking: ## Check for breaking changes in proto files
	@echo "==> Checking for breaking changes..."
	@if ! command -v buf &> /dev/null; then \
		echo "buf not found. Installing..."; \
		go install github.com/bufbuild/buf/cmd/buf@latest; \
	fi
	@buf breaking --against '.git#branch=main'

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
	@go install github.com/bufbuild/buf/cmd/buf@latest

.PHONY: generate
generate: buf-generate mocks ## Generate all code (protos and mocks)

.PHONY: mocks
mocks: ## Generate mocks
	@echo "==> Generating mocks..."
	@go generate ./...

.PHONY: proto-mocks
proto-mocks: buf-generate ## Generate mocks for proto clients (for Discord bot)
	@echo "==> Generating proto client mocks..."
	@if ! command -v mockgen &> /dev/null; then \
		echo "mockgen not found. Installing..."; \
		go install github.com/golang/mock/mockgen@latest; \
	fi
	@mkdir -p mocks/proto
	@mockgen -source=gen/go/github.com/KirkDiggler/rpg-api/api/proto/v1alpha1/dnd5e/character_grpc.pb.go -destination=mocks/proto/character_api_mock.go -package=protomocks

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "==> Building Docker image..."
	@docker build -t rpg-api:latest .

.PHONY: docker-run
docker-run: ## Run Docker container
	@echo "==> Running Docker container..."
	@docker run -p 50051:50051 rpg-api:latest