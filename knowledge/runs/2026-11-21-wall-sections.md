# Run — 2026-11-21 — Captured wall sections through the API

## Shipped

- Candidate adapters carry the Session wall-section list into both session protocol versions without filtering, unit conversion or source enrichment; native two-member/cold-host integration covers the joined path — [Courtyard S6](https://github.com/KirkDiggler/rpg-project/issues/1022).

## Decided

- Consume the actual generated Go artifact `200e4b5d6efd` after the operator merged Protos449; use real pushed Encounter9c166b2d and Session43538a14 pseudo-versions until their authorized releases, with no local replacement graph — [publication decision](https://github.com/KirkDiggler/rpg-api-protos/pull/449).

## Found

- Both clients receive one captured window section alongside the same wall's one door opening. A new host loads the persisted site once and retains both members' walls/doors/cells after the source and compiled input are changed. Omitting the v1 mapping makes that joined test fail on the missing section — [slice](https://github.com/KirkDiggler/rpg-project/issues/1022).

## Mistaken

- The first API gate found the historical v0.220.0 castle oracle differed only by newly serialized `Sections:null`. The oracle file remains untouched: the test now compares canonical JSON on both sides, omitting only that absent field at `Field.StructuralWalls`; a new assertion proves a real section remains distinguishable, and the changed-DC counterexample still fails equality — [slice](https://github.com/KirkDiggler/rpg-project/issues/1022).
