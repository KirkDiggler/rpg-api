# Run — 2026-11-08 — the lobby handler suite compiles its fixtures once

Issue [rpg-api#1200](https://github.com/KirkDiggler/rpg-api/issues/1200). Cut from `rpg-api` `dev` at [`aff1c72`](https://github.com/KirkDiggler/rpg-api/commit/aff1c72), merged up to [`5711f56f`](https://github.com/KirkDiggler/rpg-api/commit/5711f56f).

## Shipped

- `dungeonstest.ShippedStore` compiles the shipped content shelf once per test binary and shares it. `Shipped` builds on it, so every suite that asks for the shipped registry per test stops recompiling the shelf. — this PR

## Decided

- The cache lives in the shared test helper, not in one suite's `SetupTest`. Three packages build the shipped registry per test, and the helper is the one place that owns it. — this PR
- The cached shelf is staged into its own temp directory and removed once compiled, because the registry reads its directory only during construction. Since #1206 the shelf is `content/` plus the retired walk sites, the same set `FixtureContentDir` stages. — this PR
- Sharing is faithful to production: the server compiles the shelf once at boot and serves every session from the same read-only `FileRegistry`. Each test still gets its own world store, projector, session Manager and miniredis. — this PR
- The cached store's projector runs over a miniredis that lives as long as the test binary. A `t.Cleanup`-scoped one would close after the first test that asked. — this PR
- The CI timeout was not raised, and no assertion changed. — this PR

## Measured

Lobby suites, local, same machine, CI's flags:

```
go test -count=1 -race -covermode=atomic -coverprofile=c.out ./internal/handlers/dnd5e/lobby/v1alpha1/ ./internal/orchestrators/lobby/
```

| package | before | after |
|---|---|---|
| handlers/dnd5e/lobby/v1alpha1 | 349 s | 9.8 s |
| orchestrators/lobby | 318 s | 84 s |

Without `-race` (`go test -count=1 ./internal/handlers/dnd5e/lobby/v1alpha1/`) the handler suite went from 47 s to 2.4 s.

## Found

- Every lobby handler test cost about one second locally, and about ten under `-race` with atomic coverage. `SetupTest` called `dungeonstest.Shipped`, which compiled and atlas-projected every file in `content/` per test. The castle growing is why the suite drifted from 348 s to 441 s on CI.
- `internal/orchestrators/lobby` paid the same cost: 318 s to 84 s under race.
- `internal/dungeons` takes 215 to 277 s under race and is barely changed by this fix. Its time is its own compile tests, and it is the next suite to watch against the limit.

— cross-team agent, on behalf of KirkDiggler
