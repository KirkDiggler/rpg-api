#!/bin/bash
# Hermetic contract tests for toolkit-local-override.sh.
set -euo pipefail

SCRIPT_SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/toolkit-local-override.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

assert_file_contains() {
	local needle="$1"
	local file="$2"
	grep -Fq -- "$needle" "$file" || fail "expected '$needle' in $file"
}

assert_file_not_contains() {
	local needle="$1"
	local file="$2"
	if grep -Fq -- "$needle" "$file"; then
		fail "did not expect '$needle' in $file"
	fi
}

make_toolkit() {
	local root="$1"
	mkdir -p "$root/encounter" "$root/rulebooks/dnd5e"
	cat >"$root/encounter/go.mod" <<'EOF'
module github.com/KirkDiggler/rpg-toolkit/encounter

go 1.22
EOF
	cat >"$root/encounter/encounter.go" <<'EOF'
package encounter

const Name = "encounter"
EOF
	cat >"$root/rulebooks/dnd5e/go.mod" <<'EOF'
module github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e

go 1.22
EOF
	cat >"$root/rulebooks/dnd5e/dnd5e.go" <<'EOF'
package dnd5e

const Name = "dnd5e"
EOF
	git -C "$root" init -q
	git -C "$root" config user.email test@example.com
	git -C "$root" config user.name toolkit-test
	git -C "$root" add .
	git -C "$root" commit -qm fixture
}

make_api() {
	local root="$1"
	local target="$2"
	local module
	case "$target" in
	encounter) module=github.com/KirkDiggler/rpg-toolkit/encounter ;;
	rulebooks/dnd5e) module=github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e ;;
	*) fail "unsupported fixture target $target" ;;
	esac
	mkdir -p "$root/scripts"
	cp "$SCRIPT_SOURCE" "$root/scripts/toolkit-local-override.sh"
	chmod +x "$root/scripts/toolkit-local-override.sh"
	cat >"$root/go.mod" <<EOF
module example.com/api

go 1.22

require $module v0.0.0
EOF
	cat >"$root/main.go" <<'EOF'
package main

func main() {}
EOF
	git -C "$root" init -q
	git -C "$root" config user.email test@example.com
	git -C "$root" config user.name api-test
	git -C "$root" add .
	git -C "$root" commit -qm fixture
}

run_override() {
	local api="$1"
	shift
	(cd "$api" && scripts/toolkit-local-override.sh "$@")
}

expect_failure() {
	local label="$1"
	shift
	local output
	if output=$("$@" 2>&1); then
		echo "$output" >&2
		fail "$label unexpectedly succeeded"
	fi
}

snapshot_fixture() {
	local api="$1"
	local snapshot="$2"
	cp "$api/go.mod" "$snapshot.go.mod"
	if [ -e "$api/local-toolkit" ]; then
		cp -a "$api/local-toolkit" "$snapshot.local-toolkit"
	else
		: >"$snapshot.no-local-toolkit"
	fi
}

assert_snapshot_unchanged() {
	local api="$1"
	local snapshot="$2"
	cmp -s "$snapshot.go.mod" "$api/go.mod" || fail "go.mod changed unexpectedly"
	if [ -e "$snapshot.no-local-toolkit" ]; then
		test ! -e "$api/local-toolkit" || fail "local-toolkit changed unexpectedly"
	else
		diff -ruN "$snapshot.local-toolkit" "$api/local-toolkit" >/dev/null || fail "local-toolkit changed unexpectedly"
	fi
}

write_state() {
	local api="$1"
	local target="$2"
	local module="$3"
	local replace="$4"
	local source="$5"
	mkdir -p "$api/local-toolkit"
	cat >"$api/local-toolkit/.toolkit-local-override-state" <<EOF
target=$target
module=$module
replace=$replace
source=$source
revision=fixture-revision
synced_at=2026-01-01T00:00:00Z
EOF
}

seed_exact_override() {
	local api="$1"
	local target="$2"
	local module
	case "$target" in
	encounter) module=github.com/KirkDiggler/rpg-toolkit/encounter ;;
	rulebooks/dnd5e) module=github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e ;;
	esac
	mkdir -p "$api/local-toolkit/$target"
	(cd "$api" && go mod edit -replace "$module=./local-toolkit/$target")
	write_state "$api" "$target" "$module" "./local-toolkit/$target" "$WORK/toolkit"
}

# Primary D&D 5e lifecycle: on, refresh, status, and off.
toolkit="$WORK/toolkit"
make_toolkit "$toolkit"
api="$WORK/api-dnd5e"
make_api "$api" rulebooks/dnd5e
run_override "$api" on --target rulebooks/dnd5e --src "$toolkit"
assert_file_contains 'github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e => ./local-toolkit/rulebooks/dnd5e' "$api/go.mod"
printf '\nconst Refreshed = true\n' >>"$toolkit/rulebooks/dnd5e/dnd5e.go"
run_override "$api" refresh --src "$toolkit"
grep -Fq 'const Refreshed = true' "$api/local-toolkit/rulebooks/dnd5e/dnd5e.go" || fail 'refresh did not sync source changes'
run_override "$api" status >"$WORK/status.txt"
assert_file_contains 'rulebooks/dnd5e' "$WORK/status.txt"
run_override "$api" off
test ! -e "$api/local-toolkit/rulebooks/dnd5e"
test ! -e "$api/local-toolkit/.toolkit-local-override-state"
test ! -e "$api/local-toolkit"

# The no-target encounter path remains supported.
api="$WORK/api-default-encounter"
make_api "$api" encounter
run_override "$api" on --src "$toolkit"
assert_file_contains 'github.com/KirkDiggler/rpg-toolkit/encounter => ./local-toolkit/encounter' "$api/go.mod"
run_override "$api" off

