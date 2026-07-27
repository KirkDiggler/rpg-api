#!/bin/bash
# toolkit-local-override.sh — iterate on rpg-toolkit/encounter changes in the
# local Docker loop without publish -> version tag -> `go get`.
#
# THE constraint this solves: the local rpg-api image is built with
#   docker build -t rpg-api:local <rpg-api-dir>
# — the build context is the rpg-api directory ALONE. A go.mod `replace`
# pointing outside that directory (e.g. `../../rpg-toolkit/encounter`)
# compiles fine on the host but fails inside Docker, because the toolkit
# source is not in the context. This script instead SYNCS the toolkit
# module's source INTO a gitignored directory inside this repo
# (local-toolkit/encounter) and points the replace at that relative path, so
# it stays inside the build context.
#
# Only github.com/KirkDiggler/rpg-toolkit/encounter is overridden — its own
# sibling-module requires (core, events, tools/spatial, ...) stay at whatever
# published versions its go.mod already names. Do not extend this script to
# sync the whole toolkit; add a second `-replace` line the same way if a
# second module genuinely needs it.
#
# Usage:
#   scripts/toolkit-local-override.sh on [--src <path-to-rpg-toolkit-checkout>]
#   scripts/toolkit-local-override.sh off
#   scripts/toolkit-local-override.sh status
#
# on:     rsyncs <src>/encounter/ -> local-toolkit/encounter/, then
#         `go mod edit -replace` to point go.mod at it. Re-run `on` after
#         every toolkit edit to re-sync (fast — no publish, no tag, no
#         `go get`); rebuild the image with Dockerfile.local-toolkit
#         afterward (see docs/how-to/local-toolkit-override.md).
# off:    `go mod edit -dropreplace` and deletes local-toolkit/ entirely,
#         restoring go.mod/go.sum to the published pin with no residue.
# status: reports whether the override is currently active.
#
# The `replace` this script adds/removes must NEVER be committed while
# active — it points at a directory (local-toolkit/) that doesn't exist on
# CI or any other machine, and CI's own `go build` would catch an escaped
# replace regardless (defense in depth, not the only line — see
# docs/how-to/local-toolkit-override.md).
set -euo pipefail

MODULE="github.com/KirkDiggler/rpg-toolkit/encounter"
REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
LOCAL_DIR="$REPO_ROOT/local-toolkit/encounter"

usage() {
	echo "Usage: $0 {on [--src <path>]|off|status}" >&2
	exit 1
}

# resolve_src finds an rpg-toolkit checkout to sync from: an explicit --src,
# then TOOLKIT_SRC_DIR, then a couple of common game-dev workspace layouts.
# Errors with a clear message rather than guessing further — silently
# syncing the wrong checkout would be worse than asking.
resolve_src() {
	local explicit="$1"
	local candidates=()
	if [ -n "$explicit" ]; then
		candidates+=("$explicit")
	fi
	if [ -n "${TOOLKIT_SRC_DIR:-}" ]; then
		candidates+=("$TOOLKIT_SRC_DIR")
	fi
	candidates+=(
		"$HOME/game-dev/rpg-toolkit"
		"$REPO_ROOT/../../rpg-toolkit"
		"$REPO_ROOT/../../../rpg-toolkit"
	)
	for c in "${candidates[@]}"; do
		if [ -f "$c/encounter/go.mod" ]; then
			echo "$c"
			return 0
		fi
	done
	echo "error: could not find an rpg-toolkit checkout with encounter/go.mod." >&2
	echo "Pass --src <path>, or set TOOLKIT_SRC_DIR, e.g.:" >&2
	echo "  TOOLKIT_SRC_DIR=/home/kirk/game-dev/rpg-toolkit $0 on" >&2
	return 1
}

cmd_on() {
	local explicit=""
	while [ $# -gt 0 ]; do
		case "$1" in
		--src)
			explicit="$2"
			shift 2
			;;
		*)
			usage
			;;
		esac
	done

	local src
	src="$(resolve_src "$explicit")"

	local module_line
	module_line="$(head -1 "$src/encounter/go.mod")"
	if [ "$module_line" != "module $MODULE" ]; then
		echo "error: $src/encounter/go.mod declares '$module_line', expected 'module $MODULE'" >&2
		exit 1
	fi

	echo "Syncing $src/encounter/ -> $LOCAL_DIR/ ..."
	mkdir -p "$LOCAL_DIR"
	rsync -a --delete --exclude='.git' "$src/encounter/" "$LOCAL_DIR/"

	echo "Pointing go.mod at the local copy ..."
	(cd "$REPO_ROOT" && go mod edit -replace "$MODULE=./local-toolkit/encounter")

	echo "Sanity-building on the host ..."
	if ! (cd "$REPO_ROOT" && go build ./...); then
		echo "error: host build failed against the synced local copy — check the toolkit edit for compile errors." >&2
		exit 1
	fi

	cat <<EOF

Override ON. go.mod now replaces $MODULE with ./local-toolkit/encounter.

Next steps:
  docker build -f Dockerfile.local-toolkit -t rpg-api:local $REPO_ROOT
  (cd /home/kirk/game-dev/rpg-deployment && \\
   docker compose -f docker-compose.local-dev.yml \\
                  -f docker-compose.local-api-src.yml \\
                  up -d rpg-api)

Re-run '$0 on' after every toolkit edit to re-sync.
Run '$0 off' when done — publish the toolkit change, bump the version tag,
then remove the override (never leave it active on a branch you push).
EOF
}

cmd_off() {
	echo "Dropping the replace directive ..."
	(cd "$REPO_ROOT" && go mod edit -dropreplace "$MODULE")

	echo "Removing $LOCAL_DIR ..."
	rm -rf "$REPO_ROOT/local-toolkit"

	echo "Sanity-building against the published module ..."
	if ! (cd "$REPO_ROOT" && go build ./...); then
		echo "error: build failed after removing the override — go.mod/go.sum may need attention (git diff go.mod go.sum)." >&2
		exit 1
	fi

	echo "Override OFF. go.mod/go.sum are back on the published pin, local-toolkit/ removed."
}

cmd_status() {
	local replace
	replace="$(cd "$REPO_ROOT" && go list -m -f '{{if .Replace}}{{.Replace.Path}} => {{.Replace.Dir}}{{end}}' "$MODULE" 2>/dev/null || true)"
	if [ -n "$replace" ]; then
		echo "Override ON: $MODULE => $replace"
	else
		echo "Override OFF: $MODULE resolves to its published version."
	fi
	if [ -d "$LOCAL_DIR" ]; then
		echo "local-toolkit/encounter/ exists on disk ($(find "$LOCAL_DIR" -name '*.go' | wc -l | tr -d ' ') .go files)."
	else
		echo "local-toolkit/encounter/ does not exist."
	fi
}

case "${1:-}" in
on)
	shift
	cmd_on "$@"
	;;
off)
	cmd_off
	;;
status)
	cmd_status
	;;
*)
	usage
	;;
esac
