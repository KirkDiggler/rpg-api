# Run — 2026-11-11 — Courtyard S5 wall-face API mapping

Work [Project #919](https://github.com/KirkDiggler/rpg-project/issues/919), design [Project #915](https://github.com/KirkDiggler/rpg-project/pull/915).

## Shipped

- Copy captured wall-face choices into both snapshot and act-geometry protobuf messages; preserve existing recipient and opening-only semantics. Actual isolated rendering verification follows this checkpoint — [slice](https://github.com/KirkDiggler/rpg-project/issues/919).

## Decided

- Reuse upstream site storage already in API dev; no new storage adapter or compatibility migration is part of this change. Retain the current generated proto pin and adopt real pushed encounter/session feature pins on the updated baseline — [dependency resolution](https://github.com/KirkDiggler/rpg-project/issues/919#issuecomment-6805301981).
- The API copies side/material identities only. Texture lookup and wall-local mapping remain renderer responsibilities; no full authoring document or inferred region enters the response — [contract](https://github.com/KirkDiggler/rpg-api-protos/pull/427).

## Found

- Both mapper tests and native cold-host acceptance were RED with missing face lists, then GREEN. A fresh empty site provider reads the saved site from Redis after a valid source change; the old run retains Brick/Stucco and unchanged wall/cell facts — [slice](https://github.com/KirkDiggler/rpg-project/issues/919).
- `make test` and `make ci-check` pass on the exact committed dependency graph. Generated proto bytes remain upstream876f5202; encounter c78bb08a and session e755f509 are real pushed pseudo-pins, not replacement modules or released versions — [providers](https://github.com/KirkDiggler/rpg-toolkit/pull/2357).

## Mistaken

- none observed in the mapping change; the earlier unrelated SDK adoption block was resolved upstream rather than repaired inside this slice — [dependency resolution](https://github.com/KirkDiggler/rpg-project/issues/919#issuecomment-6805301981).
