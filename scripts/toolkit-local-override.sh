#!/bin/bash
# toolkit-local-override.sh — iterate on one rpg-toolkit module in the local
# Docker loop without publish -> version tag -> `go get`.
#
# The synced source lives inside this checkout because Docker's build context
# is the rpg-api directory alone.  Exactly one allowlisted target may be
# replaced at a time; the state record binds the replacements to their target
# so refresh/status/off cannot act on an ambiguous or unowned tree.
#
# rulebooks/dnd5e is an umbrella: directories below it with their own go.mod
# (session, encounter, resolution, ...) are SEPARATE Go modules.  Replacing
# only the umbrella leaves those on their published tags, so the api silently
# builds old code for exactly the modules a slice changes.  The dnd5e target
# therefore also replaces every nested module that go.mod requires and the
# synced tree provides, and fails loudly when a required one is missing.
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
ACTIVE_NESTED=''
STATE_TARGET=''
STATE_MODULE=''
STATE_REPLACE=''
STATE_SOURCE=''
STATE_REVISION=''
STATE_SYNCED_AT=''
STATE_NESTED=''

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

# target_allows_nested reports whether a target's nested modules are replaced
# alongside it.  The legacy encounter target stays a single replacement.
target_allows_nested() {
	[ "$1" = "$TARGET_DND5E" ]
}

# join_sorted prints its arguments sorted and comma-joined ('' for none).
join_sorted() {
	if [ "$#" -eq 0 ]; then
		printf '\n'
		return 0
	fi
	printf '%s\n' "$@" | LC_ALL=C sort -u | paste -sd, -
}

# required_nested prints every module go.mod requires (direct or indirect)
# strictly below the given umbrella module path.
required_nested() {
	local module="$1"
	(cd "$REPO_ROOT" && go mod edit -json) |
		jq -r --arg prefix "$module/" '(.Require // [])[] | .Path | select(startswith($prefix))'
}

usage() {
	echo "Usage: $0 on [--target encounter|rulebooks/dnd5e] [--src <path>]" >&2
	echo "       $0 refresh [--src <path>]" >&2
	echo "       $0 status" >&2
	echo "       $0 off" >&2
	exit 1
}

