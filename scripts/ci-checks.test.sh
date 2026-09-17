#!/bin/bash
# Hermetic safety tests for ci-checks.sh. Every checker invocation runs in a
# disposable Git repository with stubbed tools; this test never points the
# checker at the caller's working tree.
set -euo pipefail

# The hermeticity above is only true if the disposable git calls below
# resolve INSIDE the disposable repository. Git exports its complete
# repository-local environment (GIT_DIR, GIT_INDEX_FILE, GIT_COMMON_DIR,
# GIT_CONFIG*, GIT_OBJECT_DIRECTORY and the rest of
# `git rev-parse --local-env-vars`) to hooks, so when this test runs from a
# pre-commit hook in a linked worktree, inheriting them pointed every
# "git -C $root" call at the CALLER's repository: the fixture was committed
# to the caller's branch, the caller's index was replaced and the caller's
# COMMON config corrupted (the API1003 hook incident, 2026-09-17). Unset the
# COMPLETE repository-local list — plus the numbered GIT_CONFIG_KEY_n/
# GIT_CONFIG_VALUE_n entries the count-based mechanism reads, which no
# static list can spell — inside this subprocess only, BEFORE any fixture
# init/config/add/commit, so $root is genuinely disposable no matter where
# the test was invoked from. The caller's own environment is not touched:
# these unsets live in this script's process and its children only.
# Assignment propagates discovery failure under set -e; a process substitution
# would hide it and could leave a redirecting Git context active.
git_local_env_vars="$(git rev-parse --local-env-vars)"
while IFS= read -r var; do
	[ -n "$var" ] && unset "$var"
done <<<"$git_local_env_vars"
unset GIT_CONFIG GIT_CONFIG_PARAMETERS GIT_CONFIG_COUNT
while IFS= read -r var; do
	unset "$var"
done < <(env | sed -n 's/^\(GIT_CONFIG_KEY_[0-9]*\|GIT_CONFIG_VALUE_[0-9]*\)=.*/\1/p')

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

