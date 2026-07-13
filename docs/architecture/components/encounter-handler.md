---
name: encounter handler
description: DELETED — v1alpha1 EncounterService handler, removed in rpg-api#642
updated: 2026-07-13
confidence: high — verified by grep (zero remaining references) before deletion
---

# encounter handler — DELETED (rpg-api#642, 2026-07-13)

`internal/handlers/dnd5e/v1alpha1/encounter/handler.go` and `converters.go` are
deleted. This was the gRPC adapter for the `dnd5e.api.v1alpha1.EncounterService`
— the audit-flagged live-but-legacy path (rpg-api's twin of rpg-dnd5e-web#448's
clean slate). Verified before deletion that the web made zero calls to this
service; only the v1alpha2 `EncounterClient` was ever exercised.

Every gRPC method this doc used to list (`CreateEncounter`, `JoinEncounter`,
`SetReady`/`LeaveEncounter`, `StartCombat`, `CreateDungeon`,
`GetEncounterState`, `StreamEncounterEvents`, `ResolveAttack`,
`MoveCharacter`, `ActivateFeature`, `ActivateCombatAbility`, `ExecuteAction`,
`EndTurn`, `OpenDoor`, `PlayerDisconnected`, `GetEncounterHistory`) is gone
along with the service registration in `cmd/server/server.go`.

**Current encounter handler:** see [`encounter.md`](./encounter.md) — the
v1alpha2 encounter service, the only encounter handler remaining. It never had
the boundary violations this doc used to describe (toolkit spatial types
hardcoded, `*toolkitchar.Data` type assertions) — it was designed load→verb→
persist from the start.

See `docs/status.md` "Active work" for the full deletion tally.
