# Run — 2026-11-16 — studio today S10 folded read-back (api)

processed: 2026-11-25T190427Z

Slice [rpg-project#978](https://github.com/KirkDiggler/rpg-project/issues/978) of story [rpg-project#966](https://github.com/KirkDiggler/rpg-project/issues/966). Base: `rpg-api` `dev` at [`186dcf8f`](https://github.com/KirkDiggler/rpg-api/commit/186dcf8f).

## Shipped

- The check's read-back carries `folded`, the engine's `Folded()` mapped field for field, recursively, order kept — [rpg-api#1247](https://github.com/KirkDiggler/rpg-api/pull/1247).
- Protos pinned to `generated` at `fbf6c674` (tag `v0.1.280`); no toolkit bump — [rpg-api#1247](https://github.com/KirkDiggler/rpg-api/pull/1247).

## Decided

- `Folded()` is called in the registry's `readBackOf`, inside the read-back's recover, and carried on `PutResult.Folded`; the handler only maps, so one containment covers both and a panic folding yields no read-back, never an error — [rpg-api#1247](https://github.com/KirkDiggler/rpg-api/pull/1247).
- A second seam, `SetFold` in `export_test.go`, reaches the fold's panic the same way `SetReadBack` reaches the read-back's — [rpg-api#1247](https://github.com/KirkDiggler/rpg-api/pull/1247).
- The fold is compared at the wire against the engine's JSON of `Folded()` on the small site, and the castle (graded once) asserts every statement path is carried by some folded item — [rpg-api#1247](https://github.com/KirkDiggler/rpg-api/pull/1247).

## Found

- `Folded()` on the castle costs a median 6.4 ms in-process, beside 150.7 ms for `readback.Of` and 267.7 ms for the whole check — [rpg-api#1247](https://github.com/KirkDiggler/rpg-api/pull/1247).
- The castle's check encodes to 517,460 bytes with the fold (415,124 without; 83,138 with no read-back); gzip 62,710 (46,812; 12,068) — [rpg-api#1247](https://github.com/KirkDiggler/rpg-api/pull/1247).
- The castle's fold has 7 sections and carries the path of every one of its 642 statements — [rpg-api#1247](https://github.com/KirkDiggler/rpg-api/pull/1247).

## Mistaken

- none observed — [rpg-api#1247](https://github.com/KirkDiggler/rpg-api/pull/1247).
