# Run — 2026-11-26 — an NPC's turn is paid at the one gate, first pins

Slice [rpg-project#1200](https://github.com/KirkDiggler/rpg-project/issues/1200) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`9ef6fea3`](https://github.com/KirkDiggler/rpg-api/commit/9ef6fea3).

One line per entry. Every line ends with a link.

## Shipped

- `rulebooks/dnd5e` v0.237.0 to v0.239.0, `resolution` v0.82.0 to v0.83.0, `session` v0.215.0 to v0.216.0; encounter stays v0.261.0; no other requirement moved — [rpg-project#1200](https://github.com/KirkDiggler/rpg-project/issues/1200).

## Decided

- none; the pins are the issue's exact set and the newest tags at build time — [rests-on comment](https://github.com/KirkDiggler/rpg-project/issues/1200#issuecomment-7186848089).

## Found

- F1 to F3 hold: dev pins read as the issue says, the session tag's go.mod names rulebook v0.239.0, encounter v0.261.0, resolver v0.83.0, and nothing here reads `ReactionSpent` or `reaction_spent` (grep printed nothing) — [rests-on comment](https://github.com/KirkDiggler/rpg-project/issues/1200#issuecomment-7186848089).
- F4: `TestCastleCopiesAgree` and `TestLastWeeksCastleCompilesAsItDid` pass after the pins with no edit — [rests-on comment](https://github.com/KirkDiggler/rpg-project/issues/1200#issuecomment-7186848089).
- F5: tests that play a monster's turn are the skeleton turns in `story_events_test.go` (one driven turn), `driver_acceptance_test.go` (EndTurn across the skeleton and back, two sessions), `interrupt_acceptance_test.go` (a skeleton walks one path on its first turn, passes after) and `displacement_acceptance_test.go` (a pushed or fleeing skeleton); none plays the same monster acting on two rounds — [rests-on comment](https://github.com/KirkDiggler/rpg-project/issues/1200#issuecomment-7186848089).
- F6: `redis_repos_contract_test.go` stores a monster sheet that never acts (no `economy`) and `monsterfixture/goblin.go` marshals a fresh goblin for rehydration; no golden, seed or byte-for-byte sheet comparison exists — [rests-on comment](https://github.com/KirkDiggler/rpg-project/issues/1200#issuecomment-7186848089).
- Suite before: `go build ./...` ok, `go test ./... -count=1` 44 packages ok, no failures; after the pins: the same, 44 ok, no failures; `make lint` 0 issues — [rpg-project#1200](https://github.com/KirkDiggler/rpg-project/issues/1200).

## Mistaken

- none observed — [rpg-project#1200](https://github.com/KirkDiggler/rpg-project/issues/1200).
