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

**Current event delivery:** the v1alpha2 encounter path
(`internal/orchestrators/encounter/v2/`) publishes through the rpg-toolkit
encounter SDK's own `tkenc.Broker` — a different mechanism, not a replacement
for this package, and not documented in this file. See
[`encounter.md`](./encounter.md).

See `docs/status.md` "Active work" for the full deletion tally.
