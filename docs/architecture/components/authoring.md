---
name: authoring durability
description: DELETED — PutDungeon's per-key transaction ordering and crash guarantees, removed in rpg-project#227
updated: 2026-08-21
confidence: high — verified by grep (zero remaining references) before deletion
---

# Authoring durability — DELETED (rpg-project#227, 2026-08-21)

> **Replaced 2026-08-23 (rpg-api#806):** the session-stack `AuthoringService`
> and the `internal/dungeons` content registry are described in
> [`authoring-service.md`](./authoring-service.md). This page is history.

`internal/orchestrators/authoring/` (PutDungeon, floor-plan preview, the
durable write-through source this doc described) and
`internal/handlers/dnd5e/authoring/v1alpha1/` are deleted in full, along with
`internal/dungeonregistry` (the shared live registry PutDungeon wrote through
and `LobbyService.ListDungeons` read back out — see
[`lobby-service.md`](./lobby-service.md)).

All of it compiled content through the toolkit's OLD `github.com/KirkDiggler/
rpg-toolkit/encounter/dungeonspec` dialect — there was no new-dialect
(`rulebooks/dnd5e/encounter/dungeonspec`) path anywhere in this package.
`AuthoringService` was gated behind `RPG_AUTHORING_ENABLED` and never
registered in any real deployment; its registration is removed from
`cmd/server/server.go` rather than kept behind a permanently-off flag for a
compiler dialect nothing else uses any more.

Kirk's ruling on the deletion: "anything we build now is not playable
anyway; losing it means we lose nothing."

See `docs/status.md` "Active work" for the full deletion tally.
