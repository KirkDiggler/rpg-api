# Run — 2026-11-22 — rounds and chapel prayer api pins

Slice [rpg-project#1101](https://github.com/KirkDiggler/rpg-project/issues/1101) of stories [rpg-project#1080](https://github.com/KirkDiggler/rpg-project/issues/1080) and [rpg-project#1036](https://github.com/KirkDiggler/rpg-project/issues/1036). Base: `rpg-api` `dev` at [`81d2f264`](https://github.com/KirkDiggler/rpg-api/commit/81d2f264).

## Shipped

- The encounter, session and resolution pins move together to v0.241.0, v0.207.0 and v0.80.0; no other requirement line moved — [rpg-project#1101](https://github.com/KirkDiggler/rpg-project/issues/1101).
- The shipped castle gains the `prayed` status and the altar's `pray` option, copied exactly from the toolkit's castle — [rpg-project#1101](https://github.com/KirkDiggler/rpg-project/issues/1101).
- Four status tests: no rounds and a wrong `until` key are refused with their paths and sentences, three rounds is accepted, and the shipped castle's prayer is read — [rpg-project#1101](https://github.com/KirkDiggler/rpg-project/issues/1101).

## Decided

- None; the issue handed every choice (pins, hunks, test names, paths) — [rpg-project#1101](https://github.com/KirkDiggler/rpg-project/issues/1101).

## Found

- F1 to F6 held at dev 81d2f264 (pins, tags, test names, castle statuses and altar, toolkit hunks) — [Rests on comment](https://github.com/KirkDiggler/rpg-project/issues/1101#issuecomment-7070367281).
- After the pins alone `TestCastleCopiesAgree` failed; after the exact copy of the two hunks it passed — [rpg-project#1101](https://github.com/KirkDiggler/rpg-project/issues/1101).
- Before the pins the new until tests failed with "apply_status takes `status` and `to`, not `until`" — [rpg-project#1101](https://github.com/KirkDiggler/rpg-project/issues/1101).
- The shelf refuses a put under a shipped key ("that key is a reference"), so the prayer test reads the shipped castle through the shelf's compile instead of a put — [rpg-project#1101](https://github.com/KirkDiggler/rpg-project/issues/1101).
- Existing tests edited: none; the session integration tests all pass unchanged — [rpg-project#1101](https://github.com/KirkDiggler/rpg-project/issues/1101).

## Mistaken

- The prayer test first tried a validate-only put under the shipped key and under a mismatched key; both refused, and it was rewritten to read the shipped castle like `TestTheShippedCastlesStatusPasses` — [rpg-project#1101](https://github.com/KirkDiggler/rpg-project/issues/1101).
