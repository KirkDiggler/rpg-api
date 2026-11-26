# Run — 2026-11-26 — a character owns one of a custom item, API pins

Slice [rpg-project#1178](https://github.com/KirkDiggler/rpg-project/issues/1178) of story [rpg-project#850](https://github.com/KirkDiggler/rpg-project/issues/850). Base: `rpg-api` `dev` at [`ea4c99ae`](https://github.com/KirkDiggler/rpg-api/commit/ea4c99ae).

One line per entry. Every line ends with a link.

## Shipped

- `rulebooks/dnd5e/encounter` to v0.260.0 and `rulebooks/dnd5e/session` to v0.214.0 in `go.mod`/`go.sum`; `git diff origin/dev -- go.mod` moves only those two lines (encounter v0.259.0 to v0.260.0, session v0.213.0 to v0.214.0) — [issue](https://github.com/KirkDiggler/rpg-project/issues/1178).
- Whole suite at the final head: `go build ./...`, `go test ./... -count=1` and `make lint` (0 issues) pass; no existing test changed — [issue](https://github.com/KirkDiggler/rpg-project/issues/1178).

## Decided

- No judgment beyond the issue: the pins are exactly the two it names, and the newest tags at build time were encounter v0.260.0 and session v0.214.0 — [issue comment](https://github.com/KirkDiggler/rpg-project/issues/1178#issuecomment-7179180710).

## Found

- F1 to F3 and F5 held; F5: `git grep 'lieutenants-cloak-' origin/dev -- internal` printed nothing, so no test asserts a looted id — [issue comment](https://github.com/KirkDiggler/rpg-project/issues/1178#issuecomment-7179180710).
- Pinned on the older dev, `TestLastWeeksCastleCompilesAsItDid` failed because every wall compiled `"Sections": null`; the wall-sections change (rpg-api#1268) brought the comparison helper `historicalShape`, and after Amendment 1 merged `dev` in, both castle guards pass with no edit — [gap](https://github.com/KirkDiggler/rpg-project/issues/1178#issuecomment-7179225496).

## Mistaken

- Pinned before the wall-sections API change was on `dev`, which blocked the slice until Amendment 1 — [gap](https://github.com/KirkDiggler/rpg-project/issues/1178#issuecomment-7179225496).
