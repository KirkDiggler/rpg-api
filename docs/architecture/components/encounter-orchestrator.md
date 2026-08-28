---
name: encounter orchestrator
description: DELETED — the 5,844-line v1 encounter orchestrator, removed in rpg-api#642
updated: 2026-07-13
confidence: high — verified by grep (zero remaining references) before deletion
---

# encounter orchestrator — DELETED (rpg-api#642, 2026-07-13)

`internal/orchestrators/encounter/orchestrator.go` (5,844 lines) and its
v1-only siblings — `service.go`, `monster_turns.go`, `perception.go`,
`dungeon_mapper.go`, and their tests/mocks — are deleted. This was the most
critical and most at-risk file in the repo: proto types constructed inline
(39 `pb.` references), an `interface{}`-typed room-data field, a hardcoded
debug theme, and coordinate-space bugs that took five separate fix commits.
All of it is gone rather than incrementally fixed — the file had no live
production consumer once the v1alpha1 EncounterService it backed was
unregistered (verified: the web made zero calls to it).

Six open PRs (#459, #461, #463, #466, #467, #468) targeted this file's
coordinate-space bugs. They are now moot — see `docs/status.md` "Paused / on
hold" for the record.

**Then-current encounter orchestrator:** the v1alpha2 orchestrator
(`encounter.md`) this pointer used to name is ALSO deleted now
(rpg-project#227, 2026-08-21) — see [`encounter.md`](./encounter.md) for that
removal. Encounter construction is now `LobbyService.StartEncounter`,
building directly onto the `rulebooks/dnd5e/session` SDK (see
[`lobby-service.md`](./lobby-service.md)).

See `docs/status.md` "Active work" for the full deletion tally.
