.PHONY: help
help: ## Display this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

.PHONY: pre-commit
pre-commit: fmt tidy fix-eof lint test ## Run all pre-commit checks

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
lint: install-tools ## Run linter
	@echo "==> Running linter..."
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
	@for file in $$(git ls-files '*.go' '*.md' '*.yml' '*.yaml' '*.json' 'Makefile' '.gitignore'); do \
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
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.2.2
	@go install go.uber.org/mock/mockgen@latest
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
generate: mocks ## Generate all code (mocks only)

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
