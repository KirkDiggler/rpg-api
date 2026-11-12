# Run — 2026-11-10 — the continuation commits at each pace boundary (api)

processed: 2026-11-12T165555Z

Base: `rpg-api` `dev` at cc470ea0 (session v0.178.0, encounter v0.192.0). Story: [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879), card [rpg-project#926](https://github.com/KirkDiggler/rpg-project/issues/926) (S13, ruling R7 option D), PR 2 of 2. Upstream: session [rpg-toolkit#2364](https://github.com/KirkDiggler/rpg-toolkit/pull/2364), developed against its pushed commit 04b3e1c6 by pseudo-version (measured there), then pinned at its tag v0.179.0 (main 6d4d6c5a) with encounter v0.192.0 and record v0.4.0, no replace, go.sum tidy.

## Shipped

- The session pin. A continued walk commits at each pace boundary, so the walker's listener hears a round's raise before the walk ends.
- The continuation runner logs "saved through its last pace commit" with `commits_landed` and `saved_through` (the id rides the attribute only) when a rest fails after a commit landed; its doc and panic log say nothing is saved after the last pace commit, if any. "Nothing saved" stays for a rest with no landed commit.
- The act broker's doc: delivery is one unit per commit; a continued walk is two to four.
- The R1 harness: "first beat after answer" p50 and p95, and "commits/verb".
- `TestAcceptance_AWalkCommitsAtItsPaceBoundary`: two fenced saves for a one-raise walk; the round's unit arrives while the walk still steps.

## Decided

- No functional change to the runner, the broker or the Lua: repeated fenced saves in one verb were already legal, and the broker already delivers per PublishActs.
- No client signal for a rest that failed mid-walk; rpg-api#1198 owns it.
- "Before the walk ends" is proved by the walker's sheet reads, which the rest makes before each step: the first unit arrives before the last read.

## Found

- The harness's first-beat column measured from the verb's start. The R1 bar is from the answer, so the new column measures from there (a beat that lands before the answer counts as 0).

- Path 1 misses R1's 250 ms bar after the answer (361 ms at p95), before and after S13. The due step is the walk's only step, so its round's raise and commit are the first beat. A round on one-cell Moves comes once in six Moves and writes 617 KB, including a 131 KB journal piece. The next lever there is the raise's own cost, not the commit's place.

## Mistaken

- None so far.

## Verification

- `go test -race -count=1 ./...`: green. `golangci-lint`: 0 issues on the touched packages.
- Mutant (local replace, the continuation walking through the boundary): the acceptance test fails, expected 2 saves, got 1.

## Measurements

Measured 2026-11-10 at about 01:10 UTC. The keep is `bench-keep-garrison`, mode S1 continues, with no reload row. Redis 7 ran on a private side port. The machine is a Ryzen 9 3950X at load average 4 to 6. **Before** is rpg-api dev cc470ea0 (session v0.178.0) plus this PR's harness columns. **After** is rpg-api 2313806b (session 04b3e1c6). Toolkit e92be7a1 changes only tests and docs after that commit.

| path, N | answer p50 | first beat after answer p50 / p95 | drained p50 / p95 | commits/verb | write/verb p50 |
|---|---|---|---|---|---|
| 1, 20 before | 86 ms | 0 / 369 ms | 88 / 407 ms | 1 | 24.5 KB (round Moves 617 KB) |
| 1, 20 after | 85 ms | 0 / 361 ms | 85 / 400 ms | 1 | 24.5 KB (round Moves 617 KB) |
| 4, 20 before | 110 ms | 98 / 144 ms | 203 / 232 ms | 1 | 470 KB |
| 4, 20 after | 121 ms | 99 / 110 ms | 218 / 256 ms | 1–2 | 476 KB |
| 8, 20 before | 122 ms | 175 / 285 ms | 312 / 382 ms | 1 | 483 KB |
| 8, 20 after | 130 ms | 107 / 116 ms | 355 / 426 ms | 2 | 505 KB |
| 8, 30 before | 129 ms | 186 / 302 ms | 338 / 413 ms | 1 | 483 KB |
| 8, 30 after | 131 ms | 108 / 118 ms | 364 / 455 ms | 2 | 508 KB |

- **The deciding row.** The first beat after the answer, at p95, is under 250 ms on paths 4 and 8, one-step answers included. On path 8 the moves that answered one step fell from 251–304 ms to 102–116 ms. **Path 1 misses the bar** at 361 ms p95, unchanged by S13. Only 3 of its 20 Moves continue, and in each the due step is the walk's only step. The first beat there is that step's raise plus its commit, with nothing after it to split off. A one-cell Move pays a round once in six Moves, and that round writes 617 KB, including a 131 KB journal piece.
- **Bytes against F27.** A one-raise Move on path 8 writes 505 KB, against 483 KB before (+4.5%). Every third Move on path 8 answers one step, and its rest pays a second round on its last cell. That Move makes two round commits of about 470 KB each, about 950 KB, against 619 KB before (+54%).
- **Drained.** Path 8 drains 43 ms later at p50, 312 ms before and 355 ms after: one extra commit. That cost lands after the first beat.
- **Lag.** No subscriber lagged at the production buffer.
