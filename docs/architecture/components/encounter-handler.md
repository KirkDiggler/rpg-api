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

**Then-current encounter handler:** the v1alpha2 encounter service
(`encounter.md`) this pointer used to name is ALSO deleted now
(rpg-project#227, 2026-08-21) — see [`encounter.md`](./encounter.md) for that
removal. There is no proto-level encounter handler any more; gameplay verbs
ride `SessionService` (`internal/handlers/dnd5e/session/v1alpha1/`).

See `docs/status.md` "Active work" for the full deletion tally.
