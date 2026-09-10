#!/bin/bash
# Hermetic safety tests for ci-checks.sh. Every checker invocation runs in a
# disposable Git repository with stubbed tools; this test never points the
# checker at the caller's working tree.
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECKER_SOURCE="$SOURCE_DIR/scripts/ci-checks.sh"
MAKEFILE_SOURCE="$SOURCE_DIR/Makefile"
REAL_GIT="$(command -v git)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

make_bin() {
	local bin="$1"
	mkdir -p "$bin"
	cat >"$bin/go" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'go %s\n' "$*" >>"$CHECK_COMMAND_LOG"
case "${1:-}" in
generate)
	printf 'generated\n' >>generated/mock.go
	;;
mod)
	if [ "${2:-}" = tidy ] && [ "${3:-}" = -diff ]; then
		printf '%s' "${TIDY_DIFF:-}"
	fi
	;;
list)
	printf 'example.com/fixture\n'
	;;
test)
	exit "${GO_TEST_EXIT:-0}"
	;;
install)
	echo 'go install must not be invoked by a check' >&2
	exit 97
	;;
esac
EOF
	chmod +x "$bin/go"

	cat >"$bin/gofmt" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'gofmt %s\n' "$*" >>"$CHECK_COMMAND_LOG"
EOF
	chmod +x "$bin/gofmt"

	cat >"$bin/goimports" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'goimports %s\n' "$*" >>"$CHECK_COMMAND_LOG"
EOF
	chmod +x "$bin/goimports"

	cat >"$bin/golangci-lint" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'golangci-lint %s\n' "$*" >>"$CHECK_COMMAND_LOG"
exit "${LINT_EXIT:-0}"
EOF
	chmod +x "$bin/golangci-lint"

	cat >"$bin/git" <<EOF
#!/bin/bash
set -euo pipefail
printf 'git %s\\n' "\$*" >>"\$CHECK_COMMAND_LOG"
exec "$REAL_GIT" "\$@"
EOF
	chmod +x "$bin/git"

	cat >"$bin/curl" <<'EOF'
#!/bin/bash
echo 'curl must not be invoked by a check' >&2
exit 97
EOF
	chmod +x "$bin/curl"
}

make_fixture() {
	local root="$1"
	mkdir -p "$root/scripts" "$root/generated"
	cp "$CHECKER_SOURCE" "$root/scripts/ci-checks.sh"
	cp "$MAKEFILE_SOURCE" "$root/Makefile"
	chmod +x "$root/scripts/ci-checks.sh"
	cat >"$root/scripts/verify-release-pin.sh" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'release-pin-check\n' >>"$CHECK_COMMAND_LOG"
EOF
	chmod +x "$root/scripts/verify-release-pin.sh"
	printf 'module example.com/fixture\n\ngo 1.25.14\n' >"$root/go.mod"
	: >"$root/go.sum"
	printf 'package fixture\n' >"$root/fixture.go"
	printf 'baseline staged\n' >"$root/staged.txt"
	printf 'baseline unstaged\n' >"$root/unstaged.txt"
	printf 'baseline generated\n' >"$root/generated/mock.go"
	"$REAL_GIT" -C "$root" init -q
	"$REAL_GIT" -C "$root" config user.email test@example.com
	"$REAL_GIT" -C "$root" config user.name ci-check-test
	"$REAL_GIT" -C "$root" add .
	"$REAL_GIT" -C "$root" commit -qm fixture
}

prepare_dirty_fixture() {
	local root="$1"
	printf 'staged edit\n' >"$root/staged.txt"
	"$REAL_GIT" -C "$root" add staged.txt
	printf 'unstaged edit\n' >"$root/unstaged.txt"
	printf 'untracked sentinel\n' >"$root/untracked-sentinel.txt"
	cp "$root/staged.txt" "$root/staged.before"
	cp "$root/unstaged.txt" "$root/unstaged.before"
	cp "$root/untracked-sentinel.txt" "$root/untracked.before"
	"$REAL_GIT" -C "$root" diff --cached --binary >"$root/index.before"
	"$REAL_GIT" -C "$root" status --porcelain=v1 >"$root/status.before"
}

assert_dirty_fixture_unchanged() {
	local root="$1"
	cmp -s "$root/staged.before" "$root/staged.txt" || fail 'checker changed staged working bytes'
	cmp -s "$root/unstaged.before" "$root/unstaged.txt" || fail 'checker changed unstaged tracked bytes'
	cmp -s "$root/untracked.before" "$root/untracked-sentinel.txt" || fail 'checker changed untracked sentinel bytes'
	"$REAL_GIT" -C "$root" diff --cached --binary >"$root/index.after"
	cmp -s "$root/index.before" "$root/index.after" || fail 'checker changed the Git index'
	"$REAL_GIT" -C "$root" status --porcelain=v1 >"$root/status.after"
	# Ignore this test's own untracked snapshot files when comparing status.
	grep -vE '^\?\? (index|staged|status|unstaged|untracked)\.(before|after)$' "$root/status.before" >"$root/status.filtered.before" || true
	grep -vE '^\?\? (index|staged|status|unstaged|untracked)\.(before|after)$' "$root/status.after" >"$root/status.filtered.after" || true
	cmp -s "$root/status.filtered.before" "$root/status.filtered.after" || fail 'checker changed repository status'
}

assert_no_hidden_mutation_commands() {
	local log="$1"
	if grep -Eq '(^go generate|^go install|^git (checkout|reset|clean)|^curl )' "$log"; then
		cat "$log" >&2
		fail 'checker invoked a forbidden mutation command'
	fi
}

run_checker() {
	local root="$1"
	(
		cd "$root"
		PATH="$MOCK_BIN:$PATH" ./scripts/ci-checks.sh
	)
}

MOCK_BIN="$WORK/bin"
CHECK_COMMAND_LOG="$WORK/commands.log"
export MOCK_BIN CHECK_COMMAND_LOG
make_bin "$MOCK_BIN"

# A successful check preserves staged, unstaged, untracked, and index state.
repo="$WORK/success"
make_fixture "$repo"
prepare_dirty_fixture "$repo"
: >"$CHECK_COMMAND_LOG"
LINT_EXIT=0 GO_TEST_EXIT=0 run_checker "$repo" >"$WORK/success.out"
assert_dirty_fixture_unchanged "$repo"
assert_no_hidden_mutation_commands "$CHECK_COMMAND_LOG"

# A failing check has exactly the same preservation contract.
repo="$WORK/failure"
make_fixture "$repo"
prepare_dirty_fixture "$repo"
: >"$CHECK_COMMAND_LOG"
if LINT_EXIT=1 GO_TEST_EXIT=0 run_checker "$repo" >"$WORK/failure.out" 2>&1; then
	fail 'checker unexpectedly succeeded when the linter failed'
fi
assert_dirty_fixture_unchanged "$repo"
assert_no_hidden_mutation_commands "$CHECK_COMMAND_LOG"

# Generation remains an explicit, separate preparation command.
repo="$WORK/generate"
make_fixture "$repo"
: >"$CHECK_COMMAND_LOG"
(
	cd "$repo"
	PATH="$MOCK_BIN:$PATH" make mocks >/dev/null
)
grep -Fq 'go generate ./...' "$CHECK_COMMAND_LOG" || fail 'make mocks did not explicitly invoke go generate'
grep -Fq 'generated' "$repo/generated/mock.go" || fail 'explicit generation did not update generated output'

echo 'ci-check safety contract tests: PASS'
