#!/bin/bash
# toolkit-local-override.sh — iterate on one rpg-toolkit module in the local
# Docker loop without publish -> version tag -> `go get`.
#
# The synced source lives inside this checkout because Docker's build context
# is the rpg-api directory alone.  Exactly one allowlisted module may be
# replaced at a time; the state record binds the replacement to its target so
# refresh/status/off cannot act on an ambiguous or unowned tree.
set -euo pipefail

TARGET_ENCOUNTER='encounter'
TARGET_DND5E='rulebooks/dnd5e'
STATE_BASENAME='.toolkit-local-override-state'

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
STATE_FILE="$REPO_ROOT/local-toolkit/$STATE_BASENAME"

# These are populated by inspect_active_replacement and validate_state.
ACTIVE_COUNT=0
ACTIVE_MODULE=''
ACTIVE_REPLACE=''
ACTIVE_TARGET=''
STATE_TARGET=''
STATE_MODULE=''
STATE_REPLACE=''
STATE_SOURCE=''
STATE_REVISION=''
STATE_SYNCED_AT=''

module_for_target() {
	case "$1" in
	"$TARGET_ENCOUNTER")
		printf '%s\n' 'github.com/KirkDiggler/rpg-toolkit/encounter'
		;;
	"$TARGET_DND5E")
		printf '%s\n' 'github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e'
		;;
	*)
		printf 'error: unsupported override target: %s\n' "$1" >&2
		return 1
		;;
	esac
}

target_for_module() {
	case "$1" in
	github.com/KirkDiggler/rpg-toolkit/encounter)
		printf '%s\n' "$TARGET_ENCOUNTER"
		;;
	github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e)
		printf '%s\n' "$TARGET_DND5E"
		;;
	*)
		printf 'error: unsupported override module: %s\n' "$1" >&2
		return 1
		;;
	esac
}

usage() {
	echo "Usage: $0 on [--target encounter|rulebooks/dnd5e] [--src <path>]" >&2
	echo "       $0 refresh [--src <path>]" >&2
	echo "       $0 status" >&2
	echo "       $0 off" >&2
	exit 1
}

# inspect_active_replacement reads only go.mod's JSON replacement table.  It
# never reads the state file; callers validate the exact target path before
# they read ownership metadata.
inspect_active_replacement() {
	local json
	json="$(cd "$REPO_ROOT" && go mod edit -json)"
	ACTIVE_COUNT="$(jq -r '(.Replace // []) | length' <<<"$json")"
	ACTIVE_MODULE=''
	ACTIVE_REPLACE=''
	ACTIVE_TARGET=''

	if [ "$ACTIVE_COUNT" -eq 0 ]; then
		return 1
	fi
	if [ "$ACTIVE_COUNT" -ne 1 ]; then
		echo "error: exactly one local override is permitted; active replacements: $ACTIVE_COUNT" >&2
		return 1
	fi

	ACTIVE_MODULE="$(jq -r '.Replace[0].Old.Path // empty' <<<"$json")"
	ACTIVE_REPLACE="$(jq -r '.Replace[0].New.Path // empty' <<<"$json")"
	ACTIVE_TARGET="$(target_for_module "$ACTIVE_MODULE")" || {
		echo "error: unapproved local override for $ACTIVE_MODULE" >&2
		return 1
	}
	local expected
	expected="./local-toolkit/$ACTIVE_TARGET"
	if [ "$ACTIVE_REPLACE" != "$expected" ]; then
		echo "error: replacement for $ACTIVE_MODULE must be exactly $expected (got '$ACTIVE_REPLACE')" >&2
		return 1
	fi
	return 0
}

