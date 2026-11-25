# Run — 2026-11-16 — studio today S7 hubs pin (api)

processed: 2026-11-25T190427Z

Slice [rpg-project#973](https://github.com/KirkDiggler/rpg-project/issues/973) of story [rpg-project#966](https://github.com/KirkDiggler/rpg-project/issues/966). Base: `rpg-api` `dev` at [`dccd03c1`](https://github.com/KirkDiggler/rpg-api/commit/dccd03c1).

## Shipped

- `rulebooks/dnd5e/encounter` pinned v0.218.1 → v0.219.0 (rpg-toolkit#2447, the Records and Secrets hubs), the only `go.mod` line that moved; no protos or mapping change — [rpg-api#1251](https://github.com/KirkDiggler/rpg-api/pull/1251).

## Decided

- The castle check guards ways at the wire: the statements going `through` a join are exactly the `place.way.*` statements, and only a way carries a `method`, so a consumer counting ways by either rule counts a hub's way pointer zero times — [rpg-api#1251](https://github.com/KirkDiggler/rpg-api/pull/1251).
- The hubs are asserted by their groups' About (`{record, kitchen-way}`, `{concealment, kitchen-passage}`) and statement kinds, not by section position or count — [rpg-api#1251](https://github.com/KirkDiggler/rpg-api/pull/1251).

## Found

- At v0.219.0 the castle says 708 statements in 9 folded sections (683 in 7 at v0.218.x); its ways stay 29, so the hub pointers are not ways — [rpg-api#1251](https://github.com/KirkDiggler/rpg-api/pull/1251).
- The castle check is 286.1 ms (`readback.Of` 168.9 ms, `FoldedEach` 0.72 ms) and encodes to 584,046 bytes, gzip 71,304 — [rpg-api#1251](https://github.com/KirkDiggler/rpg-api/pull/1251).
- The way guard already held at v0.218.1, where no hub repeated a way; it is a guard for consumers, not a change this pin makes — [rpg-api#1251](https://github.com/KirkDiggler/rpg-api/pull/1251).

## Mistaken

- none observed — [rpg-api#1251](https://github.com/KirkDiggler/rpg-api/pull/1251).
