---
name: devseed
description: DELETED — CLI that seeded Redis with old-stack characters/encounters for the playtest harness, removed in rpg-project#227
updated: 2026-08-21
confidence: high — verified by grep (zero remaining references) before deletion
---

# devseed — DELETED (rpg-project#227, 2026-08-21)

`cmd/devseed` is deleted in full. It wrote fixture characters plus a
`*tkenc.Data` encounter directly into Redis under the old `enc:v2:<id>` key
prefix, for the rpg-dnd5e-web playtest harness and MCP-driven verification.
With `internal/repositories/encounters/v2` and the v1alpha2 EncounterService
it seeded for both deleted (see [`encounter.md`](./encounter.md)), the tool
had nothing left to write for and no consumer that could read what it wrote.
Had zero new-stack (session SDK) usage to carry forward — checked before
deletion.

`internal/pkg/devcombat` (the `--inject-combat` implementation devseed's CLI
wrapped) is deleted alongside it, for the same reason.

See `docs/status.md` "Active work" for the full deletion tally.
