# Run — 2026-11-07 — alive-not-lagging S7–S9 api pin

processed: 2026-11-12T165555Z

Slices [rpg-project#891](https://github.com/KirkDiggler/rpg-project/issues/891), [#892](https://github.com/KirkDiggler/rpg-project/issues/892) and [#893](https://github.com/KirkDiggler/rpg-project/issues/893) of story [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879). Base: `rpg-api` `dev` at [`aff1c72`](https://github.com/KirkDiggler/rpg-api/commit/aff1c72).

## Shipped

- `rulebooks/dnd5e/session` pinned at v0.169.0 and `rulebooks/dnd5e/encounter` at v0.184.0; no other toolkit module moved — [go.mod](https://github.com/KirkDiggler/rpg-api/blob/chore/repin-session-0.169-encounter-0.184/go.mod).
- `*ActBroker` implements `session.ActListeners`: `Listening` names the recipients of a session with a live subscription — [act_listeners.go](https://github.com/KirkDiggler/rpg-api/blob/chore/repin-session-0.169-encounter-0.184/internal/orchestrators/session/act_listeners.go).

## Decided

- `Listening` lives in a new file and touches no broker code, because rpg-api#1199 edits `act_broker.go` and the stream handlers in parallel — [rpg-api#1199](https://github.com/KirkDiggler/rpg-api/pull/1199).
- `Listening` returns names sorted and never errors; an empty session is an empty list, never nil — [act_listeners.go](https://github.com/KirkDiggler/rpg-api/blob/chore/repin-session-0.169-encounter-0.184/internal/orchestrators/session/act_listeners.go).
- No wiring change: the broker is already `sdk.Config.Acts` in `orchestrator.go`, so the SDK finds the interface by assertion — [orchestrator.go](https://github.com/KirkDiggler/rpg-api/blob/chore/repin-session-0.169-encounter-0.184/internal/orchestrators/session/orchestrator.go#L241).

## Found

- A lagged subscriber and a broker Close both drop out of `Listening` at once, because both remove the subscription from the map the answer reads — [act_listeners_test.go](https://github.com/KirkDiggler/rpg-api/blob/chore/repin-session-0.169-encounter-0.184/internal/orchestrators/session/act_listeners_test.go).
- Mutating `Listening` to name only the other member makes the acceptance test fail on "the subscribed player receives her own walk live", so the test proves the SDK consults the broker — [act_listeners_acceptance_test.go](https://github.com/KirkDiggler/rpg-api/blob/chore/repin-session-0.169-encounter-0.184/internal/integration/session/act_listeners_acceptance_test.go).

## Mistaken

- The first `go test -race ./...` failed two packages at link time with "no space left on device" on the shared disk; each passed when rerun alone. No code fault — [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879).
