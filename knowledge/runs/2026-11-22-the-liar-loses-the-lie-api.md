# Run — 2026-11-22 — machine story S the-liar-loses-the-lie (rpg-api)

processed: 2026-11-25T190427Z

Slice [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111) of story [rpg-project#989](https://github.com/KirkDiggler/rpg-project/issues/989). Base: `rpg-api` `dev` at [`d3e7c12e`](https://github.com/KirkDiggler/rpg-api/commit/d3e7c12e).

## Shipped

- The served castle's captain no longer offers "He burned Ashford…" to the one he caught lying; encounter pinned `v0.245.0` and the one hunk copied into `content/the-mad-king.yaml` — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).

## Decided

- none beyond what the issue gave: the pin and the hunk exactly as the issue states — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).

## Found

- F1 holds: go.mod pins dnd5e v0.226.0, encounter v0.244.0, resolution v0.80.0, session v0.208.0 — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).
- F2 holds: the toolkit diff v0.244.0..v0.245.0 of world-builder-v4-castle.yaml is one hunk, the `turn` requires list plus the two-line comment — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).
- F3 holds: TestCastleCopiesAgree at internal/dungeons/castle_copies_test.go:34 — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).
- F4 holds: `id: turn` at line 1993 of the shipped castle with `requires: fact: king-burned-village` — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).
- go.mod diff: only `rulebooks/dnd5e/encounter v0.244.0 => v0.245.0`; go.sum two lines — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).
- Expected failure after the pin, before the copy: TestCastleCopiesAgree fails "content/the-mad-king.yaml compiles to the toolkit's castle"; the toolkit side has the extra not/noted caught-lying requirement (Topic "caught-lying") — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).
- After the copy TestCastleCopiesAgree and the session Gate tests pass — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).
- No existing test changed — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).
- Whole suite at the final head: `go test ./... -count=1 -timeout 15m` all packages ok; one earlier run failed only internal/integration/character on a lost redis test container (connection refused), and passed on rerun; `make lint` 0 issues — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).

## Mistaken

- none observed — [rpg-project#1111](https://github.com/KirkDiggler/rpg-project/issues/1111).

— cross-team agent, on behalf of KirkDiggler
