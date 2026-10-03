# Iterate on toolkit changes with pushed commits

Toolkit development uses commits pushed to `rpg-toolkit/origin` and real Go
pseudo-versions in the consuming module. Local stacks build the normal API image
from that dependency graph; they do not copy toolkit source into the image.

## Pin an in-flight provider

Work in separate toolkit and API worktrees. Commit the toolkit changes, then:

```bash
# Set these to the worktrees for this line of work.
TOOLKIT_WORKTREE=/path/to/rpg-toolkit/.worktrees/feature
API_WORKTREE=/path/to/rpg-api/.worktrees/feature

# The provider commit must be reachable from origin before a consumer pins it.
git -C "$TOOLKIT_WORKTREE" push -u origin HEAD
TOOLKIT_COMMIT=$(git -C "$TOOLKIT_WORKTREE" rev-parse HEAD)

# Example: the root D&D 5e module. Choose the actual module being changed.
GOWORK=off go -C "$API_WORKTREE" get "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e@$TOOLKIT_COMMIT"
GOWORK=off go -C "$API_WORKTREE" mod tidy
```

Go derives the pseudo-version and checksums. Do not hand-write a version string
or tag the provider branch. Inspect and commit the consumer's `go.mod`/`go.sum`;
the pin identifies the exact pushed source used by local builds and CI.

Each nested toolkit module is a separate dependency. For a cross-module change,
update the direct consumer module first, push that consumer commit, and continue
outward to rpg-api. Updating one module does not adopt changes to its siblings.
Keep referenced commits reachable; do not rewrite or delete a provider branch
while consumers still depend on its pseudo-version.

## Run the local stack

Use the named-stack launcher from the **game-dev root checkout**. Select the API
and web feature refs, or their worktree paths while editing. The selected API
source's `go.mod` is the toolkit selector.

Do not set `RPG_TOOLKIT_PATH`, `RPG_TOOLKIT_REF` or `RPG_TOOLKIT_TARGET`, run
`toolkit-local-override.sh on/refresh`, add a local `replace`/`go.work`, or select
`Dockerfile.local-toolkit` for this workflow. Use the normal `Dockerfile`.

The launcher and manifest commands live in `game-dev/docs/dev-environments.md`.
For a path-backed API checkout, `restart` rebuilds its current source while
preserving the environment's data. A ref-backed checkout is a snapshot:
`restart` does not fetch newer commits; `up` resolves the ref again and can
reseed a disposable environment. Choose deliberately rather than resetting an
environment just to verify a pin.

Another toolkit edit means another commit/push and `go get` in the consumer,
not a source sync or override refresh. Update the API ref/path actually served
by the environment, not an unrelated checkout.

## Adopt released versions before merging consumers

After provider review and integration verification:

1. The operator merges the provider; toolkit CI publishes its module tag.
2. Resolve that actual release, then run `go get` for the corresponding version
   in the consumer. Do not guess a tag or use `@latest` to update unrelated work.
3. Commit the released pin and checksums, then complete the consumer's applicable
   checks before its merge. Continue inside-out for additional module layers.

Development pseudo-versions are valid committed dependencies, not local-path
exceptions. Consumer merge readiness requires the released provider pins for the
wave. Merging, tagging and deployment remain separate authorized actions.

## Keep proto generation separate

This workflow is for toolkit Go modules. Protobuf bindings remain CI-owned;
follow [the proto adoption guide](update-proto-dependency.md) for published
SDK revisions, not an ungenerated proto-source branch.

## Legacy override environments

`local-toolkit-override.md` is a redirect, not an alternative development recipe.
The helper and old Dockerfile remain legacy artifacts; their presence does not
make them the current workflow. If an existing environment has an override,
inspect its ownership and remove only that override before adopting a pushed
commit. Do not use a blanket reset or delete another developer's local sources.
