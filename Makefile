.PHONY: help
help: ## Display this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

.PHONY: pre-commit
pre-commit: fmt tidy fix-eof lint test ## Run all pre-commit checks

.PHONY: ci-check
ci-check: ## Run comprehensive CI checks (detects common failures)
	@./scripts/ci-checks.sh

.PHONY: ci-fix
ci-fix: install-tools generate fmt tidy fix-eof ## Fix common CI issues automatically

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
test: test-unit ## Run all tests (alias for test-unit by default)

.PHONY: test-unit
test-unit: ## Run unit tests only
	@echo "==> Running unit tests..."
	@go test -v -short -race -coverprofile=coverage.out ./...

.PHONY: test-ci
test-ci: ## Run tests exactly as CI does
	@echo "==> Running tests with CI configuration..."
	@go test -v -race -coverprofile=coverage.out -covermode=atomic \
		$$(go list ./... | grep -v /gen/ | grep -v /mock | grep -v cmd/server)
	@echo "✅ CI tests passed"

.PHONY: test-integration
test-integration: ## Run integration tests (requires Redis)
	@echo "==> Running integration tests..."
	@echo "→ Ensuring Redis is available..."
	@docker run -d --name test-redis -p 6379:6379 redis:alpine || true
	@sleep 2
	@go test -v -race -tags=integration ./...
	@docker stop test-redis && docker rm test-redis || true

.PHONY: test-all
test-all: test-unit test-integration ## Run all tests including integration

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
