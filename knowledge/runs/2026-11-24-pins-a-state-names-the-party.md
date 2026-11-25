# Run — 2026-11-24 — a state names the party S1 pins

processed: 2026-11-25T190427Z

Slice [rpg-project#1137](https://github.com/KirkDiggler/rpg-project/issues/1137) of story [rpg-project#1134](https://github.com/KirkDiggler/rpg-project/issues/1134). Base: `rpg-api` `dev` at [`23646fc2`](https://github.com/KirkDiggler/rpg-api/commit/23646fc2).

## Shipped

- `rulebooks/dnd5e/encounter` v0.252.0 to v0.254.0 and `rulebooks/dnd5e/session` v0.208.5 to v0.208.6, one commit, only those two `go.mod` requirement lines moved (plus `go.sum`) — [rpg-project#1137](https://github.com/KirkDiggler/rpg-project/issues/1137).

## Decided

- none beyond the issue's pins — [rpg-project#1137](https://github.com/KirkDiggler/rpg-project/issues/1137).

## Found

- F1 printed dnd5e v0.226.0, encounter v0.252.0, resolution v0.80.0, session v0.208.5 — [facts comment](https://github.com/KirkDiggler/rpg-project/issues/1137#issuecomment-7116063620).
- F2 `namedPartySeen` count 1 at encounter v0.254.0; session v0.208.6 requires encounter v0.254.0; `scene_move.go` is absent from the tag — [facts comment](https://github.com/KirkDiggler/rpg-project/issues/1137#issuecomment-7116063620).
- F3 `TestCastleCopiesAgree` exists; the castle yaml and compiled json have an empty diff stat between v0.252.0 and v0.254.0 — [facts comment](https://github.com/KirkDiggler/rpg-project/issues/1137#issuecomment-7116063620).
- F4 no fixed hash `2b6632fd` in the repository; no hash edit was needed — [facts comment](https://github.com/KirkDiggler/rpg-project/issues/1137#issuecomment-7116063620).
- F5 nothing in `content` or `internal` mentions `sceneMove` — [facts comment](https://github.com/KirkDiggler/rpg-project/issues/1137#issuecomment-7116063620).
- F6 `GetMachineWords` is at `get_machine_words.go:31`; no test stated the who-words list, so no existing test changed — [facts comment](https://github.com/KirkDiggler/rpg-project/issues/1137#issuecomment-7116063620).
- `go build ./...`, `go test ./internal/dungeons/ ./internal/notedwords/` and `go test ./internal/integration/session/ -run 'Gate|ContentHash'` pass with no edit — [rpg-project#1137](https://github.com/KirkDiggler/rpg-project/issues/1137).
- Whole suite `go test ./... -count=1` passes at the final head (first run lost the shared test redis container in the character integration package; the rerun was green); `make lint` reports 0 issues — [rpg-project#1137](https://github.com/KirkDiggler/rpg-project/issues/1137).

## Mistaken

- none observed — [rpg-project#1137](https://github.com/KirkDiggler/rpg-project/issues/1137).

— cross-team agent, on behalf of KirkDiggler
