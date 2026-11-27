# Run — 2026-11-27 — an NPC's turn is paid at the one gate S-last pins

Slice [rpg-project#1210](https://github.com/KirkDiggler/rpg-project/issues/1210) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`6572a43d`](https://github.com/KirkDiggler/rpg-api/commit/6572a43d3bf7b94cbf5e7fde1edd37bf698a6529).

## Shipped

- The api pins rulebook `v0.240.0`, encounter `v0.262.0`, resolver `v0.84.0`, session `v0.217.0` as one set; only `go.mod` and `go.sum` move — [rpg-project#1210](https://github.com/KirkDiggler/rpg-project/issues/1210).

## Decided

- The four pins move in one commit, because the new encounter with the old session tells every NPC it has a full turn and no NPC could run — [rpg-project#1210](https://github.com/KirkDiggler/rpg-project/issues/1210).

## Found

- F1: dev held `v0.239.0` / `v0.261.0` / `v0.83.0` / `v0.216.0` before the pins — [comment](https://github.com/KirkDiggler/rpg-project/issues/1210#issuecomment-7201517992).
- F2: the session `v0.217.0` go.mod requires rulebook `v0.240.0`, encounter `v0.262.0`, resolver `v0.84.0` — [comment](https://github.com/KirkDiggler/rpg-project/issues/1210#issuecomment-7201517992).
- F3: the grep for encounter actors and capabilities in `internal` and `cmd` printed nothing — [comment](https://github.com/KirkDiggler/rpg-project/issues/1210#issuecomment-7201517992).
- F4: `TestCastleCopiesAgree` and `TestLastWeeksCastleCompilesAsItDid` pass with no edit — [comment](https://github.com/KirkDiggler/rpg-project/issues/1210#issuecomment-7201517992).
- F5: the skeleton turns in the story-events, driver, interrupt and displacement acceptance tests still pass; none plays an NPC's run; the only Dash test is a player's — [comment](https://github.com/KirkDiggler/rpg-project/issues/1210#issuecomment-7201517992).
- F6: the grep for `attacks_left` and `bonus_actions"` printed nothing — [comment](https://github.com/KirkDiggler/rpg-project/issues/1210#issuecomment-7201517992).
- Suite before: `go build ./...` and `go test ./... -count=1` pass at dev. After: both pass, `make lint` prints 0 issues — [comment](https://github.com/KirkDiggler/rpg-project/issues/1210#issuecomment-7201517992).

## Mistaken

- none observed — [rpg-project#1210](https://github.com/KirkDiggler/rpg-project/issues/1210).
