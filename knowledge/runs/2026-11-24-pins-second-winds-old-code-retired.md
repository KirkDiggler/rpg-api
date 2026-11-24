# Run — 2026-11-24 — one way to pay for an ability, pins: second wind's old code retired

Slice [rpg-project#1153](https://github.com/KirkDiggler/rpg-project/issues/1153) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`2c57e373`](https://github.com/KirkDiggler/rpg-api/commit/2c57e37334a8f6238921d5cdbb07ed3d551f5cf0).

## Shipped

- `rulebooks/dnd5e` v0.229.0, `resolution` v0.81.1, `session` v0.210.1 in one commit; `encounter` stays v0.255.0 — [rpg-project#1153](https://github.com/KirkDiggler/rpg-project/issues/1153).

## Decided

- none; the issue gave every choice — [rpg-project#1153](https://github.com/KirkDiggler/rpg-project/issues/1153).

## Found

- F1 holds: dev pinned dnd5e v0.228.0, encounter v0.255.0, resolution v0.81.0, session v0.210.0 — [rpg-project#1153 first comment](https://github.com/KirkDiggler/rpg-project/issues/1153#issuecomment-7123081014).
- F2 holds: session v0.210.1 requires dnd5e v0.229.0, encounter v0.255.0, resolution v0.81.1 — [rpg-project#1153 first comment](https://github.com/KirkDiggler/rpg-project/issues/1153#issuecomment-7123081014).
- F3 holds: `ErrUsedFromItsDescription` appears 2 times in `features/described.go` at dnd5e v0.229.0 — [rpg-project#1153 first comment](https://github.com/KirkDiggler/rpg-project/issues/1153#issuecomment-7123081014).
- F4 holds: no `ActivateAbility(` call under `internal`, test or non-test, at dev — [rpg-project#1153 first comment](https://github.com/KirkDiggler/rpg-project/issues/1153#issuecomment-7123081014).
- F5 holds: `TestCastleCopiesAgree` and `TestLastWeeksCastleCompilesAsItDid` pass with no edit — [rpg-project#1153](https://github.com/KirkDiggler/rpg-project/issues/1153).
- `go.mod` diff: only the three requirement lines move; build passes; handler and orchestrator tests, the Activation|Gate|ContentHash integration run and the whole `go test ./... -count=1` pass; `make lint` reports 0 issues — [rpg-project#1153](https://github.com/KirkDiggler/rpg-project/issues/1153).
- Existing tests changed: none — [rpg-project#1153](https://github.com/KirkDiggler/rpg-project/issues/1153).

## Mistaken

- none observed — [rpg-project#1153](https://github.com/KirkDiggler/rpg-project/issues/1153).
