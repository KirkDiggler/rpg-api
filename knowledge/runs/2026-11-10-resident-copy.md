# Run — 2026-11-10 — the resident copy behind the store's fence

Base: `rpg-api` `dev` at c119a0df. Story: [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879), card [rpg-project#927](https://github.com/KirkDiggler/rpg-project/issues/927), ruling R6, S6 PR 3 of 3. Upstream: encounter [rpg-toolkit#2367](https://github.com/KirkDiggler/rpg-toolkit/pull/2367) (Rebind) and session [rpg-toolkit#2368](https://github.com/KirkDiggler/rpg-toolkit/pull/2368) (Residency, CheckpointVersionReader, ResidencyObserver), pinned at their pushed commits by pseudo-version (session b7543a2e, encounter c14f6bf6); re-pinned at the tags when they land.

## Shipped

- The Redis encounter repository carries `sdk.CheckpointVersionReader`. `GetCheckpointVersion` is one GET of `session-enc-ver:v1alpha1:<id>` and nothing else. An absent key answers 0, which no kept copy holds. An unreadable value fails closed with `sdk.ErrBadRepository`.
- One residency per process: `sessionorch.NewResidency` wraps the session's `MemoryResidency` at the host's bounds and is injected through `Config.Residency`, as `NewSites` is. The server reads `RPG_RESIDENT_ENTRIES` (default 64, `-1` keeps none) and `RPG_RESIDENT_MAX_MB` (default 512) and logs the bounds at boot.
- The residency is also the session's `ResidencyObserver`. It counts hits and misses by reason (`Stats`) and logs each miss with its reason at debug level. No metrics system is introduced.
- A real-Redis acceptance suite counts what each verb reads of the encounter. A warm verb reads only the version key. Another process's save forces a reload. A stale save drops the copy. A process keeping none loads every verb.
- The measure harness reads `RPG_RESIDENT_ENTRIES`. It reports resident hits, misses and estimated MiB per run. The cold-reload table gains the version GET, Rebind, the warm open floor, and paired whole Moves with the copy warm and after a forced miss.

## Decided

- The orchestrator builds its own residency when none is injected, as it does for sites. Two orchestrators in one test process never share copies by accident, and every existing acceptance test now runs with the copy on.
- `RPG_RESIDENT_ENTRIES=0` is refused rather than read as the default. A knob set to zero most likely means "keep none", and guessing either way is worse than stopping the boot.
- The forced-miss row moves the store's version with an INCR between paired Moves, as another process's save would. The miss then costs exactly one GET plus one load. Keeping none is a separate run with the knob, and it pays the load without the GET.
- The version reader does not check for the head beside the version. The head and the version share one TTL, and a copy can only match a version its own save wrote.

## Found

- Launch keeps its first board resident, so the first verb after a launch is already warm.
- The cold-reload harness's "warm provider" Move now also runs on the copy the first Move kept, so that row is labelled "copy kept".

## Mistaken

- The first warm-verb test asserted the first verb after Launch loads. Launch keeps, so the test now proves the contrast with a second process that holds no copy.

## Verification

- `go test -race -count=1` of the session integration, orchestrator and server packages: green.
- Mutant: a version reader that answers one more than the store fails three of the four residency acceptance tests.
- `make ci-check`: see the PR.
- Measure: `RPG_MEASURE=1 RPG_MEASURE_REDIS=127.0.0.1:6547 RPG_MEASURE_MODES=whole RPG_MEASURE_PATH=<1|8> go test ./internal/integration/session/ -run 'TestMeasure_AnswerAndFirstBeat$' -count=1 -v` on a private Redis. Before on dev c119a0df, after at this head. The table is in the PR body.

## Deploy

Nothing stored changes and no key changes. A restart simply starts cold: the first verb on each session loads, as every verb did before.
