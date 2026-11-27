# Run — 2026-11-27 — a stat block says what its creature can use

Slice [rpg-project#1230](https://github.com/KirkDiggler/rpg-project/issues/1230) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`1479356e`](https://github.com/KirkDiggler/rpg-api/commit/1479356e).

## Shipped

- The api pins rulebook `v0.243.0`, encounter `v0.263.0`, session `v0.218.0` as one set, resolver stays `v0.84.0`; only `go.mod` and `go.sum` move — [rpg-project#1230](https://github.com/KirkDiggler/rpg-project/issues/1230).
- A site with a stat block that says `lets` is accepted, and its read-back says it at `templates.gate-lieutenant.lets` — [rpg-project#1230](https://github.com/KirkDiggler/rpg-project/issues/1230).
- A stat block the rules cannot run is refused by a save at `templates.gate-lieutenant`, nothing stored — [amendment 1](https://github.com/KirkDiggler/rpg-project/issues/1230#issuecomment-7207747956).
- The item words served include an entry's own `uses` and its `max` (1 to 1000) — [rpg-project#1230](https://github.com/KirkDiggler/rpg-project/issues/1230).

## Decided

- No loop is added to `contentRefusals`, because a save assembles every stat block first and refuses it there; the content check's `Templates` refusals are not reached from this repository — [amendment 1](https://github.com/KirkDiggler/rpg-project/issues/1230#issuecomment-7207747956).
- The proof (rulebook pin alone back to `v0.240.0`) is not possible, because `go get` of the rulebook alone also downgrades session to `v0.217.1`; not forced — [amendment 1](https://github.com/KirkDiggler/rpg-project/issues/1230#issuecomment-7207747956).

## Found

- F1 to F4 and F7 held; F5 (the castle guards) passed with no edit after the pins — [rpg-project#1230](https://github.com/KirkDiggler/rpg-project/issues/1230).
- F6 printed nothing: no test pins a list of words that grew, so no existing test was edited — [rpg-project#1230](https://github.com/KirkDiggler/rpg-project/issues/1230).
- Refusal for `use: fireball`: `template "gate-lieutenant": invalid encounter data: stat block "gate-lieutenant": lets[0]: `use` is "fireball"; it is one of second wind` — [gap](https://github.com/KirkDiggler/rpg-project/issues/1230#issuecomment-7207740164).
- Refusal for `max: 0`: `... lets[0] `uses.max` is 0; it is a whole number from 1 to 1000` — [gap](https://github.com/KirkDiggler/rpg-project/issues/1230#issuecomment-7207740164).
- A `lets` that is not a list gives two errors: `line 24` unmarshal, then `templates.gate-lieutenant.lets: must be a list: what the creature can use, one entry each` — [amendment 1](https://github.com/KirkDiggler/rpg-project/issues/1230#issuecomment-7207747956).
- A save carries no read-back; only `validate_only` does — [amendment 1](https://github.com/KirkDiggler/rpg-project/issues/1230#issuecomment-7207747956).
- Suite: 44 packages ok at base and after the pins — [rpg-project#1230](https://github.com/KirkDiggler/rpg-project/issues/1230).

## Mistaken

- The brief expected the refusal at `templates.<id>.lets` through the content check; the compile refuses first at `templates.<id>`, found by the first test run and reported before any loop was written — [gap](https://github.com/KirkDiggler/rpg-project/issues/1230#issuecomment-7207740164).
