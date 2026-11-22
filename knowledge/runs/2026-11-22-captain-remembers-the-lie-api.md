# Run — 2026-11-22 — Machine story S-api the captain remembers the lie

Slice [rpg-project#1107](https://github.com/KirkDiggler/rpg-project/issues/1107) of story [rpg-project#989](https://github.com/KirkDiggler/rpg-project/issues/989). Base: `rpg-api` `dev` at [`ed4bfc65`](https://github.com/KirkDiggler/rpg-api/commit/ed4bfc65).

One line per entry. Every line ends with a link.

## Shipped

- Pins `encounter` v0.241.0 to v0.244.0 and `session` v0.207.0 to v0.208.0 in one commit; the served castle gains the three captain hunks (`the-way-in` requires, `back-to-gate` pass, `captain-policy` outcomes) — [rpg-project#1107](https://github.com/KirkDiggler/rpg-project/issues/1107).

## Decided

- none; every choice was given by the issue — [rpg-project#1107](https://github.com/KirkDiggler/rpg-project/issues/1107).

## Found

- F1 holds: dev pins dnd5e v0.226.0, encounter v0.241.0, resolution v0.80.0, session v0.207.0 — [Rests on comment](https://github.com/KirkDiggler/rpg-project/issues/1107#issuecomment-7072371979).
- F2 holds: encounter v0.244.0 castle has 4 `caught-lying`; session v0.208.0 launch.go has 1 `join.Outcomes = monster.placement.Outcomes` — [Rests on comment](https://github.com/KirkDiggler/rpg-project/issues/1107#issuecomment-7072371979).
- F3, F4, F5, F6 hold: `TestCastleCopiesAgree` at castle_copies_test.go:34; the three places at 2026, 2049, 2806; three toolkit hunks; 16 gate-family acceptance test files — [Rests on comment](https://github.com/KirkDiggler/rpg-project/issues/1107#issuecomment-7072371979).
- go.mod diff: only the encounter and session requirement lines moved — [rpg-project#1107](https://github.com/KirkDiggler/rpg-project/issues/1107).
- Expected failure of step 2 after the pins: `TestCastleCopiesAgree` fails with the diff `Requires: len=2 vs len=1` on `the-way-in` (the `not: noted: caught-lying` trigger missing), the same for `Pass` on `back-to-gate`, and the missing outcomes — [rpg-project#1107](https://github.com/KirkDiggler/rpg-project/issues/1107).
- After the copy: `go test ./internal/dungeons/` and `go test ./internal/integration/session/ -run Gate` pass, so the castle launches with the session that hands outcomes over — [rpg-project#1107](https://github.com/KirkDiggler/rpg-project/issues/1107).
- Whole suite `go test ./... -count=1 -timeout 15m` prints no failure; `make lint` prints 0 issues; `make release-pin-check tidy-check` pass — [rpg-project#1107](https://github.com/KirkDiggler/rpg-project/issues/1107).
- Existing tests changed: none — [rpg-project#1107](https://github.com/KirkDiggler/rpg-project/issues/1107).

## Mistaken

- none observed — [rpg-project#1107](https://github.com/KirkDiggler/rpg-project/issues/1107).
