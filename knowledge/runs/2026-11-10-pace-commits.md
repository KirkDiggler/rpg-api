# Run — 2026-11-10 — the continuation commits at each pace boundary (api)

Base: `rpg-api` `dev` at cc470ea0 (session v0.178.0, encounter v0.192.0). Story: [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879), card [rpg-project#926](https://github.com/KirkDiggler/rpg-project/issues/926) (S13, ruling R7 option D), PR 2 of 2. Upstream: session [rpg-toolkit#2364](https://github.com/KirkDiggler/rpg-toolkit/pull/2364), pinned at its pushed commit 04b3e1c6 by pseudo-version; re-pinned at its tag when it merges.

## Shipped

- The session pin. A continued walk commits at each pace boundary, so the walker's listener hears a round's raise before the walk ends.
- The continuation runner logs "saved through <record id>" with `commits_landed` and `saved_through` when a rest fails after a commit landed. "Nothing saved" stays for a rest with no landed commit.
- The act broker's doc: delivery is one unit per commit; a continued walk is two to four.
- The R1 harness: "first beat after answer" p50 and p95, and "commits/verb".
- `TestAcceptance_AWalkCommitsAtItsPaceBoundary`: two fenced saves for a one-raise walk; the round's unit arrives while the walk still steps.

## Decided

- No functional change to the runner, the broker or the Lua: repeated fenced saves in one verb were already legal, and the broker already delivers per PublishActs.
- No client signal for a rest that failed mid-walk; rpg-api#1198 owns it.
- "Before the walk ends" is proved by the walker's sheet reads, which the rest makes before each step: the first unit arrives before the last read.

## Found

- The harness's first-beat column measured from the verb's start. The R1 bar is from the answer, so the new column measures from there (a beat that lands before the answer counts as 0).

## Mistaken

- None so far.

## Verification

- `go test -race -count=1 ./...`: green. `golangci-lint`: 0 issues on the touched packages.
- Mutant (local replace, the continuation walking through the boundary): the acceptance test fails, expected 2 saves, got 1.
- Measurements: bench-keep-garrison, S1 continues, paths 1, 4, 8 at N=20 and 8 at N=30, before (dev cc470ea0) and after; added in a follow-up commit.
