# Run — 2026-11-16 — studio today S12 one-thing fold (api)

Slice [rpg-project#980](https://github.com/KirkDiggler/rpg-project/issues/980) of story [rpg-project#966](https://github.com/KirkDiggler/rpg-project/issues/966). Base: `rpg-api` `dev` at [`be41d848`](https://github.com/KirkDiggler/rpg-api/commit/be41d848).

## Shipped

- The check's `folded` is the engine's one-thing fold, `FoldedEach()`, in place of the whole-file `Folded()`; no protos change — [rpg-api#1249](https://github.com/KirkDiggler/rpg-api/pull/1249).
- `rulebooks/dnd5e/encounter` pinned v0.215.1 → v0.217.0, the only `go.mod` line that moved — [rpg-api#1249](https://github.com/KirkDiggler/rpg-api/pull/1249).

## Decided

- `session` and the umbrella `rulebooks/dnd5e` are not bumped: Go selects encounter v0.217.0 under them and the whole suite is green on it — [rpg-api#1249](https://github.com/KirkDiggler/rpg-api/pull/1249).
- The loudness assertion keys on "words reach everyone", because an answer's fact clause ("every member those words reach … learns") legitimately keeps the words "words reach" — [rpg-api#1249](https://github.com/KirkDiggler/rpg-api/pull/1249).
- The one-thing claims are asserted on the castle, which the suite already grades once; the small site's two folds are identical, so the field-for-field test proves the mapping, not the fold — [rpg-api#1249](https://github.com/KirkDiggler/rpg-api/pull/1249).

## Found

- At encounter v0.217.0 the castle's read-back has 654 statements (642 at v0.215.1); the difference is the join statements v0.216.0 added — [rpg-api#1249](https://github.com/KirkDiggler/rpg-api/pull/1249).
- `FoldedEach()` on the castle costs a median 0.53 ms; `Folded()` cost 6.4 ms at v0.215.1 — [rpg-api#1249](https://github.com/KirkDiggler/rpg-api/pull/1249).
- The castle's check encodes to 529,579 bytes with the one-thing fold (gzip 64,466), against 517,460 (62,710) with the whole-file fold at v0.215.1 — [rpg-api#1249](https://github.com/KirkDiggler/rpg-api/pull/1249).
- No api test pinned the statement count, and nothing else needed changing for the two-minor-version encounter jump — [rpg-api#1249](https://github.com/KirkDiggler/rpg-api/pull/1249).

## Mistaken

- none observed — [rpg-api#1249](https://github.com/KirkDiggler/rpg-api/pull/1249).
