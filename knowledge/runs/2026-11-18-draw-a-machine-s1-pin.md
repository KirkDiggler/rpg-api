# Run — 2026-11-18 — draw a machine S1 pin (api)

Slice [rpg-project#994](https://github.com/KirkDiggler/rpg-project/issues/994) of story [rpg-project#993](https://github.com/KirkDiggler/rpg-project/issues/993). Base: `rpg-api` `dev` at [`f376ea60`](https://github.com/KirkDiggler/rpg-api/commit/f376ea60).

## Shipped

- `rulebooks/dnd5e/encounter` pinned v0.219.0 → v0.220.0 (the routine shape connections), the only `go.mod` line that moved; no protos or mapping change — a connection's kind and role are open strings.

## Decided

- The castle check asserts the gate guard's machine by the routine named in each statement's path (`behaviorDefinitions.gate-1-policy.`), not its subject, and counts only `routine.wake` statements, so the repeats under fact and topic hubs are not counted.
- The only count pinned is the twelve steps with a `to` named in the issue; total steps are logged, not asserted.

## Found

- At v0.220.0 `gate-1-policy` says 26 steps, 12 of which move; `turned` has steps but none that move, and no state says `final`.
- The castle says 708 statements in 9 folded sections, unchanged from v0.219.0; its ways stay 29.

## Mistaken

- none observed.