# inspect_active_replacement reads only go.mod's JSON replacement table.  It
# never reads the state file; callers validate the exact target paths before
# they read ownership metadata.  Exactly one allowlisted target root must be
# replaced.  Any other replacement must be a module nested below that root
# (dnd5e target only) pointing at its directory inside the synced copy.
inspect_active_replacement() {
	local json
	json="$(cd "$REPO_ROOT" && go mod edit -json)"
	ACTIVE_COUNT="$(jq -r '(.Replace // []) | length' <<<"$json")"
	ACTIVE_MODULE=''
	ACTIVE_REPLACE=''
	ACTIVE_TARGET=''
	ACTIVE_NESTED=''

	if [ "$ACTIVE_COUNT" -eq 0 ]; then
		return 1
	fi

	local -a olds=() news=()
	local old new
	while IFS=$'\t' read -r old new; do
		olds+=("$old")
		news+=("$new")
	done < <(jq -r '.Replace[] | [.Old.Path, (.New.Path // "")] | @tsv' <<<"$json")

	local i root_index=-1 roots=0
	for i in "${!olds[@]}"; do
		if target_for_module "${olds[$i]}" >/dev/null 2>&1; then
			roots=$((roots + 1))
			root_index="$i"
		fi
	done
	if [ "$roots" -eq 0 ]; then
		echo "error: unapproved local override for ${olds[0]}" >&2
		return 1
	fi
	if [ "$roots" -ne 1 ]; then
		echo "error: exactly one local override target is permitted; active targets: $roots" >&2
		return 1
	fi

	ACTIVE_MODULE="${olds[$root_index]}"
	ACTIVE_REPLACE="${news[$root_index]}"
	ACTIVE_TARGET="$(target_for_module "$ACTIVE_MODULE")"
	local expected
	expected="./local-toolkit/$ACTIVE_TARGET"
	if [ "$ACTIVE_REPLACE" != "$expected" ]; then
		echo "error: replacement for $ACTIVE_MODULE must be exactly $expected (got '$ACTIVE_REPLACE')" >&2
		return 1
	fi

	local -a nested=()
	for i in "${!olds[@]}"; do
		[ "$i" -ne "$root_index" ] || continue
		old="${olds[$i]}"
		new="${news[$i]}"
		if ! target_allows_nested "$ACTIVE_TARGET" || [[ "$old" != "$ACTIVE_MODULE"/* ]]; then
			echo "error: exactly one local override is permitted; unapproved extra replacement for $old (active replacements: $ACTIVE_COUNT)" >&2
			return 1
		fi
		expected="./local-toolkit/$ACTIVE_TARGET${old#"$ACTIVE_MODULE"}"
		if [ "$new" != "$expected" ]; then
			echo "error: replacement for $old must be exactly $expected (got '$new')" >&2
			return 1
		fi
		nested+=("$old")
	done
	ACTIVE_NESTED="$(join_sorted "${nested[@]}")"
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
	local expected_nested="${4:-}"
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
		target|module|replace|source|revision|synced_at|nested) ;;
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
		nested) STATE_NESTED="$value" ;;
		esac
	done <"$STATE_FILE"

	# nested is optional so an override recorded before nested modules were
	# tracked stays readable; 'none' records an empty set.
	if [ -z "${seen[nested]+present}" ] || [ "$STATE_NESTED" = none ]; then
		STATE_NESTED=''
	fi

	local required
	for required in target module replace source revision synced_at; do
		if [[ -z "${seen[$required]+present}" ]]; then
			echo "error: missing ownership state key: $required" >&2
			return 1
		fi
	done

	if [ "$STATE_TARGET" != "$expected_target" ] ||
		[ "$STATE_MODULE" != "$expected_module" ] ||
		[ "$STATE_REPLACE" != "$expected_replace" ] ||
		[ "$STATE_NESTED" != "$expected_nested" ]; then
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
	validate_state "$ACTIVE_TARGET" "$ACTIVE_MODULE" "$ACTIVE_REPLACE" "$ACTIVE_NESTED"
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
	validate_state "$ACTIVE_TARGET" "$ACTIVE_MODULE" "$ACTIVE_REPLACE" "$ACTIVE_NESTED"
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
	local nested="${6:-}"
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
		printf 'nested=%s\n' "${nested:-none}"
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

tidy_modules() {
	local message="$1"
	echo "Running go mod tidy ..."
	if ! (cd "$REPO_ROOT" && go mod tidy); then
		echo "error: go mod tidy failed $message" >&2
		return 1
	fi
}

# discover_nested_modules walks the synced copy for go.mod files strictly below
# the target root and prints each module path whose module line matches its
# directory (the layout the replacement path is derived from).
discover_nested_modules() {
	local target="$1"
	local module="$2"
	local local_dir="$REPO_ROOT/local-toolkit/$target"
	local go_mod rel declared
	while IFS= read -r -d '' go_mod; do
		rel="${go_mod#"$local_dir"/}"
		rel="${rel%/go.mod}"
		declared="$(awk '$1 == "module" { print $2; exit }' "$go_mod")"
		if [ "$declared" != "$module/$rel" ]; then
			echo "note: skipping local-toolkit/$target/$rel/go.mod: declares '$declared', not '$module/$rel'" >&2
			continue
		fi
		printf '%s\n' "$declared"
	done < <(find "$local_dir" -mindepth 2 -type f -name go.mod -not -path '*/.git/*' -print0 | LC_ALL=C sort -z)
}

# replace_required_nested adds a replacement for each nested module that
# go.mod requires, the synced copy provides, and is not yet replaced.  It sets
# NESTED_ADDED to the number added.  Unrequired nested modules are left alone
# so no dangling replacement is written.
NESTED_ADDED=0
replace_required_nested() {
	local target="$1"
	local module="$2"
	local -A available=()
	local -a edits=()
	local m
	NESTED_ADDED=0
	while IFS= read -r m; do
		available["$m"]=1
	done < <(discover_nested_modules "$target" "$module")
	inspect_active_replacement || return 1
	local active=",$ACTIVE_NESTED,"
	while IFS= read -r m; do
		[ -n "$m" ] || continue
		[[ "$active" != *",$m,"* ]] || continue
		[[ -n "${available[$m]+present}" ]] || continue
		edits+=("-replace=$m=./local-toolkit/$target${m#"$module"}")
	done < <(required_nested "$module")
	if [ "${#edits[@]}" -gt 0 ]; then
		(cd "$REPO_ROOT" && go mod edit "${edits[@]}")
		NESTED_ADDED="${#edits[@]}"
	fi
}

# record_state re-reads go.mod, proves the replacements are exactly the
# selected target, and writes the ownership record for them.
record_state() {
	local target="$1"
	local module="$2"
	local source="$3"
	local revision="$4"
	local replace="./local-toolkit/$target"
	if ! inspect_active_replacement ||
		[ "$ACTIVE_TARGET" != "$target" ] ||
		[ "$ACTIVE_MODULE" != "$module" ] ||
		[ "$ACTIVE_REPLACE" != "$replace" ]; then
		echo "error: active replacement is not the exact selected target after sync" >&2
		return 1
	fi
	write_state "$target" "$module" "$replace" "$source" "$revision" "$ACTIVE_NESTED"
}

# check_nested_coverage fails loudly when go.mod requires a module nested
# below the target that is not replaced: the build would silently use its
# published tag instead of the synced source.
check_nested_coverage() {
	local module="$1"
	inspect_active_replacement || return 1
	local active=",$ACTIVE_NESTED,"
	local -a missing=()
	local m
	while IFS= read -r m; do
		[ -n "$m" ] || continue
		[[ "$active" == *",$m,"* ]] || missing+=("$m")
	done < <(required_nested "$module")
	if [ "${#missing[@]}" -gt 0 ]; then
		echo "error: go.mod requires nested toolkit modules that are NOT replaced; the build would use their published tags:" >&2
		for m in "${missing[@]}"; do
			echo "  $m (no matching go.mod at local-toolkit/$ACTIVE_TARGET${m#"$module"}/ in the synced source)" >&2
		done
		return 1
	fi
}

# print_replaced lists the active replacements, root first.
print_replaced() {
	local -a all=("$ACTIVE_MODULE")
	local m
	if [ -n "$ACTIVE_NESTED" ]; then
		local -a nested=()
		IFS=, read -r -a nested <<<"$ACTIVE_NESTED"
		all+=("${nested[@]}")
	fi
	echo "Active replace modules (${#all[@]}):"
	for m in "${all[@]}"; do
		echo "  $m => ./local-toolkit/$ACTIVE_TARGET${m#"$ACTIVE_MODULE"}"
	done
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

	if ! target_allows_nested "$target"; then
		# State is written only after sync and the exact JSON replacement check.
		record_state "$target" "$module" "$source" "$revision"
		return 0
	fi

	# State is recorded after every go.mod edit so a later failure (tidy, the
	# coverage check) still leaves an owned override that 'off' can remove.
	replace_required_nested "$target" "$module"
	record_state "$target" "$module" "$source" "$revision"
	tidy_modules "with the local replacements — check the toolkit edit's dependencies."
	# tidy can surface a new requirement (the local copy imports another
	# nested module); replace it and tidy again until the set is stable.
	local pass
	for pass in 1 2 3; do
		replace_required_nested "$target" "$module"
		[ "$NESTED_ADDED" -gt 0 ] || break
		record_state "$target" "$module" "$source" "$revision"
		tidy_modules "with the local replacements (pass $pass)."
	done
	check_nested_coverage "$module"
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
	if [ "$ACTIVE_COUNT" -ge 1 ] && [ "$ACTIVE_TARGET" = "$target" ]; then
		state_source="$STATE_SOURCE"
	fi
	local source
	source="$(resolve_src "$explicit" "$target" "$state_source")"
	sync_target "$target" "$module" "$source" yes
	host_build "against the synced local copy — check the toolkit edit for compile errors."
	inspect_active_replacement

	echo
	echo "Override ON. go.mod now replaces:"
	print_replaced
	cat <<EOF

Verify a built binary uses the local copies (each line shows '=> ./local-toolkit/...'):
  go build -o /tmp/rpg-api-server ./cmd/server && go version -m /tmp/rpg-api-server | grep rpg-toolkit

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
	inspect_active_replacement
	echo "Override refreshed for $ACTIVE_MODULE from $source."
	print_replaced
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
	validate_state "$ACTIVE_TARGET" "$ACTIVE_MODULE" "$ACTIVE_REPLACE" "$ACTIVE_NESTED"
	print_replaced
	echo "Override ON: $ACTIVE_MODULE => $ACTIVE_REPLACE"
	echo "Target: $ACTIVE_TARGET"
	echo "Source: $STATE_SOURCE"
	if [ -d "$REPO_ROOT/local-toolkit/$ACTIVE_TARGET" ]; then
		echo "local-toolkit/$ACTIVE_TARGET/ exists on disk."
	else
		echo "error: synced directory is missing: local-toolkit/$ACTIVE_TARGET" >&2
		return 1
	fi
	if target_allows_nested "$ACTIVE_TARGET" && ! check_nested_coverage "$ACTIVE_MODULE"; then
		echo "Run '$0 refresh' to replace them." >&2
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
	validate_state "$ACTIVE_TARGET" "$ACTIVE_MODULE" "$ACTIVE_REPLACE" "$ACTIVE_NESTED"
	local target_dir="$REPO_ROOT/local-toolkit/$ACTIVE_TARGET"
	local -a drops=("-dropreplace=$ACTIVE_MODULE")
	local m
	if [ -n "$ACTIVE_NESTED" ]; then
		local -a nested=()
		IFS=, read -r -a nested <<<"$ACTIVE_NESTED"
		for m in "${nested[@]}"; do
			drops+=("-dropreplace=$m")
		done
	fi
	print_replaced
	echo "Dropping ${#drops[@]} replace directive(s) ..."
	(cd "$REPO_ROOT" && go mod edit "${drops[@]}")
	echo "Removing $target_dir ..."
	rm -rf "$target_dir"
	rm -f "$STATE_FILE"
	# Only empty structural parents are removed; an unrelated sibling survives.
	rmdir "$(dirname "$target_dir")" 2>/dev/null || true
	rmdir "$REPO_ROOT/local-toolkit" 2>/dev/null || true
	if target_allows_nested "$ACTIVE_TARGET"; then
		# 'on' tidied go.sum against the local copies; restore published sums.
		tidy_modules "after removing the override — check go.mod/go.sum."
	fi
	host_build "after removing the override — check go.mod/go.sum."
	echo "Override OFF. ${#drops[@]} module(s) are back on their published pins."
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