# The API1003 hook incident (2026-09-17), as a permanent regression. This
# test ran from a pre-commit hook in a LINKED WORKTREE and inherited git's
# repository-local environment (GIT_DIR, GIT_INDEX_FILE, GIT_COMMON_DIR,
# GIT_CONFIG*, ...); the fixture git calls above then resolved into the
# CALLER's repository — the fixture was committed to the caller's branch,
# the caller's index was replaced and the caller's COMMON config corrupted.
# The sanitization at the top of this file is the fix; this scenario is its
# durable proof: it re-invokes the WHOLE test the way a linked-worktree hook
# sees it — repository-local environment pointed at a disposable caller that
# carries HEAD/ref, index, working bytes, COMMON config and an existing
# stash — and asserts the caller is untouched and the test still passes.
#
# BOUNDED SELF-INVOCATION: the nested run is marked CI_CHECKS_TEST_DEPTH=1
# and skips this scenario, so the recursion cannot go deeper than one level.
scenario_inherited_hook_context() {
	[ "${CI_CHECKS_TEST_DEPTH:-0}" -ge 1 ] && return 0

	local caller="$WORK/hook-caller"
	local wt="$WORK/hook-wt"
	"$REAL_GIT" init -q -b main "$caller"
	"$REAL_GIT" -C "$caller" config user.name ci-check-test
	"$REAL_GIT" -C "$caller" config user.email test@example.com
	printf 'caller base\n' >"$caller/caller.txt"
	printf 'baseline unstaged\n' >"$caller/unstaged.txt"
	"$REAL_GIT" -C "$caller" add caller.txt unstaged.txt
	"$REAL_GIT" -C "$caller" commit -qm caller-base

	# A linked worktree: the shape that makes a hook export the hostile
	# environment in the first place.
	"$REAL_GIT" -C "$caller" worktree add -q "$WORK/hook-wt" -b wtbranch

	# An EXISTING stash, so a repeat of the escape is caught even when every
	# other datum happens to survive.
	printf 'stash payload\n' >"$caller/stashme.txt"
	"$REAL_GIT" -C "$caller" add stashme.txt
	"$REAL_GIT" -C "$caller" stash push -q -m incident-signal

	# State the escape would have hit: staged, unstaged and untracked bytes.
	printf 'staged edit\n' >"$caller/staged.txt"
	"$REAL_GIT" -C "$caller" add staged.txt
	printf 'unstaged edit\n' >"$caller/unstaged.txt"
	printf 'untracked sentinel\n' >"$caller/untracked.txt"

	local caller_gitdir wt_gitdir
	caller_gitdir="$($REAL_GIT -C "$caller" rev-parse --absolute-git-dir)"
	wt_gitdir="$($REAL_GIT -C "$wt" rev-parse --absolute-git-dir)"

	# The exported index belongs to the linked worktree, not the primary
	# checkout. Give that exact target its own nonempty state to preserve.
	printf 'worktree staged\n' >"$wt/staged.txt"
	"$REAL_GIT" -C "$wt" add staged.txt
	printf 'worktree unstaged\n' >"$wt/unstaged.txt"
	printf 'worktree untracked\n' >"$wt/untracked.txt"
	"$REAL_GIT" -C "$wt" rev-parse HEAD >"$WORK/wt-head.before"
	"$REAL_GIT" -C "$wt" diff --cached --binary >"$WORK/wt-index.before"
	"$REAL_GIT" -C "$wt" status --porcelain=v1 >"$WORK/wt-status.before"
	sha256sum "$wt/staged.txt" "$wt/unstaged.txt" "$wt/untracked.txt" >"$WORK/wt-bytes.before"

	"$REAL_GIT" -C "$caller" rev-parse HEAD >"$WORK/hook-head.before"
	"$REAL_GIT" -C "$caller" for-each-ref >"$WORK/hook-refs.before"
	"$REAL_GIT" -C "$caller" diff --cached --binary >"$WORK/hook-index.before"
	"$REAL_GIT" -C "$caller" status --porcelain=v1 >"$WORK/hook-status.before"
	"$REAL_GIT" -C "$caller" stash list >"$WORK/hook-stashlist.before"
	cp "$caller/caller.txt" "$WORK/hook-bytes.before"
	cp "$caller/untracked.txt" "$WORK/hook-untracked.before"
	cp "$caller_gitdir/config" "$WORK/hook-commonconfig.before"

	# The hostile context, as a linked-worktree hook exports it (GIT_DIR and
	# GIT_INDEX_FILE) plus the COMPLETE repository-local list pointed at the
	# caller — the strongest form the fix has to survive.
	export GIT_DIR="$wt_gitdir"
	export GIT_INDEX_FILE="$wt_gitdir/index"
	export GIT_COMMON_DIR="$caller_gitdir"
	export GIT_OBJECT_DIRECTORY="$caller_gitdir/objects"
	export GIT_ALTERNATE_OBJECT_DIRECTORIES="$caller_gitdir/objects"
	export GIT_CONFIG="$caller_gitdir/config"
	export GIT_WORK_TREE="$WORK/hook-wt"
	export GIT_CONFIG_COUNT=1
	export GIT_CONFIG_KEY_0=caller.safety
	export GIT_CONFIG_VALUE_0=hostile

	if ! CI_CHECKS_TEST_DEPTH=1 bash "$SOURCE_DIR/scripts/ci-checks.test.sh" \
		>"$WORK/hook-context.out" 2>&1; then
		cat "$WORK/hook-context.out" >&2
		fail 'the test did not pass under inherited linked-worktree hook context'
	fi

	# The after-snapshots run in a SANITIZED subshell: reading the caller
	# back while the hostile variables are still exported would read through
	# them (GIT_INDEX_FILE would answer with the worktree's index, not the
	# caller's) — the same redirection this scenario exists to catch.
	(
		unset GIT_DIR GIT_INDEX_FILE GIT_COMMON_DIR GIT_OBJECT_DIRECTORY \
			GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CONFIG GIT_WORK_TREE \
			GIT_CONFIG_COUNT GIT_CONFIG_KEY_0 GIT_CONFIG_VALUE_0
		while IFS= read -r var; do
			unset "$var"
		done < <(env | sed -n 's/^\(GIT_CONFIG_KEY_[0-9]*\|GIT_CONFIG_VALUE_[0-9]*\)=.*/\1/p')
		"$REAL_GIT" -C "$caller" rev-parse HEAD >"$WORK/hook-head.after"
		"$REAL_GIT" -C "$caller" for-each-ref >"$WORK/hook-refs.after"
		"$REAL_GIT" -C "$caller" diff --cached --binary >"$WORK/hook-index.after"
		"$REAL_GIT" -C "$caller" status --porcelain=v1 >"$WORK/hook-status.after"
		"$REAL_GIT" -C "$caller" stash list >"$WORK/hook-stashlist.after"
		"$REAL_GIT" -C "$wt" rev-parse HEAD >"$WORK/wt-head.after"
		"$REAL_GIT" -C "$wt" diff --cached --binary >"$WORK/wt-index.after"
		"$REAL_GIT" -C "$wt" status --porcelain=v1 >"$WORK/wt-status.after"
		sha256sum "$wt/staged.txt" "$wt/unstaged.txt" "$wt/untracked.txt" >"$WORK/wt-bytes.after"
	)
	cmp -s "$WORK/hook-head.before" "$WORK/hook-head.after" \
		|| fail 'hook-context run moved the caller HEAD'
	cmp -s "$WORK/hook-refs.before" "$WORK/hook-refs.after" \
		|| fail 'hook-context run changed the caller refs'
	cmp -s "$WORK/hook-index.before" "$WORK/hook-index.after" \
		|| fail 'hook-context run changed the caller index'
	cmp -s "$WORK/hook-status.before" "$WORK/hook-status.after" \
		|| fail 'hook-context run changed the caller status'
	cmp -s "$WORK/hook-stashlist.before" "$WORK/hook-stashlist.after" \
		|| fail 'hook-context run changed the caller stash identity'
	cmp -s "$WORK/hook-bytes.before" "$caller/caller.txt" \
		|| fail 'hook-context run changed the caller working bytes'
	cmp -s "$WORK/hook-untracked.before" "$caller/untracked.txt" \
		|| fail 'hook-context run changed the caller untracked bytes'
	cmp -s "$WORK/hook-commonconfig.before" "$caller_gitdir/config" \
		|| fail 'hook-context run changed the caller COMMON git config'
	for datum in head index status bytes; do
		cmp -s "$WORK/wt-$datum.before" "$WORK/wt-$datum.after" \
			|| fail "hook-context run changed linked-worktree $datum"
	done

	unset GIT_DIR GIT_INDEX_FILE GIT_COMMON_DIR GIT_OBJECT_DIRECTORY \
		GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CONFIG GIT_WORK_TREE \
		GIT_CONFIG_COUNT GIT_CONFIG_KEY_0 GIT_CONFIG_VALUE_0
}

scenario_inherited_hook_context

echo 'ci-check safety contract tests: PASS'