# validate_state parses literal key=value lines.  It deliberately never
# source-evaluates the file: values are data, including a hostile value such
# as $(touch /tmp/marker).  Every key is required exactly once and no unknown
# key is accepted.
validate_state() {
	local expected_target="$1"
	local expected_module="$2"
	local expected_replace="$3"
	local -A seen=()
	local line key value

	if [ ! -f "$STATE_FILE" ]; then
		echo "error: ownership state is missing: $STATE_FILE" >&2
		return 1
	fi

	while IFS= read -r line || [ -n "$line" ]; do
		if [[ "$line" != *=* ]]; then
			echo "error: invalid ownership state line" >&2
			return 1
		fi
		key="${line%%=*}"
		value="${line#*=}"
		if [ -z "$key" ] || [ -z "$value" ]; then
			echo "error: ownership state keys and values must be non-empty" >&2
			return 1
		fi
		case "$key" in
		target|module|replace|source|revision|synced_at) ;;
		*)
			echo "error: unknown ownership state key: $key" >&2
			return 1
			;;
		esac
		if [[ -n "${seen[$key]+present}" ]]; then
			echo "error: duplicate ownership state key: $key" >&2
			return 1
		fi
		seen["$key"]=1
		case "$key" in
		target) STATE_TARGET="$value" ;;
		module) STATE_MODULE="$value" ;;
		replace) STATE_REPLACE="$value" ;;
		source) STATE_SOURCE="$value" ;;
		revision) STATE_REVISION="$value" ;;
		synced_at) STATE_SYNCED_AT="$value" ;;
		esac
	done <"$STATE_FILE"

	local required
	for required in target module replace source revision synced_at; do
		if [[ -z "${seen[$required]+present}" ]]; then
			echo "error: missing ownership state key: $required" >&2
			return 1
		fi
	done

	if [ "$STATE_TARGET" != "$expected_target" ] ||
		[ "$STATE_MODULE" != "$expected_module" ] ||
		[ "$STATE_REPLACE" != "$expected_replace" ]; then
		echo "error: ownership state does not match the active replacement" >&2
		return 1
	fi
}

# require_active_override establishes a single exact replacement and then
# validates its ownership record.  It is used by refresh/status/off.
require_active_override() {
	if ! inspect_active_replacement; then
		if [ "$ACTIVE_COUNT" -eq 0 ]; then
			echo "error: no active allowlisted toolkit override" >&2
		fi
		return 1
	fi
	validate_state "$ACTIVE_TARGET" "$ACTIVE_MODULE" "$ACTIVE_REPLACE"
}

# assert_on_target accepts no replacement or the same exact owned target.  A
# different target, unknown replacement, second replacement, or unowned exact
# replacement fails before source validation or rsync.
assert_on_target() {
	local target="$1"
	local module
	module="$(module_for_target "$target")" || return 1
	if ! inspect_active_replacement; then
		if [ "$ACTIVE_COUNT" -eq 0 ]; then
			return 0
		fi
		return 1
	fi
	if [ "$ACTIVE_MODULE" != "$module" ]; then
		echo "error: target $target cannot be enabled while $ACTIVE_MODULE is active" >&2
		return 1
	fi
	validate_state "$ACTIVE_TARGET" "$ACTIVE_MODULE" "$ACTIVE_REPLACE"
}

# resolve_src finds a checkout containing the selected module.  An explicit
# path wins, followed by TOOLKIT_SRC_DIR, an existing state source (for a
# refresh/re-run), and the historical workspace guesses.
resolve_src() {
	local explicit="$1"
	local target="$2"
	local state_source="${3:-}"
	local candidates=()
	local c
	if [ -n "$explicit" ]; then
		candidates+=("$explicit")
	fi
	if [ -n "${TOOLKIT_SRC_DIR:-}" ]; then
		candidates+=("$TOOLKIT_SRC_DIR")
	fi
	if [ -n "$state_source" ]; then
		candidates+=("$state_source")
	fi
	candidates+=(
		"$HOME/game-dev/rpg-toolkit"
		"$REPO_ROOT/../../rpg-toolkit"
		"$REPO_ROOT/../../../rpg-toolkit"
	)
	for c in "${candidates[@]}"; do
		if [ -f "$c/$target/go.mod" ]; then
			(cd "$c" && pwd -P)
			return 0
		fi
	done
	echo "error: could not find an rpg-toolkit checkout with $target/go.mod." >&2
	echo "Pass --src <path>, or set TOOLKIT_SRC_DIR." >&2
	return 1
}

