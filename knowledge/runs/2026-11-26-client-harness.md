# Run — 2026-11-26 — A game served for a client's tests

The interim for [rpg-api#1287](https://github.com/KirkDiggler/rpg-api/issues/1287), which gives the harness its lasting shape. Part of [rpg-project#1154](https://github.com/KirkDiggler/rpg-project/issues/1154).

## Shipped

- `internal/integration/session/zz_ue_spike_test.go`: `TestUESpikeServe` listens on `UE_SPIKE_ADDR`, launches the three runs a client's tests need, writes `UE_SPIKE_INFO` and serves until killed; skipped unless both variables are set — [rpg-api#1287](https://github.com/KirkDiggler/rpg-api/issues/1287).

## Decided

- Moved verbatim, spike names and all (`zz_`, `UE_SPIKE_*`): a pure move is reviewed in seconds and renames nothing a client's recipe names; the lasting shape is #1287's to give — [rpg-api#1287](https://github.com/KirkDiggler/rpg-api/issues/1287).
- A test that skips, not a command and not a build tag: `measure_answer_first_beat_test.go` in the same package already reads the environment — [the package](https://github.com/KirkDiggler/rpg-api/tree/dev/internal/integration/session).
- It left an idea's folder in rpg-project, where it had been the only copy in git; a documentation repository is not where a Go harness lives — [game-dev#140](https://github.com/KirkDiggler/game-dev/pull/140).

## Found

- `make test` with the file in place: 44 packages ok, 0 failed, 409 seconds; alone and unset, the test skips in 0.007 seconds — [this PR](https://github.com/KirkDiggler/rpg-api/pull/1289).

## Mistaken

- The record linked this PR by a number written before the PR existed, and was wrong until this commit; a number is written down after the thing exists — [this PR](https://github.com/KirkDiggler/rpg-api/pull/1289).
