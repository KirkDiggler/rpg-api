# Run — 2026-11-26 — item story pins: an item's author can say it is unique

Slice [rpg-project#1183](https://github.com/KirkDiggler/rpg-project/issues/1183) of story [rpg-project#850](https://github.com/KirkDiggler/rpg-project/issues/850). Base: `rpg-api` `dev` at [`94db68d0`](https://github.com/KirkDiggler/rpg-api/commit/94db68d0).

## Shipped

- Pins: rulebook v0.236.0, encounter v0.261.0, session v0.215.0; resolver stays v0.82.0; `go.mod` moves only those three requirement lines — [https://github.com/KirkDiggler/rpg-project/issues/1183](https://github.com/KirkDiggler/rpg-project/issues/1183)
- `TestPutTakesAnItemTheAuthorSaysIsUniqueAndRefusesAWrongValue`: `unique: true` is accepted on validate-only and on save; `unique: sometimes` gives one error at `customItems.lieutenants-cloak.unique` containing "true or false" and stores nothing — [https://github.com/KirkDiggler/rpg-project/issues/1183](https://github.com/KirkDiggler/rpg-project/issues/1183)

## Decided

- none: every choice was the issue's — [https://github.com/KirkDiggler/rpg-project/issues/1183](https://github.com/KirkDiggler/rpg-project/issues/1183)

## Found

- F1 to F3 and F6 held; the newest tags were exactly the pins (rulebook v0.236.0, encounter v0.261.0, session v0.215.0) — [rests-on comment](https://github.com/KirkDiggler/rpg-project/issues/1183#issuecomment-7183332327)
- F5 printed nothing: no test in `internal` pins the old read-back sentence or the old word list — [rests-on comment](https://github.com/KirkDiggler/rpg-project/issues/1183#issuecomment-7183332327)
- F4 held: with the pins, `internal/notedwords` (`TestLastWeeksCastleCompilesAsItDid`) and `TestCastleCopiesAgree` pass with no edit to a test, helper or golden — [https://github.com/KirkDiggler/rpg-project/issues/1183](https://github.com/KirkDiggler/rpg-project/issues/1183)
- Baseline and the same tests after the pins: all green, none failed — [https://github.com/KirkDiggler/rpg-project/issues/1183](https://github.com/KirkDiggler/rpg-project/issues/1183)
- The new test failed on dev's pins with `"unique" is not a word of a custom item` and failed again with the encounter pin put back to v0.260.0; it passes on the new pins in 0.01s — [https://github.com/KirkDiggler/rpg-project/issues/1183](https://github.com/KirkDiggler/rpg-project/issues/1183)
- `go test ./internal/integration/...`, whole suite (`go test ./... -count=1`) and `make lint` (0 issues) green at the final head — [https://github.com/KirkDiggler/rpg-project/issues/1183](https://github.com/KirkDiggler/rpg-project/issues/1183)
- No existing test was changed — [https://github.com/KirkDiggler/rpg-project/issues/1183](https://github.com/KirkDiggler/rpg-project/issues/1183)

## Mistaken

- Restoring with `git checkout -- go.mod go.sum` before the pins were committed put dev's pins back, not the new ones; the three `go get` and `go mod tidy` were run again and the diff re-checked — [https://github.com/KirkDiggler/rpg-project/issues/1183](https://github.com/KirkDiggler/rpg-project/issues/1183)
