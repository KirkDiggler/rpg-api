# Run — 2026-11-08 — ship only the castle, the keep and the bench keeps

Issue [rpg-api#1205](https://github.com/KirkDiggler/rpg-api/issues/1205). Ruling (KirkDiggler, 2026-11-08): "We only need the keep and mad king castle. We should do a better job cleaning up the special case reference environments; after they are used to verify, they do not serve a purpose. Ideally they would live in their worktree only."

Built on `rpg-api` `dev` [`3abf710`](https://github.com/KirkDiggler/rpg-api/commit/3abf710), branch `chore/content-ship-less`.

## Shipped

- `content/` holds `the-mad-king.yaml`, `reference-keep.yaml` and `bench/` (three bench keeps). Eleven walk sites moved to `internal/dungeons/testdata/sites/`; the image copies only `content/`, so they never ship.
- `dungeons.DefaultKey` is `the-mad-king`. The tomb was the default; it is retired with the other sites. Nothing in rpg-dnd5e-web or rpg-api-protos names the default at runtime: the web's authoring probe is `GetDungeon("")`.
- `dungeonstest.FixtureContentDir`: a temp shelf of `content/` plus the sites. `Shipped`, `ShippedStore` and `Scratch` load it, so suites that play a retired site still find it by key.
- `scripts/verify-content-allowlist.sh`: fails when `content/` TRACKS a file outside the allowlist. Wired into `.github/workflows/test.yml` and `scripts/ci-checks.sh`.

## Decided

- The allowlist reads `git ls-files`, not the directory. A walk site sits untracked in its slice worktree's `content/` (the walk skill's rule); a filesystem check would fail every local run on a walk env, a tracked-file check fails only the PR that commits one.
- No test was deleted. Every test that opened a retired file reads it from `testdata/sites/` now; the tomb-specific lobby tests name `reference-tomb`, and the registry mechanics suite writes the small tomb under the default key rather than compiling the castle per test.
- The TEMPORARY demo vendor (rpg-api#903 Phase 1) was gated on `dungeons.DefaultKey`. Moving the default would have put the merchant in every castle launch. It is now gated on its own `reference-tomb` key. The tomb no longer ships, so in production the gate is dead by design until Phase 2 removes the vendor. Review on PR #1206 caught this.
- The allowlist script captures `git ls-files` in a plain assignment, so a git failure stops it under `set -e` (exit 128 on a corrupt index) instead of reading as empty and green.
- The default is now a single-room (version 5) file, so `ListDungeons` lists it; the tomb, a version 2 file, was not listed. The list tests assert both.
- The atlas pin covers `content/` and the sites, so the retired sites' captured atlases stay checked.

## Commands and proof

```
go test ./...                          # 41 packages ok (again after the review fix)
golangci-lint run ./...                # 0 issues
./scripts/ci-checks.test.sh            # ci-check safety contract tests: PASS
./scripts/verify-content-allowlist.sh  # on the branch
Verifying content/ holds only the shipped allowlist...
content/ allowlist OK
```

Red, then cleaned up:

```
$ printf ... > content/x-walk.yaml   # untracked
$ ./scripts/verify-content-allowlist.sh
Verifying content/ holds only the shipped allowlist...
content/ allowlist OK
exit=0

$ git add content/x-walk.yaml
$ ./scripts/verify-content-allowlist.sh
Verifying content/ holds only the shipped allowlist...
error: content/x-walk.yaml is not on the content/ allowlist
content/ ships only the-mad-king.yaml, reference-keep.yaml and bench/*.
a walk site lives in the slice worktree's content/ untracked and is served from RPG_API_PATH; it never ships (rpg-project .agents/skills/walk)
exit=1
```

## Follow-ups

- `rpg-deployment/docker-compose.prod.yml` still sets `RPG_DUNGEON_KEY=reference-tomb`. rpg-api no longer reads that variable, so it is a no-op; it can go in the next deployment PR.
- A persistent content directory that already holds the retired files keeps serving them: the seed never deletes. The issue's own route applies: clear them by hand, or after rpg-api#1204 lands.
