# Run — 2026-11-10 — the resident copy behind the store's fence

Base: `rpg-api` `dev` at c119a0df. Story: [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879), card [rpg-project#927](https://github.com/KirkDiggler/rpg-project/issues/927), ruling R6, S6 PR 3 of 3. Upstream: encounter [rpg-toolkit#2367](https://github.com/KirkDiggler/rpg-toolkit/pull/2367) (Rebind) and session [rpg-toolkit#2368](https://github.com/KirkDiggler/rpg-toolkit/pull/2368) (Residency, CheckpointVersionReader, ResidencyObserver), developed against their pushed commits by pseudo-version and pinned at the tags: session v0.180.0 (toolkit main f417da27), encounter v0.193.0 (e2733aa2).

## Shipped

- The Redis encounter repository carries `sdk.CheckpointVersionReader`. `GetCheckpointVersion` is one GET of `session-enc-ver:v1alpha1:<id>` and nothing else. An absent key answers 0, which no kept copy holds. An unreadable value fails closed with `sdk.ErrBadRepository`.
- A checkpoint version never repeats for an encounter id. A new key `session-enc-ver-high:v1alpha1`, one for the store and never expiring, holds the highest version ever issued. Every save raises it. A checkpoint's first save starts one above it, and later saves add one as before.
- One residency per process: `sessionorch.NewResidency` wraps the session's `MemoryResidency` at the host's bounds and is injected through `Config.Residency`, as `NewSites` is. The server reads `RPG_RESIDENT_ENTRIES` (default 64, `-1` keeps none) and `RPG_RESIDENT_MAX_MB` (default 512) and logs the bounds at boot.
- The residency is also the session's `ResidencyObserver`. It counts hits and misses by reason (`Stats`) and logs each miss with its reason at debug level. No metrics system is introduced.
- A real-Redis acceptance suite counts what each verb reads of the encounter. A warm verb reads only the version key. Another process's save forces a reload. A stale save drops the copy. A process keeping none loads every verb.
- The measure harness reads `RPG_RESIDENT_ENTRIES`. It reports resident hits, misses and estimated MiB per run. The cold-reload table gains the version GET, Rebind, the warm open floor, and paired whole Moves with the copy warm and after a forced miss.

## Decided

- The orchestrator builds its own residency when none is injected, as it does for sites. Two orchestrators in one test process never share copies by accident, and every existing acceptance test now runs with the copy on.
- `RPG_RESIDENT_ENTRIES=0` is refused rather than read as the default. A knob set to zero most likely means "keep none", and guessing either way is worse than stopping the boot.
- The forced-miss row moves the store's version with an INCR between paired Moves, as another process's save would. The miss then costs exactly one GET plus one load. Keeping none is a separate run with the knob, and it pays the load without the GET.
- The never-repeat source is a non-expiring high-water counter, not Redis `TIME`. It is exact and monotonic with no clock in the argument. It costs one GET and one SET of one tiny key per save, and it leaves one key for the whole store rather than one per id. Within a checkpoint versions still count saves, so a fresh store still launches at 1. A per-id epoch key would leak one key per session ever launched. `TIME` would depend on the wall clock never stepping back. The one case it does not cover is a store flushed whole, high-water included. A process holding copies across a flush must restart, and no production path flushes.
- The version reader does not check for the head beside the version. Head and version share one TTL, and since a version never repeats, a copy can only match the version its own save wrote.

## Found

- Launch keeps its first board resident, so the first verb after a launch is already warm.
- The cold-reload harness's "warm provider" Move now also runs on the copy the first Move kept, so that row is labelled "copy kept".

## Mistaken

- The first warm-verb test asserted the first verb after Launch loads. Launch keeps, so the test now proves the contrast with a second process that holds no copy.

## Verification

- `go test -race -count=1` of the session integration, orchestrator and server packages: green.
- Contract: launch, save four times, delete every checkpoint key, relaunch and save four times, then let the TTL expire and do it again. No version repeats across the three lives, and another id's launch starts above all of them. Seeding the first save at 1 again fails it.
- Mutant: a version reader that answers one more than the store fails three of the four residency acceptance tests.
- `make ci-check` at the tag pins: all CI checks passed.
- Measure: `RPG_MEASURE=1 RPG_MEASURE_REDIS=127.0.0.1:6547 RPG_MEASURE_MODES=whole RPG_MEASURE_PATH=<1|8> go test ./internal/integration/session/ -run 'TestMeasure_AnswerAndFirstBeat$' -count=1 -v` on a private Redis. Before on dev c119a0df, after at this head. The table is in the PR body.

## Deploy

No stored key changes shape. One key is new: `session-enc-ver-high:v1alpha1`, created by the first save after deploy. Checkpoints already stored keep their versions and keep counting. A restart simply starts cold: the first verb on each session loads, as every verb did before.
