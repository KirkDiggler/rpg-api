# Run — 2026-11-24 — one way to pay for an ability S pins second wind at the door

processed: 2026-11-25T190427Z

Slice [rpg-project#1148](https://github.com/KirkDiggler/rpg-project/issues/1148) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`7dc38c59`](https://github.com/KirkDiggler/rpg-api/commit/7dc38c59).

## Shipped

- Pins moved as one set: rulebook `v0.226.0` to `v0.228.0`, resolution `v0.80.0` to `v0.81.0`, session `v0.209.0` to `v0.210.0`; encounter stays `v0.255.0`, wire pin unmoved — [rpg-project#1148](https://github.com/KirkDiggler/rpg-project/issues/1148).
- The go.mod diff is exactly those three requirement lines; go.sum moved with them — [rpg-project#1148](https://github.com/KirkDiggler/rpg-project/issues/1148).

## Decided

- Four test fixtures that gave a fighter a private Second Wind count now give it the shared pool, `second_wind` 1 of 1 reset on a short rest (the spent fighter at 0 of 1), because the toolkit moved the uses into the pool; allowed edit kind one — [rpg-project#1148](https://github.com/KirkDiggler/rpg-project/issues/1148).
- The monk view test deletes the fighter fixture's `second_wind` resource, because a monk has no such pool and the status view refuses it — [rpg-project#1148](https://github.com/KirkDiggler/rpg-project/issues/1148).

## Found

- F1 to F5 held: the dev pins were 0.226.0, 0.255.0, 0.80.0, 0.209.0; the session tag requires rulebook 0.228.0, resolution 0.81.0, encounter 0.255.0; `case classes.Fighter:` is one line at draft.go:2023; `optionalResourceKey` is one line; the castle tests exist — [comment](https://github.com/KirkDiggler/rpg-project/issues/1148#issuecomment-7122539406).
- After the pins alone, two packages failed to compile (`features.SecondWindData` has no `Uses` or `MaxUses`: handler_test.go, view_test.go) and one acceptance test failed at launch ("owner does not carry second_wind"); no refusal-text or roll-source test failed — [rpg-project#1148](https://github.com/KirkDiggler/rpg-project/issues/1148).
- Lobby start_encounter_session_stack_test compiled but pinned the private count; it moved to the pool assertion — [rpg-project#1148](https://github.com/KirkDiggler/rpg-project/issues/1148).
- Whole suite `go test ./... -count=1` passes at the final head; `golangci-lint run` reports 0 issues — [rpg-project#1148](https://github.com/KirkDiggler/rpg-project/issues/1148).

## Mistaken

- The shared fighter fixture gained the pool and the monk test that reuses it failed until it dropped it; caught by the package run — [rpg-project#1148](https://github.com/KirkDiggler/rpg-project/issues/1148).
