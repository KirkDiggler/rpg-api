# Run — 2026-11-11 — gate conversation S3 address on the Speak row

Slice [rpg-project#923](https://github.com/KirkDiggler/rpg-project/issues/923) of story [rpg-project#769](https://github.com/KirkDiggler/rpg-project/issues/769). Base: `rpg-api` `dev` at [`d06228ff`](https://github.com/KirkDiggler/rpg-api/commit/d06228ff).

## Shipped

- Session `Declaration.Address` crosses as proto `Declaration.address` (nil absent, malformed or off-Speak refused Internal) — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).
- `content/the-mad-king.yaml` matches the toolkit castle at encounter `3a5428e8` (front-gate approach within 2; captain, gate-1, gate-2 hail rows) — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).
- A lobby launch refused as `ErrInvalidWorld` answers FAILED_PRECONDITION with the SDK's sentence — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).

- Session `TableAnswer.To` crosses as v1alpha2 `TableAnswer.to` on the table-answer act and a speak act's answer (zero party absent), after the operator's early look asked that the guard's line say who it is spoken to — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).

## Decided

- `MoveOutput.Addressed` stays unmapped: the Move response already says STOPPED with the steps taken, and the Speak row is the panel's one source — [#923 How it works](https://github.com/KirkDiggler/rpg-project/issues/923).
- ErrInvalidWorld in the lobby is FAILED_PRECONDITION, not the INVALID_ARGUMENT `sdkerr` gives it, because the host's request is well formed and the stored dungeon must change before a retry — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).
- The api does not check the approach against Speak reach at authoring; that would be the api judging a rule, so a saved 5 is refused only at launch until the toolkit refuses it at compile — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).
- Gate walks in older acceptance tests are hailed once and walk on; the `walk` helper requires the Speak address when a walk stops with no reason — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).

## Found

- Walking the road from the party start, the first hailer is the captain (alice at offset (-1,10), the captain 2 cells away), with the line "You there. Nobody passes this gate without my say. Speak." — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).
- The dungeonspec compile accepts `approach: { within: 5 }`; only `session.Launch` (`ValidateApproachReach`) refuses it — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).
- Before this run a launch's ErrInvalidWorld through StartEncounter (seat count included) reached the client as Internal "lobby: ..." — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).
- A hailed Move carries an empty stop reason — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).

- On the castle the captain's hail reaches the hailed member's stream with `to` = that member — [rpg-api#1222](https://github.com/KirkDiggler/rpg-api/pull/1222).

## Mistaken

- none observed
