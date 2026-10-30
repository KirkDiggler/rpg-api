# 2026-10-30 · Frank's list S9 · The ending's result crosses the api

Story [rpg-project#714](https://github.com/KirkDiggler/rpg-project/issues/714), slice [rpg-project#733](https://github.com/KirkDiggler/rpg-project/issues/733). Contract [rpg-api-protos#408](https://github.com/KirkDiggler/rpg-api-protos/pull/408), session [rpg-toolkit#2180](https://github.com/KirkDiggler/rpg-toolkit/pull/2180), web [rpg-dnd5e-web#1361](https://github.com/KirkDiggler/rpg-dnd5e-web/pull/1361).

## Shipped

- The v1alpha2 clock translator copies `session.ClockEntry.Outcome` onto `ClockEntry.outcome`. An empty result crosses as unspecified; a result this build cannot spell refuses the act, as every closed-set value does · [rpg-api#1158](https://github.com/KirkDiggler/rpg-api/pull/1158)
- Pins: generated protos for v0.1.247 ([rpg-api-protos#408](https://github.com/KirkDiggler/rpg-api-protos/pull/408)) and the session module at v0.148.0, which carries the result ([rpg-toolkit#2180](https://github.com/KirkDiggler/rpg-toolkit/pull/2180)) · [rpg-api#1158](https://github.com/KirkDiggler/rpg-api/pull/1158)

## Decided

- A combatant's side needs no api change: `Sighting.stance` is already copied from `session.Sighting.Stance` · [convert.go sightingToProto](https://github.com/KirkDiggler/rpg-api/blob/c667ccc7d8dccef3cb5a98693baf1f639d07da50/internal/handlers/dnd5e/session/v1alpha1/convert.go)
- The Docker builder moves to golang 1.26, because the generated protos module requires Go 1.26 since v0.1.246 and the golang image builds with its own toolchain only · [rpg-api#1158](https://github.com/KirkDiggler/rpg-api/pull/1158)

## Found

- Every protos pin past v0.1.245 needs Go 1.26 in the builder image. rpg-api#1151 (Go 1.27) is the larger step · [rpg-api#1151](https://github.com/KirkDiggler/rpg-api/issues/1151), [rpg-api#1158](https://github.com/KirkDiggler/rpg-api/pull/1158)
- The repo had `knowledge/` but no `runs/` level; this record starts it · [rpg-api#1158](https://github.com/KirkDiggler/rpg-api/pull/1158)

## Mistaken

- A first `go get` named the protos root module; the api consumes `rpg-api-protos/gen/go` at the generated commit's pseudo-version. Caught before any commit; the first pushed pin is already the right one · [d104a7d](https://github.com/KirkDiggler/rpg-api/commit/d104a7d3e054baf7fc1c91c2ac826b7390ad1b4d)

— cross-team agent, on behalf of KirkDiggler
