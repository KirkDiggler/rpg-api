.PHONY: help
help: ## Display this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

.PHONY: pre-commit
pre-commit: fmt tidy fix-eof buf-lint lint test ## Run all pre-commit checks

.PHONY: fmt
fmt: ## Format Go code with gofmt and goimports
	@echo "==> Formatting code..."
	@if ! command -v goimports &> /dev/null; then \
		echo "goimports not found. Installing..."; \
		go install golang.org/x/tools/cmd/goimports@latest; \
	fi
	@echo "→ Running gofmt with simplify..."
	@find . -name "*.go" -not -path "./vendor/*" -not -path "./gen/*" -not -path "./mock/*" -exec gofmt -s -w {} \;
	@echo "→ Running goimports..."
	@find . -name "*.go" -not -path "./vendor/*" -not -path "./gen/*" -not -path "./mock/*" -exec goimports -w -local github.com/KirkDiggler {} \;
	@echo "✅ Formatting complete"

.PHONY: tidy
tidy: ## Tidy go.mod
	@echo "==> Tidying go.mod..."
	@go mod tidy

.PHONY: lint
lint: ## Run linter
	@echo "==> Running linter..."
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "golangci-lint not found. Installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0; \
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

# Check if proto files have changed
PROTO_FILES := $(shell find api/proto -name '*.proto' 2>/dev/null)
GEN_GO_FILES := $(shell find gen/go -name '*.pb.go' 2>/dev/null)

# Only regenerate if protos are newer than generated files
.PHONY: buf-generate
buf-generate: ## Generate code from proto files using buf
	@if [ -z "$(GEN_GO_FILES)" ] || [ -n "$$(find api/proto -name '*.proto' -newer gen/go -print -quit 2>/dev/null)" ]; then \
		echo "==> Generating proto code with buf..."; \
		if ! command -v buf &> /dev/null; then \
			echo "buf not found. Installing..."; \
			go install github.com/bufbuild/buf/cmd/buf@latest; \
		fi; \
		buf generate; \
	else \
		echo "==> Proto files unchanged, skipping generation"; \
	fi

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
	@go run cmd/server/*.go server

.PHONY: dev
dev: ## Run the server in development mode with hot reload
	@echo "==> Running server in dev mode..."
	@go run cmd/server/*.go server --port 50051

.PHONY: build
build: ## Build the server binary
	@echo "==> Building server..."
	@go build -o bin/rpg-api cmd/server/*.go

.PHONY: clean
clean: ## Clean build artifacts
	@echo "==> Cleaning..."
	@rm -rf bin/ coverage.out coverage.html

.PHONY: fix-eof
fix-eof: ## Add missing EOF newlines
	@echo "==> Fixing EOF newlines..."
	@for file in $$(git ls-files '*.go' '*.proto' '*.md' '*.yml' '*.yaml' '*.json' 'Makefile' '.gitignore'); do \
		if [ -f "$$file" ] && [ -s "$$file" ] && [ $$(tail -c1 "$$file" | wc -l) -eq 0 ]; then \
			echo "Fixing: $$file"; \
			echo >> "$$file"; \
		fi \
	done

.PHONY: deps
deps: install-tools ## Install development dependencies (alias for install-tools)

.PHONY: install-tools
install-tools: ## Install all development tools
	@echo "==> Installing development tools..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0
	@go install go.uber.org/mock/mockgen@latest
	@go install github.com/bufbuild/buf/cmd/buf@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@echo "✅ Tools installed successfully"

.PHONY: install-hooks
install-hooks: ## Install git hooks for pre-commit checks
	@echo "==> Installing git hooks..."
	@mkdir -p .githooks
	@if [ ! -f .githooks/pre-commit ]; then \
		echo "Creating pre-commit hook..."; \
		cp scripts/pre-commit.sh .githooks/pre-commit 2>/dev/null || \
		echo "#!/bin/bash" > .githooks/pre-commit && \
		echo "set -e" >> .githooks/pre-commit && \
		echo "make pre-commit" >> .githooks/pre-commit; \
		chmod +x .githooks/pre-commit; \
	fi
	@git config core.hooksPath .githooks
	@echo "✅ Git hooks installed"

.PHONY: fix
fix: fmt tidy fix-eof ## Fix all auto-fixable issues
	@echo "✅ All auto-fixable issues resolved"
	@echo "Run 'git add -u' to stage the changes"

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
