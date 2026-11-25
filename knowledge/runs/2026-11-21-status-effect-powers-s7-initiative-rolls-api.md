# Run — 2026-11-21 — Status effect powers S7 initiative rolls through the api

Slice [rpg-project#1060](https://github.com/KirkDiggler/rpg-project/issues/1060) of story [rpg-project#1036](https://github.com/KirkDiggler/rpg-project/issues/1036). Base: `rpg-api` `dev` at [`2cddb1a3`](https://github.com/KirkDiggler/rpg-api/commit/2cddb1a3def63fdff3e8a4ce58e9d4d3bdf136ef).

One line per entry. Every line ends with a link: a PR, an issue comment, a commit, or a design section anchor. This file rides on the work PR, under the `knowledge/runs/` of the lowest directory that contains everything the PR changed. The ingest reads it once and adds a `processed:` line at the top; nobody else edits it after merge.

## Shipped

- The act stream's fight-formed entry carries each member's initiative roll; pins: session v0.204.0, encounter v0.237.0, dnd5e v0.224.0, wire format v0.1.285 — [rpg-api#1267](https://github.com/KirkDiggler/rpg-api/pull/1267).

## Decided

- The debug record needs no change here, because a record entry's payload passes through as it is — [rpg-project#1060](https://github.com/KirkDiggler/rpg-project/issues/1060).

## Found

- No existing test handed initiative an illegal d20 face under the new dice rule, so no fixture changed — [rpg-project#1060](https://github.com/KirkDiggler/rpg-project/issues/1060).
- The wire-format tag v0.1.285 is an annotated tag; `git rev-parse` gives the tag object and `^{commit}` gives d313f00 — [Rests on comment](https://github.com/KirkDiggler/rpg-project/issues/1060).

## Mistaken

- none observed — [rpg-project#1060](https://github.com/KirkDiggler/rpg-project/issues/1060).
