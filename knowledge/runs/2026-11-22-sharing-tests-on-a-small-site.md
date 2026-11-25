# Run — 2026-11-22 — sharing tests on a small site

Issue [rpg-api#1274](https://github.com/KirkDiggler/rpg-api/issues/1274). Base: `rpg-api` `dev` at [`ed4bfc65`](https://github.com/KirkDiggler/rpg-api/commit/ed4bfc65).

## Shipped

- The five `TestSharing*` tests run on the kitchen site instead of the shipped castle, every assertion unchanged; each takes 0.01-0.03 s, down from 0.22-0.85 s — [rpg-api#1274](https://github.com/KirkDiggler/rpg-api/issues/1274).

## Decided

- The shelf's default dungeon is the kitchen rekeyed to `the-mad-king`, because `NewFileRegistry` refuses a shelf with no default, and the registry suite already uses this stand-in pattern — [amendment 1](https://github.com/KirkDiggler/rpg-api/issues/1274#issuecomment-7075120450).
- `TestSharingUnmarkedCastleIsUnchanged` is renamed `TestSharingAnUnmarkedSiteIsUnchanged`, because it no longer runs on the castle — [the brief](https://github.com/KirkDiggler/rpg-api/issues/1274#issuecomment-7075104314).

## Found

- A shelf with no `the-mad-king.yaml` fails the registry build before any assertion runs (`internal/dungeons/registry.go`, default dungeon check) — [the gap](https://github.com/KirkDiggler/rpg-api/issues/1274#issuecomment-7075116165).
- Before, on the castle: EachTheirOwn 0.85 s, SharedView 0.78 s, UnmarkedCastle 0.78 s, UnknownWord 0.22 s, SharedKnowledge 0.80 s — [rpg-api#1274](https://github.com/KirkDiggler/rpg-api/issues/1274).
- After, on the kitchen: EachTheirOwn 0.03 s, SharedView 0.02 s, UnmarkedSite 0.02 s, UnknownWord 0.01 s, SharedKnowledge 0.02 s; no assertion needed changing — [rpg-api#1274](https://github.com/KirkDiggler/rpg-api/issues/1274).

## Mistaken

- The brief omitted that the registry requires a default dungeon on the shelf, so the first attempt failed all five tests; the amendment fixed the staging — [the gap](https://github.com/KirkDiggler/rpg-api/issues/1274#issuecomment-7075116165).
