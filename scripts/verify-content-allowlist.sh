#!/bin/bash
# verify-content-allowlist.sh fails when content/ tracks a file the server
# should not ship (rpg-api#1205). content/ ships what is played or
# benchmarked: the castle, the keep, the bench keeps, and the default dungeon
# the registry requires. A slice's walk site is not on the list: it lives in
# the slice worktree's content/ UNTRACKED and is served from RPG_API_PATH, so
# this check reads tracked files only (git ls-files) and an untracked walk
# site never trips it.
set -euo pipefail

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
cd "$REPO_ROOT"

# The allowlist. Keep internal/dungeons.DefaultKey's file on it: the registry
# refuses to boot without the default dungeon.
allowed() {
	case "$1" in
	content/the-mad-king.yaml) return 0 ;;
	content/reference-keep.yaml) return 0 ;;
	content/README.md) return 0 ;;
	content/bench/*/*) return 1 ;;
	content/bench/*) return 0 ;;
	*) return 1 ;;
	esac
}

echo "Verifying content/ holds only the shipped allowlist..."

# A plain assignment, not a process substitution: if git ls-files fails,
# set -e stops here instead of the loop reading nothing and passing.
tracked="$(git ls-files -- content)"

failures=0
while IFS= read -r path; do
	[ -n "$path" ] || continue
	if ! allowed "$path"; then
		echo "error: $path is not on the content/ allowlist" >&2
		failures=$((failures + 1))
	fi
done <<<"$tracked"

if [ "$failures" -gt 0 ]; then
	cat >&2 <<'MSG'
content/ ships only the-mad-king.yaml, reference-keep.yaml and bench/*.
a walk site lives in the slice worktree's content/ untracked and is served from RPG_API_PATH; it never ships (rpg-project .agents/skills/walk)
MSG
	exit 1
fi

echo "content/ allowlist OK"
