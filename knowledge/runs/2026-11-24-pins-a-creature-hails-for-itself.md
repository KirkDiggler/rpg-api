# Run — 2026-11-24 — a creature hails for itself S5 api pins

Slice [rpg-project#1142](https://github.com/KirkDiggler/rpg-project/issues/1142) of story [rpg-project#1139](https://github.com/KirkDiggler/rpg-project/issues/1139). Base: `rpg-api` `dev` at [`6073114d`](https://github.com/KirkDiggler/rpg-api/commit/6073114d).

## Shipped

- `rulebooks/dnd5e/encounter` v0.254.0 to v0.255.0 and `rulebooks/dnd5e/session` v0.208.6 to v0.209.0 as one set; no other requirement line moved — [rpg-project#1142](https://github.com/KirkDiggler/rpg-project/issues/1142).

## Decided

- none; the issue handed every choice — [rpg-project#1142](https://github.com/KirkDiggler/rpg-project/issues/1142).

## Found

- F1 printed dnd5e v0.226.0, encounter v0.254.0, resolution v0.80.0, session v0.208.6 at dev — [rpg-project#1142](https://github.com/KirkDiggler/rpg-project/issues/1142).
- F2 printed 1, `v0.255.0`, 1 for the tags — [rpg-project#1142](https://github.com/KirkDiggler/rpg-project/issues/1142).
- F3 `TestCastleCopiesAgree` found at line 34; the castle files did not change between the tags (empty diff stat); `internal/dungeons` and `internal/notedwords` pass with no edit — [rpg-project#1142](https://github.com/KirkDiggler/rpg-project/issues/1142).
- F4 `go test ./internal/integration/session/ -run 'Gate|ContentHash'` passes — [rpg-project#1142](https://github.com/KirkDiggler/rpg-project/issues/1142).
- F5 `GetMachineWords` found at line 31 of `get_machine_words.go`; no test needed an update — [rpg-project#1142](https://github.com/KirkDiggler/rpg-project/issues/1142).
- `go build ./...` passes with no converter change; `go test ./... -count=1` passes with no failures; `make lint` prints 0 issues. Existing tests changed: none — [rpg-project#1142](https://github.com/KirkDiggler/rpg-project/issues/1142).

## Mistaken

- none observed — [rpg-project#1142](https://github.com/KirkDiggler/rpg-project/issues/1142).
