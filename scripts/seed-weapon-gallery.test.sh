#!/bin/bash
# Hermetic contract tests for seed-weapon-gallery.sh.
set -euo pipefail

SCRIPT_SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/seed-weapon-gallery.sh"
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

make_repo() {
	local root="$1"
	mkdir -p "$root/scripts" "$root/cmd/sandboxseed"
	if [ ! -f "$SCRIPT_SOURCE" ]; then
		fail "expected source script at $SCRIPT_SOURCE"
	fi
	cp "$SCRIPT_SOURCE" "$root/scripts/seed-weapon-gallery.sh"
	chmod +x "$root/scripts/seed-weapon-gallery.sh"
	cat >"$root/cmd/sandboxseed/main.go" <<'EOF'
package main

func main() {}
EOF
	git -C "$root" init -q
	git -C "$root" config user.email test@example.com
	git -C "$root" config user.name seed-wrapper-test
	git -C "$root" add .
	git -C "$root" commit -qm fixture
}

make_bin() {
	local bin="$1"
	mkdir -p "$bin"
	cat >"$bin/docker" <<'EOF'
#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >>"$MOCK_DOCKER_LOG"
case "${MOCK_DOCKER_MODE:-ip}" in
ip)
	printf '%s\n' "${MOCK_DOCKER_IP:-172.22.0.9}"
	;;
missing)
	exit 1
	;;
empty)
	printf '\n'
	;;
*)
	echo "unsupported MOCK_DOCKER_MODE=$MOCK_DOCKER_MODE" >&2
	exit 1
	;;
esac
EOF
	chmod +x "$bin/docker"
	cat >"$bin/go" <<'EOF'
#!/bin/bash
set -euo pipefail
printf '%s\n' "$PWD" >"$MOCK_GO_PWD"
printf '%s\0' "$@" >"$MOCK_GO_ARGS"
EOF
	chmod +x "$bin/go"
}

assert_go_args() {
	local expected="$1"
	python3 - "$MOCK_GO_ARGS" "$expected" <<'EOF'
import sys
path, expected = sys.argv[1], sys.argv[2].split('|')
with open(path, 'rb') as fh:
    actual = [arg.decode() for arg in fh.read().split(b'\0') if arg]
if actual != expected:
    raise SystemExit(f"got {actual!r}, want {expected!r}")
EOF
}

run_wrapper() {
	local repo="$1"
	shift
	(
		cd "$repo/cmd/sandboxseed"
		PATH="$MOCK_BIN:$PATH" "$repo/scripts/seed-weapon-gallery.sh" "$@"
	)
}

expect_failure() {
	local label="$1"
	shift
	local output
	if output=$("$@" 2>&1); then
		echo "$output" >&2
		fail "$label unexpectedly succeeded"
	fi
	printf '%s' "$output"
}

MOCK_BIN="$WORK/bin"
MOCK_DOCKER_LOG="$WORK/docker.log"
MOCK_GO_ARGS="$WORK/go.args"
MOCK_GO_PWD="$WORK/go.pwd"
export MOCK_DOCKER_LOG MOCK_GO_ARGS MOCK_GO_PWD
make_bin "$MOCK_BIN"

repo="$WORK/repo-default"
make_repo "$repo"
MOCK_DOCKER_MODE=ip MOCK_DOCKER_IP=172.22.0.9 run_wrapper "$repo" >"$WORK/default.out"
assert_file_contains 'inspect' "$MOCK_DOCKER_LOG"
assert_file_contains 'rpg-dev-redis-1' "$MOCK_DOCKER_LOG"
assert_go_args 'run|./cmd/sandboxseed|--fixture=weapon-gallery|--address=localhost:8080|--redis-address=172.22.0.9:6379'
assert_file_contains "$repo" "$MOCK_GO_PWD"
assert_file_contains 'API=localhost:8080' "$WORK/default.out"
assert_file_contains 'Redis=172.22.0.9:6379' "$WORK/default.out"

: >"$MOCK_DOCKER_LOG"
repo="$WORK/repo-override"
make_repo "$repo"
RPG_API_ADDRESS=envoy.internal:18080 \
RPG_REDIS_ADDRESS=redis.internal:6380 \
MOCK_DOCKER_MODE=missing \
run_wrapper "$repo" >"$WORK/override.out"
if [ -s "$MOCK_DOCKER_LOG" ]; then
	fail 'docker should not be called when RPG_REDIS_ADDRESS is set'
fi
assert_go_args 'run|./cmd/sandboxseed|--fixture=weapon-gallery|--address=envoy.internal:18080|--redis-address=redis.internal:6380'
assert_file_contains 'API=envoy.internal:18080' "$WORK/override.out"
assert_file_contains 'Redis=redis.internal:6380' "$WORK/override.out"

: >"$MOCK_DOCKER_LOG"
repo="$WORK/repo-missing"
make_repo "$repo"
output=$(MOCK_DOCKER_MODE=missing expect_failure missing-container run_wrapper "$repo")
assert_file_contains 'inspect' "$MOCK_DOCKER_LOG"
assert_file_contains 'rpg-dev-redis-1' "$MOCK_DOCKER_LOG"
printf '%s' "$output" >"$WORK/missing.err"
assert_file_contains 'could not resolve Redis from Docker container rpg-dev-redis-1' "$WORK/missing.err"
assert_file_contains 'Set RPG_REDIS_ADDRESS explicitly or ensure Docker and the container are available.' "$WORK/missing.err"

: >"$MOCK_DOCKER_LOG"
repo="$WORK/repo-empty-ip"
make_repo "$repo"
output=$(MOCK_DOCKER_MODE=empty expect_failure empty-ip run_wrapper "$repo")
assert_file_contains 'inspect' "$MOCK_DOCKER_LOG"
printf '%s' "$output" >"$WORK/empty.err"
assert_file_contains 'could not resolve Redis from Docker container rpg-dev-redis-1' "$WORK/empty.err"

echo 'seed-weapon-gallery wrapper tests: PASS'
