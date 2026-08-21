---
name: encounter (v1alpha2)
description: DELETED — v1alpha2 encounter service and its old-stack toolkit dependency, removed in rpg-project#227
updated: 2026-08-21
confidence: high — verified by grep (zero remaining references) before deletion
---

# encounter (v1alpha2) — DELETED (rpg-project#227, 2026-08-21)

`internal/handlers/dnd5e/v2/encounter/`, `internal/orchestrators/encounter/v2/`,
and `internal/repositories/encounters/v2/` are deleted in full (~22.5k lines).
This was the second-generation encounter vertical, built on the toolkit's
old `github.com/KirkDiggler/rpg-toolkit/encounter` module — replaced by the
`rulebooks/dnd5e/session` SDK, integrated directly into the lobby service
(see [`lobby-service.md`](./lobby-service.md)). The old EncounterService
(`dnd5e.api.v1alpha2.encounter.EncounterService`) is not reimplemented; there
is no new proto-level encounter service — the lobby's `StartEncounter` is the
sole path a session comes into existence, and gameplay verbs ride the
`SessionService` (`internal/handlers/dnd5e/session/v1alpha1/`, not yet
documented here).

`BuildEquipmentCharacterData` (this package's one export the character
handler depended on) moved to `internal/handlers/dnd5e/v2/character/
character_data.go` — see [`character-v2-handler.md`](./character-v2-handler.md).

See `docs/status.md` "Active work" for the full deletion tally.
