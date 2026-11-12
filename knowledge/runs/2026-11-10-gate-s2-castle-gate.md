# Run — 2026-11-10 — the entrance remembers S2 castle gate

processed: 2026-11-12T165555Z

Slice [rpg-project#922](https://github.com/KirkDiggler/rpg-project/issues/922) of story [rpg-project#769](https://github.com/KirkDiggler/rpg-project/issues/769). Base: `rpg-api` `dev` at [`83082f15`](https://github.com/KirkDiggler/rpg-api/commit/83082f15).

## Shipped

- The-mad-king's root `front-gate` interaction, the captain's guest claim beside the troupe claim, and both claims on gate-1 and gate-2 — [rpg-api#1218](https://github.com/KirkDiggler/rpg-api/pull/1218).

## Decided

- A first failed claim plays a warning line and no longer teaches `party-recognized`, because the keep design makes the second failure the arrest — [rpg-project#922](https://github.com/KirkDiggler/rpg-project/issues/922).
- The guest claim's success teaches `troupe-admitted`, because that fact is the gate post's pass until Gate S5 — [rpg-project#922](https://github.com/KirkDiggler/rpg-project/issues/922).

## Found

- The castle compiles under the Gate S2 dungeonspec on the first pass; `front-gate` lowers onto the field with five states — [rpg-api#1218](https://github.com/KirkDiggler/rpg-api/pull/1218).
- Session PR head 00a3694a predates session v0.176.0, which api dev already needs, so `internal/orchestrators/session` does not build at that pin — [rpg-toolkit#2359](https://github.com/KirkDiggler/rpg-toolkit/pull/2359).

## Mistaken

- none observed
