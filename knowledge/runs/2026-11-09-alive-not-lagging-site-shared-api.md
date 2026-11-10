# Run — 2026-11-09 — alive, not lagging: the site is shared by hash and stored once (api)

Api leg of slice S5 of story [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879) (card [#914](https://github.com/KirkDiggler/rpg-project/issues/914)): the compiled site is shared by content hash, the checkpoint names it and never carries the field, and the api boots its sites. Toolkit legs rpg-toolkit#2345 (encounter v0.188.0), #2348 (resolution v0.75.0), #2349 (session v0.175.0). PR rpg-api#1216.

Measured before at `rpg-api` `dev` [`2253b33e`](https://github.com/KirkDiggler/rpg-api/commit/2253b33e) on session v0.174.0 / encounter v0.187.x, and after on the PR branch at the tagged pins. Keep `bench-keep-garrison` (10k hexes, 99 garrison, 20 patrols), N=20 per path, Redis on a private loopback port flushed between paths, CPU AMD Ryzen 9 3950X. Command, once per path and per tree:

```
redis-cli -p <port> flushall
RPG_MEASURE=1 RPG_MEASURE_N=20 RPG_MEASURE_PATH=<1|4|8> RPG_MEASURE_REDIS=127.0.0.1:<port> \
  go test ./internal/integration/session/ -run 'TestMeasure_AnswerAndFirstBeat$' -count=1 -v -timeout 60m
```

## Shipped

- One site provider per process (`sessionorch.NewSites` over the toolkit's bounded cache, `RPG_SITE_CACHE_ENTRIES`, default 16), injected through `Config.Sites`; launch, reload and the dungeon map preview share one compiled site per hash.
- The atlas projection compiles the shipped shelf at boot; the warm-up names each key's site.
- The Redis encounter repository carries `sdk.SiteRepository`: `session-site:v1alpha1:<hash>`, no TTL, EXISTS then SETNX. A cold process loads through the piece.
- `session.ErrUnknownSite` maps to `FAILED_PRECONDITION`, checked ahead of `ErrInvalidWorld`.
- The measure harness adds the stored-piece, cold-site and warm-vs-cold Move rows.

## Decided

- The site piece has no TTL, because it is content, not session state: a checkpoint's TTL is refreshed by every save while the piece is written once at launch, so any fixed TTL on the piece could expire under a game still being played. Collecting pieces no checkpoint names is a follow-up · `internal/orchestrators/session/redis_sites.go`
- The repository returns `sdk.ErrNotFound` on a missing piece, not `session.ErrUnknownSite`, because that is the `SiteRepository` contract in session v0.175.0: the SDK turns a missing piece into `ErrUnknownSite` after its own provider misses, and the api maps that to `FAILED_PRECONDITION`. A corrupt piece is a distinct error, never "not found", so the SDK cannot fall through to compiling from nothing.
- The in-process cache is bounded at 16 sites by default (`RPG_SITE_CACHE_ENTRIES`), because a site is about 42% of an encounter's heap (rpg-toolkit#2345) and the dev shelf ships three sites; the bound is an operator knob, not a tuned number.
- A pre-site checkpoint is refused, not migrated, because a migration would guess a start cell for a field the checkpoint never named, and pre-playtest every live session can be relaunched.
- PutSite is EXISTS then SETNX rather than a plain SET, because the piece is content-addressed and immutable: a repeat launch on the same content neither encodes nor ships 507 KB, and two launches racing on new content write identical bytes once.

## The table

| row | path 1 before → after | path 4 | path 8 |
|---|---|---|---|
| Move whole (answer) | 146 → 93 ms | 595 → 210 ms | 851 → 350 ms |
| read load whole (Manager.Where) | 66.6 → 38.1 ms | 80.6 → 51.9 ms | 96.6 → 76.8 ms |
| 3 site build, warm provider | 45.2 → 28.1 ms | 47.2 → (shared) | 54.0 → 41.1 ms |
| 3p site piece GetSite (GET + decode, 507 KB) | — → 8.7 ms | | — → 7.9 ms |
| 3c cold site build (fresh provider + piece) | — → 49.4 ms | | — → 63.9 ms |
| Move whole, cold provider (fresh process, first verb) | — → 162 ms | | — → 971 ms |
| Move whole, warm provider (same process, next verb) | — → 131 ms | | — → 825 ms |
| encounter blob bytes | 1,876,612 → 1,378,438 | 3,557,333 → 3,059,159 | |
| 2 decode (JSON → EncounterData) | 16.0 → 8.6 ms | 22.7 → 15.7 ms | |
| 5b encode (JSON) | 15.4 → 8.9 ms | 25.8 → 17.6 ms | |

## Found

- The checkpoint loses the field: 498 KB per blob, and decode and encode halve with it.
- A cold process pays the piece fetch plus the compile once per hash (50 to 64 ms); every later verb on that site is warm.
- "Site build, warm provider" is still 28 to 41 ms per verb: that is `LoadEncounter` attaching the instance to the site (perception, members, behaviors), not a compile. It is the next per-verb cost (slice S6 territory).
- Path 1 Move whole is under the 100 ms answer bar in the harness for the first time.

## Deploy note

A checkpoint saved before this head carries a field and no `site_hash`; the api refuses it as `INVALID_ARGUMENT`, named by field. Every session live on the dev env at deploy time has to be relaunched. Pre-playtest, no migration is written.

## Mistaken

- The first PR body described the run record as a command list instead of shipping this file; the review round caught it, and the corrupt-piece decode path had no test until the same round · rpg-api#1216
- The builder that wrote the leg went quiet after finishing its commit and never pushed it; the commit was recovered from its worktree about an hour later. Builders must push and open the PR before any after-measurement, not after.

## Verification

- `go build ./...`, `go vet` on the touched packages, `go mod verify`, `go mod tidy -diff`: clean.
- `go test -count=1 -race ./internal/orchestrators/session/ ./internal/handlers/dnd5e/sdkerr/ ./cmd/server/`: ok.
- `go test -count=1 ./internal/integration/session/`: ok (15.8 s).
- Measure runs above: paths 1, 4, 8, exit 0 each, before and after.

— cross-team agent, on behalf of KirkDiggler
