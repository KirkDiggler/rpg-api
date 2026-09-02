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
#
# This step writes into the working tree, so it refuses rather than repairs
# (rpg-api#880). On a dirty tree a post-generation diff is not attributable to
# the generator, and undoing it would mean restoring paths the generator never
# wrote -- which is how this check silently discarded a branch's uncommitted
# work. It now runs only on a clean tree, and only ever restores the generated
# mock paths listed below, after printing them.
echo -e "\n🔧 Checking mock generation..."
# Every mockgen destination in this repository lives in a mock/ or mocks/
# directory next to the interface it mocks (see CLAUDE.md, "Mock
# Organization"). This is the entire set of paths this step can write, and so
# the entire set it is allowed to restore.
MOCK_PATHS=(':(glob)**/mock/*.go' ':(glob)**/mocks/*.go')
NON_MOCK_PATHS=(':/' ':(glob,exclude)**/mock/*.go' ':(glob,exclude)**/mocks/*.go')
if [ -n "$(git status --porcelain)" ]; then
    echo -e "${YELLOW}⚠️  Skipped: working tree is dirty${NC}"
    echo "  The mock freshness check only means something on a clean tree."
    echo "  Commit or stash your changes and re-run to check mocks."
elif ! GENERATE_OUTPUT=$(go generate ./... 2>&1); then
    # Reported rather than swallowed: under `set -e` a failing generator used
    # to abort the whole run with its stderr sent to /dev/null.
    echo -e "${RED}go generate failed:${NC}"
    echo "$GENERATE_OUTPUT"
    record_failure "go generate failed (install generators with: make install-tools)"
else
    STALE_MOCKS=$(git diff --name-only -- "${MOCK_PATHS[@]}")
    NEW_MOCKS=$(git ls-files --others --exclude-standard -- "${MOCK_PATHS[@]}")
    STRAY_CHANGES=$(git status --porcelain -- "${NON_MOCK_PATHS[@]}")

    if [ -n "$STALE_MOCKS" ] || [ -n "$NEW_MOCKS" ]; then
        echo -e "${RED}Mocks need to be regenerated!${NC}"
        echo -e "${YELLOW}Restoring these generated files to leave the tree as it was found:${NC}"
        if [ -n "$STALE_MOCKS" ]; then
            mapfile -t STALE_LIST <<< "$STALE_MOCKS"
            for f in "${STALE_LIST[@]}"; do echo "  restore $f"; done
            git checkout -- "${STALE_LIST[@]}"
        fi
        if [ -n "$NEW_MOCKS" ]; then
            mapfile -t NEW_LIST <<< "$NEW_MOCKS"
            for f in "${NEW_LIST[@]}"; do echo "  remove  $f (generated just now; did not exist before)"; done
            rm -f -- "${NEW_LIST[@]}"
        fi
        record_failure "Mocks need regeneration (run: go generate ./...)"
    elif [ -z "$STRAY_CHANGES" ]; then
        echo -e "${GREEN}✅ Mocks are up to date${NC}"
    fi

    if [ -n "$STRAY_CHANGES" ]; then
        echo -e "${YELLOW}⚠️  go generate also wrote outside the mock directories:${NC}"
        echo "$STRAY_CHANGES"
        echo "  Left untouched -- this step restores generated mocks only."
        record_failure "go generate wrote outside the mock directories (review the files above)"
    fi
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