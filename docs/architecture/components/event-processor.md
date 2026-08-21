---
name: event processor + Redis publisher
description: DELETED — v1 encounter event persistence/publication, removed in rpg-api#642
updated: 2026-07-13
confidence: high — verified by grep (zero remaining consumers) before deletion
---

# event processor + Redis publisher — DELETED (rpg-api#642, 2026-07-13)

`internal/processors/event/` and `internal/publishers/encounter/` are both
deleted in full. These persisted and published `entities.EncounterEvent`s for
the v1alpha1 EncounterService; they had zero consumers left once the v1
orchestrator and handler were gone (verified by grep before deletion — the
event processor's only callers were the v1 orchestrator and `cmd/server`/test
harness wiring; the publisher's only caller was the event processor and the
v1 handler directly).

The known gap this doc used to describe (publish errors silently discarded
via `_, _ = p.publisher.Publish(...)`, no logging/retry/alert) is moot — the
code is gone.

**Then-current event delivery:** the v1alpha2 encounter path
(`encounter.md`) this pointer used to name is ALSO deleted now
(rpg-project#227, 2026-08-21) — see [`encounter.md`](./encounter.md) for that
removal. Event delivery is now the `rulebooks/dnd5e/session` SDK's own event
stream, fanned out by `internal/orchestrators/session.Broker` — a different
mechanism again, not documented in this file.

See `docs/status.md` "Active work" for the full deletion tally.
