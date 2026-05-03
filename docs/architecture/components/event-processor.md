---
name: event processor + Redis publisher
description: Encounter event persistence and real-time publication via Redis pub/sub
updated: 2026-05-02
confidence: high — verified by reading processor.go, redis.go (publisher), publisher.go
---

# event processor + Redis publisher

Two tightly-linked components that handle encounter event delivery: the event processor coordinates persistence + publication as a unit, and the Redis publisher delivers events to subscribed clients over Redis pub/sub.

## Files

| File | Lines | Purpose |
|---|---|---|
| `processors/event/processor.go` | 86 | Event processor: persist then publish |
| `publishers/encounter/publisher.go` | ~50 | Publisher interface + Input/Output types |
| `publishers/encounter/redis.go` | ~233 | Redis pub/sub implementation |

## Purpose

Every game action that mutates encounter state emits one or more `EncounterEvent`s. The processor provides a single point to:
1. Persist the event to the encounter log (source of truth).
2. Publish the event to the Redis channel for real-time client updates.

The separation means a Redis publish failure does not fail the game action — persistence is the source of truth. Clients that miss a real-time event can replay from the encounter log.

## Public interface

**Processor:**
```go
type Processor interface {
    Process(ctx context.Context, input *ProcessInput) (*ProcessOutput, error)
}

type ProcessInput struct {
    EncounterID string
    Event       *entities.EncounterEvent
}

type ProcessOutput struct {
    EventID string
}
```

**Publisher:**
```go
type Publisher interface {
    Publish(ctx, *PublishInput) (*PublishOutput, error)
    Subscribe(ctx, *SubscribeInput) (*SubscribeOutput, error)
    Unsubscribe(ctx, *UnsubscribeInput) (*UnsubscribeOutput, error)
}
```

## Implementation details

**Processor flow (`processor.go:62`):**
1. Append event to encounter log repo — this is the authoritative write. If it fails, the operation fails.
2. Publish to Redis channel — best-effort. If this fails, the error is silently discarded (line 78: `_, _ = p.publisher.Publish(...)`). There is no retry, no dead-letter queue, no alert.

**Redis publisher:**
- Channel pattern: `encounter:{encounterID}:events`
- Event serialized as JSON via `json.Marshal(input.Event)`.
- Subscribe creates a goroutine per subscription that reads from Redis pub/sub and forwards to a buffered Go channel (buffer size: 100).
- Subscription lifecycle: `Subscribe` returns an events channel; `Unsubscribe` cancels the goroutine and closes channels.

**Subscription context:** `context.WithCancel(context.Background())` — not the request context. The subscription lives beyond the HTTP/gRPC request that created it.

## Known issues

### `fmt.Printf` in publish failure path

`processor.go` drops the publish error silently with `_, _ = p.publisher.Publish(...)`. The original PR dropped the `fmt.Printf` that was previously there — the current state is completely silent on publish failure. In the combat hot path (every attack, every move emits events), publish failures are invisible.

**Recommended fix:** inject a logger via `Config` and use structured logging for publish failures.

### No retry on publish failure

Consistent with the best-effort semantic, but there is no circuit breaker or backpressure mechanism. If Redis is temporarily unavailable, all events silently fail to publish. The encounter log provides replay but only for clients that know to replay (late joiners using `GetEncounterHistory`). Clients already connected and streaming will miss events with no indication.

### Subscription goroutine leaks

`Subscribe` spawns a goroutine that runs until `Unsubscribe` is called or context cancellation. If a client disconnects without calling `Unsubscribe`, the goroutine runs until the server restarts. The goroutine is waiting on Redis pub/sub, so it will not consume CPU, but it holds memory and a Redis connection.

**Mitigation:** The streaming handler's `defer` block should always call `Unsubscribe`. Verify this is the case in `handlers/dnd5e/v1alpha1/encounter/handler.go`.

### JSON deserialization on subscribe

The subscriber deserializes events from Redis as `entities.EncounterEvent` (the full struct with custom JSON marshaling). This requires `encounter_events_json.go` custom marshaling to be correct. Any marshaling bug means the subscriber silently drops the event with an error logged to the subscription's errors channel.
