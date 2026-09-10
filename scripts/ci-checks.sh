#!/bin/bash
# Validation only: this script must not generate, format, tidy in place, install
# tools, or restore Git state. Use `make fix` and `make generate` explicitly for
# deliberate preparation before running checks.
set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

printf '%b\n' "${YELLOW}Running comprehensive CI checks...${NC}"

FAILURES=0
FAILURE_MESSAGES=""
record_failure() {
	FAILURES=$((FAILURES + 1))
	FAILURE_MESSAGES="${FAILURE_MESSAGES}\n  ❌ $1"
}

printf '\n📌 Checking release pins...\n'
if ! ./scripts/verify-release-pin.sh; then
	record_failure 'Release pins are not clean (remove go.mod replace, go.work/go.work.sum, and local-toolkit/)'
else
	printf '%b\n' "${GREEN}✅ Release pins are clean${NC}"
fi

printf '\n📝 Checking EOF newlines...\n'
FILES_MISSING_EOF=""
while IFS= read -r file; do
	if [ -f "$file" ] && [ -s "$file" ] && [ "$(tail -c1 "$file" | wc -l)" -eq 0 ]; then
		FILES_MISSING_EOF="${FILES_MISSING_EOF}  - ${file}\n"
	fi
done < <(git ls-files '*.go' '*.md' '*.yml' '*.yaml' '*.json' 'Makefile' '.gitignore')
if [ -n "$FILES_MISSING_EOF" ]; then
	printf '%b\n' "${RED}Files missing EOF newline:${NC}"
	printf '%b' "$FILES_MISSING_EOF"
	record_failure 'Files missing EOF newline (run: make fix-eof)'
else
	printf '%b\n' "${GREEN}✅ All files have proper EOF newlines${NC}"
fi

printf '\n🔧 Checking generated-code workflow...\n'
printf '%b\n' "${GREEN}✅ Checks do not generate code; run 'make generate' explicitly before validation${NC}"

printf '\n📐 Checking code formatting...\n'
if ! command -v gofmt >/dev/null 2>&1; then
	record_failure 'gofmt is required (install the repository Go version)'
else
	UNFORMATTED_FILES=$(find . -name '*.go' -not -path './vendor/*' -not -path './gen/*' -not -path './mock/*' -not -path '*/mock/*' -exec gofmt -l {} \;)
	if [ -n "$UNFORMATTED_FILES" ]; then
		printf '%b\n%s\n' "${RED}Files need formatting:${NC}" "$UNFORMATTED_FILES"
		record_failure 'Files need formatting (run: make fmt)'
	else
		printf '%b\n' "${GREEN}✅ All files are properly formatted${NC}"
	fi
fi

printf '\n📦 Checking imports...\n'
if ! command -v goimports >/dev/null 2>&1; then
	printf '%b\n' "${RED}goimports is required; run 'make install-tools' explicitly${NC}"
	record_failure "goimports is missing (run 'make install-tools' explicitly)"
else
	IMPORT_ISSUES=$(find . -name '*.go' -not -path './vendor/*' -not -path './gen/*' -not -path './mock/*' -not -path '*/mock/*' -exec goimports -l -local github.com/KirkDiggler {} \;)
	if [ -n "$IMPORT_ISSUES" ]; then
		printf '%b\n%s\n' "${RED}Files have import issues:${NC}" "$IMPORT_ISSUES"
		record_failure 'Import issues (run: make fmt)'
	else
		printf '%b\n' "${GREEN}✅ All imports are properly organized${NC}"
	fi
fi

printf '\n📋 Checking go.mod/go.sum tidiness...\n'
TIDY_OUTPUT=$(go mod tidy -diff 2>&1)
TIDY_STATUS=$?
if [ "$TIDY_STATUS" -ne 0 ]; then
	printf '%s\n' "$TIDY_OUTPUT"
	record_failure "go.mod/go.sum need tidying or tidy failed (run 'make tidy' explicitly)"
elif [ -n "$TIDY_OUTPUT" ]; then
	printf '%s\n' "$TIDY_OUTPUT"
	record_failure "go.mod/go.sum need tidying (run 'make tidy' explicitly)"
else
	printf '%b\n' "${GREEN}✅ go.mod/go.sum are tidy${NC}"
fi

printf '\n🔍 Running linter...\n'
if ! command -v golangci-lint >/dev/null 2>&1; then
	printf '%b\n' "${RED}golangci-lint is required; run 'make install-tools' explicitly${NC}"
	record_failure "golangci-lint is missing (run 'make install-tools' explicitly)"
elif ! golangci-lint run; then
	record_failure 'Linter found issues'
else
	printf '%b\n' "${GREEN}✅ Linter passed${NC}"
fi

printf '\n🧪 Running tests (CI mode)...\n'
PACKAGE_OUTPUT=$(go list ./... 2>&1)
PACKAGE_STATUS=$?
if [ "$PACKAGE_STATUS" -ne 0 ]; then
	printf '%s\n' "$PACKAGE_OUTPUT"
	record_failure 'Could not list test packages'
else
	PACKAGES=$(printf '%s\n' "$PACKAGE_OUTPUT" | grep -v /gen/ | grep -v /mock | grep -v cmd/server || true)
	COVERAGE_FILE=$(mktemp "${TMPDIR:-/tmp}/rpg-api-ci-coverage.XXXXXX")
	if [ -n "$PACKAGES" ] && ! go test -v -race -coverprofile="$COVERAGE_FILE" -covermode=atomic $PACKAGES; then
		record_failure 'Tests failed'
	else
		printf '%b\n' "${GREEN}✅ All tests passed${NC}"
	fi
	rm -f "$COVERAGE_FILE"
fi

printf '\n📊 Summary:\n'
if [ "$FAILURES" -eq 0 ]; then
	printf '%b\n' "${GREEN}✅ All CI checks passed!${NC}"
	printf '%b\n' "${GREEN}Your code should pass CI.${NC}"
	exit 0
fi
printf '%b\n' "${RED}❌ ${FAILURES} check(s) failed:${NC}"
printf '%b\n' "$FAILURE_MESSAGES"
printf '\n%b\n' "${YELLOW}Fix these issues before pushing to avoid CI failures.${NC}"
exit 1
