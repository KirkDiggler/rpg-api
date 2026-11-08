# Run — 2026-11-08 — the lobby handler suite compiles its fixtures once

Issue [rpg-api#1200](https://github.com/KirkDiggler/rpg-api/issues/1200). Base: `rpg-api` `dev` at [`85c8bcc4`](https://github.com/KirkDiggler/rpg-api/commit/85c8bcc4).

## Shipped

- `dungeonstest.ShippedStore` compiles the shipped content shelf once per test binary and shares it. `Shipped` builds on it, so every suite that asks for the shipped registry per test stops recompiling the shelf. — this PR

## Decided

- The cache lives in the shared test helper, not in one suite's `SetupTest`. Three packages build the shipped registry per test, and the helper is the one place that owns it. — this PR
- Sharing is faithful to production: the server compiles the shelf once at boot and serves every session from the same read-only `FileRegistry`. Each test still gets its own world store, projector, session Manager and miniredis. — this PR
- The cached store's projector runs over a miniredis that lives as long as the test binary. A `t.Cleanup`-scoped one would close after the first test that asked. — this PR
- The CI timeout was not raised, and no assertion changed. — this PR

## Found

- Every lobby handler test cost about one second locally, and about ten under `-race` with atomic coverage. `SetupTest` called `dungeonstest.Shipped`, which compiled and atlas-projected every file in `content/` per test. The castle growing is why the suite drifted from 348 s to 441 s on CI.
- `internal/orchestrators/lobby` paid the same cost: 318 s to 84 s under race.
- `internal/dungeons` takes about 270 s under race and is not changed by this fix. Its time is its own compile tests, and it is the next suite to watch against the limit.

— cross-team agent, on behalf of KirkDiggler