validate_source_module() {
	local src="$1"
	local target="$2"
	local module="$3"
	local go_mod="$src/$target/go.mod"
	local declared
	declared="$(awk '$1 == "module" { print $2; exit }' "$go_mod")"
	if [ "$declared" != "$module" ]; then
		echo "error: $go_mod declares '$declared', expected 'module $module'" >&2
		return 1
	fi
}

source_revision() {
	local src="$1"
	if git -C "$src" rev-parse --verify HEAD 2>/dev/null; then
		return 0
	fi
	printf '%s\n' unknown
}

write_state() {
	local target="$1"
	local module="$2"
	local replace="$3"
	local source="$4"
	local revision="$5"
	local synced_at
	local state_tmp
	synced_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	mkdir -p "$(dirname "$STATE_FILE")"
	state_tmp="$(mktemp "$REPO_ROOT/local-toolkit/$STATE_BASENAME.tmp.XXXXXX")"
	{
		printf 'target=%s\n' "$target"
		printf 'module=%s\n' "$module"
		printf 'replace=%s\n' "$replace"
		printf 'source=%s\n' "$source"
		printf 'revision=%s\n' "$revision"
		printf 'synced_at=%s\n' "$synced_at"
	} >"$state_tmp"
	mv -f "$state_tmp" "$STATE_FILE"
}

host_build() {
	local message="$1"
	if ! (cd "$REPO_ROOT" && go build ./...); then
		echo "error: host build failed $message" >&2
		return 1
	fi
}

sync_target() {
	local target="$1"
	local module="$2"
	local source="$3"
	local apply_replace="$4"
	local local_dir="$REPO_ROOT/local-toolkit/$target"
	local replace="./local-toolkit/$target"
	local revision

	# This check is intentionally before mkdir/rsync.
	validate_source_module "$source" "$target" "$module"
	revision="$(source_revision "$source")"
	echo "Syncing $source/$target/ -> $local_dir/ ..."
	mkdir -p "$local_dir"
	rsync -a --delete --exclude='.git' "$source/$target/" "$local_dir/"

	if [ "$apply_replace" = yes ]; then
		echo "Pointing go.mod at the local copy ..."
		(cd "$REPO_ROOT" && go mod edit -replace "$module=$replace")
	fi

	# State is written only after sync and the exact JSON replacement check.
	if ! inspect_active_replacement ||
		[ "$ACTIVE_TARGET" != "$target" ] ||
		[ "$ACTIVE_MODULE" != "$module" ] ||
		[ "$ACTIVE_REPLACE" != "$replace" ]; then
		echo "error: active replacement is not the exact selected target after sync" >&2
		return 1
	fi
	write_state "$target" "$module" "$replace" "$source" "$revision"
}

cmd_on() {
	local target="$TARGET_ENCOUNTER"
	local explicit=''
	while [ "$#" -gt 0 ]; do
		case "$1" in
		--target)
			[ "$#" -ge 2 ] || usage
			target="$2"
			shift 2
			;;
		--src)
			[ "$#" -ge 2 ] || usage
			explicit="$2"
			shift 2
			;;
		*) usage ;;
		esac
	done
	local module
	module="$(module_for_target "$target")" || exit 1
	assert_on_target "$target"

	local state_source=''
	if [ "$ACTIVE_COUNT" -eq 1 ] && [ "$ACTIVE_TARGET" = "$target" ]; then
		state_source="$STATE_SOURCE"
	fi
	local source
	source="$(resolve_src "$explicit" "$target" "$state_source")"
	sync_target "$target" "$module" "$source" yes
	host_build "against the synced local copy — check the toolkit edit for compile errors."

	cat <<EOF