# A source declaring a different module must fail before either sync or go.mod edit.
bad_source="$WORK/bad-source"
mkdir -p "$bad_source/rulebooks/dnd5e"
cat >"$bad_source/rulebooks/dnd5e/go.mod" <<'EOF'
module example.com/not-allowed

go 1.22
EOF
snapshot_fixture "$api" "$WORK/bad-source-before"
expect_failure bad-source run_override "$api" on --target rulebooks/dnd5e --src "$bad_source"
assert_snapshot_unchanged "$api" "$WORK/bad-source-before"

# Unknown and second active replacements are rejected before rsync.
api="$WORK/api-unknown"
make_api "$api" rulebooks/dnd5e
(cd "$api" && go mod edit -replace example.com/unknown=./local-toolkit/unknown)
snapshot_fixture "$api" "$WORK/unknown-before"
expect_failure unknown-replace run_override "$api" on --target rulebooks/dnd5e --src "$toolkit"
assert_snapshot_unchanged "$api" "$WORK/unknown-before"

api="$WORK/api-second"
make_api "$api" rulebooks/dnd5e
run_override "$api" on --target rulebooks/dnd5e --src "$toolkit"
mkdir -p "$api/local-toolkit/encounter"
printf 'keep\n' >"$api/local-toolkit/encounter/keep.txt"
(cd "$api" && go mod edit -replace example.com/second=./local-toolkit/second)
snapshot_fixture "$api" "$WORK/second-before"
expect_failure second-replace run_override "$api" on --target rulebooks/dnd5e --src "$toolkit"
assert_snapshot_unchanged "$api" "$WORK/second-before"
(cd "$api" && go mod edit -dropreplace example.com/second)
run_override "$api" off
[ -e "$api/local-toolkit/encounter/keep.txt" ] || fail 'off removed unrelated local-toolkit sibling'
test "$(cd "$api" && go mod edit -json | jq -r '(.Replace // []) | length')" -eq 0 || fail 'replacement remained after off'

# Every lifecycle command must refuse an unowned RHS, missing state, and
# mismatched ownership without changing go.mod or local-toolkit.
for operation in refresh status off; do
	for bad_case in bad-rhs missing-state mismatched-state; do
		api="$WORK/api-$operation-${bad_case//-/_}"
		make_api "$api" rulebooks/dnd5e
		seed_exact_override "$api" rulebooks/dnd5e
		case "$bad_case" in
		bad-rhs)
			(cd "$api" && go mod edit -replace github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e=./local-toolkit/not-the-target)
			;;
		missing-state)
			rm "$api/local-toolkit/.toolkit-local-override-state"
			;;
		mismatched-state)
			write_state "$api" encounter github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e ./local-toolkit/rulebooks/dnd5e "$toolkit"
			;;
		esac
		snapshot_fixture "$api" "$WORK/$operation-${bad_case//-/_}-before"
		case "$operation" in
		refresh) expect_failure "$operation/$bad_case" run_override "$api" refresh --src "$toolkit" ;;
		status|off) expect_failure "$operation/$bad_case" run_override "$api" "$operation" ;;
		esac
		assert_snapshot_unchanged "$api" "$WORK/$operation-${bad_case//-/_}-before"
	done
done

# Duplicate and missing keys are rejected without evaluating the state file.
api="$WORK/api-state-parser"
make_api "$api" rulebooks/dnd5e
seed_exact_override "$api" rulebooks/dnd5e
state="$api/local-toolkit/.toolkit-local-override-state"
cat >>"$state" <<'EOF'
target=rulebooks/dnd5e
EOF
snapshot_fixture "$api" "$WORK/duplicate-before"
expect_failure duplicate-state run_override "$api" status
assert_snapshot_unchanged "$api" "$WORK/duplicate-before"

api="$WORK/api-state-missing"
make_api "$api" rulebooks/dnd5e
seed_exact_override "$api" rulebooks/dnd5e
state="$api/local-toolkit/.toolkit-local-override-state"
sed -i '/^revision=/d' "$state"
snapshot_fixture "$api" "$WORK/missing-key-before"
expect_failure missing-state-key run_override "$api" status
assert_snapshot_unchanged "$api" "$WORK/missing-key-before"

api="$WORK/api-state-no-source"
make_api "$api" rulebooks/dnd5e
seed_exact_override "$api" rulebooks/dnd5e
state="$api/local-toolkit/.toolkit-local-override-state"
marker="$WORK/state-evaluated"
sed -i "s#^source=.*#source=\$(touch '$marker')#" "$state"
run_override "$api" status >/dev/null
test ! -e "$marker" || fail 'state file was evaluated as shell code'

# Switching targets while one exact target is active is rejected before any
# source validation, rsync, or go.mod mutation. Test both directions.
api="$WORK/api-switch-from-dnd5e"
make_api "$api" rulebooks/dnd5e
run_override "$api" on --target rulebooks/dnd5e --src "$toolkit"
snapshot_fixture "$api" "$WORK/switch-from-dnd5e-before"
expect_failure switch-from-dnd5e run_override "$api" on --target encounter --src "$toolkit"
assert_snapshot_unchanged "$api" "$WORK/switch-from-dnd5e-before"
run_override "$api" off

api="$WORK/api-switch-from-encounter"
make_api "$api" encounter
run_override "$api" on --src "$toolkit"
snapshot_fixture "$api" "$WORK/switch-from-encounter-before"
expect_failure switch-from-encounter run_override "$api" on --target rulebooks/dnd5e --src "$toolkit"
assert_snapshot_unchanged "$api" "$WORK/switch-from-encounter-before"
run_override "$api" off

echo 'toolkit-local-override contract tests passed'
