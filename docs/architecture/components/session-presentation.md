---
name: session presentation
description: SessionPresentationService v1alpha1 — live-only shared dice throw choreography for a started session
updated: 2026-08-28
confidence: medium-high — handler/orchestrator/repository unit suites plus cross-instance Redis integration prove wiring, auth, byte-equal fan-out, and no Story mutation; no browser walkthrough yet
---

# session presentation

`SessionPresentationService` is a presentation-only side channel for shared
rigid-body dice throw choreography. It is intentionally separate from
`SessionService`: game truth, combat results, and replayable history stay in
the toolkit session Story; this service validates and fans out bounded visual
plans that illustrate an already-authoritative beat.

## Surface

Proto package/service:
`dnd5e.api.session.presentation.v1alpha1.SessionPresentationService`

Methods:

- `PublishDiceThrow` — validates one client-generated `DiceThrowDraft`, binds
  `session` and `roller` from the authenticated request, stores the first
  accepted `(session, presentation_id, attempt)` payload, and returns that
  accepted `DiceThrowPlan`.
- `StreamDiceThrows` — live-only server stream of accepted plans for a seated
  session member. There is no replay contract.

## Layers and wiring

```
internal/handlers/dnd5e/sessionpresentation/v1alpha1/   proto <-> domain, auth/status mapping
internal/orchestrators/sessionpresentation/             validate, bind, deterministic JSON payload
internal/repositories/sessionpresentation/              Redis SET/PUBLISH + SUBSCRIBE
```

Production (`cmd/server/server.go`) and the integration harness construct:

1. the Redis-backed roster repository shared with `LobbyService.StartEncounter`;
2. one shared `sessionaccess.Access` over the character repository and roster;
3. the `SessionService` handler with that access;
4. the Redis presentation repository and orchestrator;
5. the presentation handler with the same access; and
6. service registration plus health status for exactly
   `dnd5e.api.session.presentation.v1alpha1.SessionPresentationService`.

The test harness also exposes `SessionClient`, `SessionPresentationClient`,
`HealthClient`, and `RosterRepo` so integration tests can prove the real wire
and storage paths rather than bypassing the gate.

## Auth and access

Both RPCs require the normal gRPC auth interceptors. The handler calls
`sessionaccess.Access.CallerMemberSeated` before it touches the presentation
service:

- `Unauthenticated` when auth did not inject a player ID;
- `InvalidArgument` for empty `session` or `member`;
- `NotFound` when the character/member or roster row is absent;
- `PermissionDenied` when the caller does not own `member` or owns no player
  row seated in the session.

The seated check reads the launch-written Redis roster row. Server B accepting a
stream for Bob after server A launched the lobby is therefore a production-gate
proof, not an in-memory test shortcut.

## Validation limits

The accepted plan shape is intentionally narrow:

- `schema_version` must be `1`.
- `presentation_id` and every `die_id` must match
  `^[A-Za-z0-9][A-Za-z0-9:_-]{0,127}$`.
- `attempt` is `1..32`.
- `physics_schema` must be
  `DICE_PHYSICS_SCHEMA_RAPIER_DUNGEON_D20_V1`.
- `collider_fingerprint` is exactly 32 bytes.
- Body count is `1..20`; this schema accepts D20 bodies only.
- Terminal count must match body count; terminal steps are `1..480`; terminal
  kind is `SETTLED` or `OFF_TABLE`.
- Contacts are ordered by non-decreasing step, step `1..480`, at most 128
  contacts, and at most 256 total checkpoint body states. Equal-step contacts
  must be in strict canonical order by `PrimaryDieID`, target kind string
  (`dice`, `door`, `wall`), then target ID (`OtherDieID` or
  `StaticCollider.ColliderID`).
- Static contacts must name printable ASCII collider IDs of length `1..256`
  with `wall:` or `door:` prefixes matching their kind.
- Position components are finite and within `±4096`; linear velocity magnitude
  is `<=64`; angular velocity magnitude is `<=128`.
- Quaternions must be finite and normalized within absolute norm error
  `<=0.0001`.
- The deterministic encoded plan payload must be at most 64 KiB.

## Redis and live-only semantics

The repository stores accepted payloads under hashed session keys for two
minutes. The first publisher for `(session, presentation_id, attempt)` writes
and publishes. An identical duplicate returns the accepted payload without a
second publish. A different payload for the same tuple returns `AlreadyExists`.

Subscriptions use Redis Pub/Sub on the hashed session channel. They are
live-only by design: a subscriber that connects after a publish does not receive
old choreography. Stream context cancellation closes the Redis subscription and
the handler returns without appending anything to session history.

## Story/toolkit boundary

The presentation orchestrator does not call `session.Manager`, `Story`, dice,
combat, movement, or any toolkit rule API. `authority_seq` is only an opaque
reference to a game beat the client already knows; it is not checked by this
service and cannot create or mutate a beat. `PublishDiceThrow` changes only the
presentation repository/PubSub state.

`internal/integration/sessionpresentation/shared_throw_test.go` proves this end
to end: two `TestServer` instances share one Redis container; server A launches
Alice and Bob through the real `LobbyService` path; Alice and Bob open streams
on servers A and B; Alice publishes one valid one-body draft through server A;
both streams receive deterministic proto byte-equal plans; and Alice's
`SessionService.GetStory` sequence list is identical before and after publish.

## Verify

- `go test ./internal/handlers/dnd5e/sessionpresentation/v1alpha1`
- `go test ./internal/orchestrators/sessionpresentation`
- `go test ./internal/repositories/sessionpresentation`
- `go test ./internal/integration/sessionpresentation -race -count=3`

No browser walkthrough has been recorded yet; confidence is medium-high until a
web client consumes the live-only side channel in a real play session.
