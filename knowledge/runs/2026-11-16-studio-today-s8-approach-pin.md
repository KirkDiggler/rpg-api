# Run — 2026-11-16 — studio today S8 approach pin (api)

processed: 2026-11-25T190427Z

Slice [rpg-project#974](https://github.com/KirkDiggler/rpg-project/issues/974) of story [rpg-project#966](https://github.com/KirkDiggler/rpg-project/issues/966). Base: `rpg-api` `dev` at [`e54e2749`](https://github.com/KirkDiggler/rpg-api/commit/e54e2749).

## Shipped

- `rulebooks/dnd5e/encounter` pinned v0.218.0 → v0.218.1 (rpg-toolkit#2445), the only `go.mod` line that moved; no protos or mapping change — [rpg-api#1250](https://github.com/KirkDiggler/rpg-api/pull/1250).

## Decided

- The castle check finds the kitchen door's thieves' tools lock way by its `through` join and its tool, never by index, and asserts the tool's `with` connection immediately follows the Dexterity `by` connection at the same difficulty — [rpg-toolkit#2445](https://github.com/KirkDiggler/rpg-toolkit/pull/2445).
- The troupe way's two checks are asserted `by`: each is an alternative, not a part — [rpg-toolkit#2445](https://github.com/KirkDiggler/rpg-toolkit/pull/2445).

## Found

- At v0.218.0 the kitchen lock's thieves' tools connection carried role `by`, which read as a third alternative; the new assertion failed on exactly that before the pin — [rpg-toolkit#2445](https://github.com/KirkDiggler/rpg-toolkit/pull/2445).
- The patch needed no other api change: `make test` green, 42 packages — [rpg-toolkit#2445](https://github.com/KirkDiggler/rpg-toolkit/pull/2445).

## Mistaken

- none observed — [rpg-toolkit#2445](https://github.com/KirkDiggler/rpg-toolkit/pull/2445).
