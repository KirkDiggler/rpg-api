#!/bin/bash
# verify-release-pin.sh checks that this checkout resolves only published Go
# modules. It intentionally runs with GOWORK=off so CI and pre-PR checks cannot
# inherit a developer's workspace configuration.
set -euo pipefail

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
cd "$REPO_ROOT"

failures=0
fail() {
	echo "error: $*" >&2
	failures=$((failures + 1))
}

echo "Verifying published release pins (GOWORK=off)..."

# A workspace file is forbidden even though this check disables it: committing
# one would make ordinary developer commands resolve a non-release graph.
for workspace_file in go.work go.work.sum; do
	if [ -e "$workspace_file" ]; then
		fail "$workspace_file must not be present in a release-pinned checkout"
	fi
done

# local-toolkit is intentionally ignored for the short local override loop,
# but its presence means this tree is not eligible for a release/PR gate.
if [ -e local-toolkit ]; then
	fail "local-toolkit/ must be removed; run scripts/toolkit-local-override.sh off"
fi

replace_modules="$(GOWORK=off go mod edit -json | jq -r '.Replace[]? | .Old.Path')"
if [ -n "$replace_modules" ]; then
	fail "go.mod must not contain replace directives (found: $(tr '\n' ' ' <<<"$replace_modules"))"
fi

if [ "$failures" -ne 0 ]; then
	exit 1
fi

echo "Release pins verified: GOWORK=off, no workspace files, replaces, or local toolkit override."