Override ON. go.mod now replaces $module with ./local-toolkit/$target.

Next steps:
  docker build -f Dockerfile.local-toolkit -t rpg-api:local $REPO_ROOT
  (cd /home/kirk/game-dev/rpg-deployment && \\
   docker compose -f docker-compose.local-dev.yml \\
                  -f docker-compose.local-api-src.yml \\
                  up -d rpg-api)

Re-run '$0 on --target $target' after every toolkit edit to re-sync.
Run '$0 off' when done — publish the toolkit change, bump the version tag,
then remove the override (never leave it active on a branch you push).
EOF
}

cmd_refresh() {
	local explicit=''
	while [ "$#" -gt 0 ]; do
		case "$1" in
		--src)
			[ "$#" -ge 2 ] || usage
			explicit="$2"
			shift 2
			;;
		*) usage ;;
		esac
	done
	require_active_override
	local source
	source="$(resolve_src "$explicit" "$ACTIVE_TARGET" "$STATE_SOURCE")"
	sync_target "$ACTIVE_TARGET" "$ACTIVE_MODULE" "$source" no
	host_build "after refresh — check the toolkit edit for compile errors."
	echo "Override refreshed for $ACTIVE_MODULE from $source."
}

cmd_status() {
	if ! inspect_active_replacement; then
		if [ "$ACTIVE_COUNT" -eq 0 ]; then
			echo 'Active replace modules (0): none'
			echo 'Override OFF: no allowlisted local replacement is active.'
			return 0
		fi
		return 1
	fi
	validate_state "$ACTIVE_TARGET" "$ACTIVE_MODULE" "$ACTIVE_REPLACE"
	echo "Active replace modules (1): $ACTIVE_MODULE"
	echo "Override ON: $ACTIVE_MODULE => $ACTIVE_REPLACE"
	echo "Target: $ACTIVE_TARGET"
	echo "Source: $STATE_SOURCE"
	if [ -d "$REPO_ROOT/local-toolkit/$ACTIVE_TARGET" ]; then
		echo "local-toolkit/$ACTIVE_TARGET/ exists on disk."
	else
		echo "error: synced directory is missing: local-toolkit/$ACTIVE_TARGET" >&2
		return 1
	fi
}

cmd_off() {
	if ! inspect_active_replacement; then
		if [ "$ACTIVE_COUNT" -ne 0 ]; then
			return 1
		fi
		if [ -e "$STATE_FILE" ]; then
			echo "error: ownership state exists without an active replacement" >&2
			return 1
		fi
		host_build "with no active override."
		echo 'Override already OFF: no active allowlisted local replacement.'
		return 0
	fi
	# All ownership checks happen before the first mutation.
	validate_state "$ACTIVE_TARGET" "$ACTIVE_MODULE" "$ACTIVE_REPLACE"
	local target_dir="$REPO_ROOT/local-toolkit/$ACTIVE_TARGET"
	echo "Dropping the replace directive ..."
	(cd "$REPO_ROOT" && go mod edit -dropreplace "$ACTIVE_MODULE")
	echo "Removing $target_dir ..."
	rm -rf "$target_dir"
	rm -f "$STATE_FILE"
	# Only empty structural parents are removed; an unrelated sibling survives.
	rmdir "$(dirname "$target_dir")" 2>/dev/null || true
	host_build "after removing the override — check go.mod/go.sum."
	echo "Override OFF. $ACTIVE_MODULE is back on its published pin."
}

case "${1:-}" in
on)
	shift
	cmd_on "$@"
	;;
refresh)
	shift
	cmd_refresh "$@"
	;;
status)
	shift
	[ "$#" -eq 0 ] || usage
	cmd_status
	;;
off)
	shift
	[ "$#" -eq 0 ] || usage
	cmd_off
	;;
*) usage ;;
esac
