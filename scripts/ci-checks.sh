#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Running comprehensive CI checks...${NC}"

# Track failures
FAILURES=0
FAILURE_MESSAGES=""

# Function to record failures
record_failure() {
    FAILURES=$((FAILURES + 1))
    FAILURE_MESSAGES="${FAILURE_MESSAGES}\n  ❌ $1"
}

# 1. Check that CI/pre-PR resolves only committed, published module pins.
echo -e "\n📌 Checking release pins..."
if ! ./scripts/verify-release-pin.sh; then
    record_failure "Release pins are not clean (remove go.mod replace, go.work/go.work.sum, and local-toolkit/)"
else
    echo -e "${GREEN}✅ Release pins are clean${NC}"
fi

# 2. Check for missing EOF newlines
echo -e "\n📝 Checking EOF newlines..."
FILES_MISSING_EOF=""
for file in $(git ls-files '*.go' '*.md' '*.yml' '*.yaml' '*.json' 'Makefile' '.gitignore'); do
    if [ -f "$file" ] && [ -s "$file" ] && [ $(tail -c1 "$file" | wc -l) -eq 0 ]; then
        FILES_MISSING_EOF="${FILES_MISSING_EOF}  - $file\n"
    fi
done
if [ -n "$FILES_MISSING_EOF" ]; then
    echo -e "${RED}Files missing EOF newline:${NC}"
    echo -e "$FILES_MISSING_EOF"
    record_failure "Files missing EOF newline"
else
    echo -e "${GREEN}✅ All files have proper EOF newlines${NC}"
fi

# 3. Check if mocks need regeneration
echo -e "\n🔧 Checking mock generation..."
go generate ./... 2>/dev/null
if ! git diff --quiet; then
    echo -e "${RED}Mocks need to be regenerated!${NC}"
    git diff --name-only | grep -E '(mock|Mock)' || true
    record_failure "Mocks need regeneration (run: go generate ./...)"
    git checkout -- .
else
    echo -e "${GREEN}✅ Mocks are up to date${NC}"
fi

# 4. Check formatting
echo -e "\n📐 Checking code formatting..."
UNFORMATTED_FILES=$(find . -name "*.go" -not -path "./vendor/*" -not -path "./gen/*" -not -path "./mock/*" -exec gofmt -l {} \;)
if [ -n "$UNFORMATTED_FILES" ]; then
    echo -e "${RED}Files need formatting:${NC}"
    echo "$UNFORMATTED_FILES"
    record_failure "Files need formatting (run: make fmt)"
else
    echo -e "${GREEN}✅ All files are properly formatted${NC}"
fi

# 5. Check imports
echo -e "\n📦 Checking imports..."
if command -v goimports &> /dev/null; then
    # MockGen owns nested mock/ imports and emits a grouping that goimports
    # intentionally rewrites for this repository's local prefix. Exclude every
    # generated mock directory so the two generators do not contradict each other.
    IMPORT_ISSUES=$(find . -name "*.go" -not -path "./vendor/*" -not -path "./gen/*" -not -path "./mock/*" -not -path "*/mock/*" -exec goimports -l -local github.com/KirkDiggler {} \;)
    if [ -n "$IMPORT_ISSUES" ]; then
        echo -e "${RED}Files have import issues:${NC}"
        echo "$IMPORT_ISSUES"
        record_failure "Import issues (run: make fmt)"
    else
        echo -e "${GREEN}✅ All imports are properly organized${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  goimports not found, skipping import check${NC}"
fi

# 6. Check go.mod tidy
echo -e "\n📋 Checking go.mod..."
cp go.mod go.mod.backup
cp go.sum go.sum.backup 2>/dev/null || touch go.sum.backup
go mod tidy
if ! diff -q go.mod go.mod.backup >/dev/null || ! diff -q go.sum go.sum.backup >/dev/null; then
    echo -e "${RED}go.mod/go.sum needs tidying${NC}"
    record_failure "go.mod needs tidying (run: go mod tidy)"
    mv go.mod.backup go.mod
    mv go.sum.backup go.sum
else
    echo -e "${GREEN}✅ go.mod is tidy${NC}"
    rm go.mod.backup go.sum.backup
fi

# 7. Run linter with CI configuration
echo -e "\n🔍 Running linter..."
if command -v golangci-lint &> /dev/null; then
    if ! golangci-lint run; then
        record_failure "Linter found issues (run: golangci-lint run)"
    else
        echo -e "${GREEN}✅ Linter passed${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  golangci-lint not found, skipping lint check${NC}"
    echo "  Install with: make install-tools"
fi

# 8. Run tests with CI configuration
echo -e "\n🧪 Running tests (CI mode)..."
if ! go test -v -race -coverprofile=coverage.out -covermode=atomic \
    $(go list ./... | grep -v /gen/ | grep -v /mock | grep -v cmd/server); then
    record_failure "Tests failed"
else
    echo -e "${GREEN}✅ All tests passed${NC}"
fi

# 9. Additional checks can be added here as we discover new patterns

# Summary
echo -e "\n📊 Summary:"
if [ $FAILURES -eq 0 ]; then
    echo -e "${GREEN}✅ All CI checks passed!${NC}"
    echo -e "${GREEN}Your code should pass CI.${NC}"
    exit 0
else
    echo -e "${RED}❌ $FAILURES check(s) failed:${NC}"
    echo -e "$FAILURE_MESSAGES"
    echo -e "\n${YELLOW}Fix these issues before pushing to avoid CI failures.${NC}"
    exit 1
fi