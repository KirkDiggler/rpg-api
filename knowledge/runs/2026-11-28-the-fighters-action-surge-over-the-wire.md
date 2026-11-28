# Run — 2026-11-28 — second ability S6d the fighter's action surge over the wire

Slice [rpg-project#1272](https://github.com/KirkDiggler/rpg-project/issues/1272) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`5ae70188`](https://github.com/KirkDiggler/rpg-api/commit/5ae70188).

One line per entry. Every line ends with a link.

## Shipped

- The server's pins move to rules `v0.249.0`, resolver `v0.86.1` and session `v0.223.1`, with no production change — [d1a05f8b](https://github.com/KirkDiggler/rpg-api/commit/d1a05f8b).
- The two fixtures that saved Action Surge's count on the feature save it in the fighter's pool — [5e0ee2d1](https://github.com/KirkDiggler/rpg-api/commit/5e0ee2d1).
- Her own Action Surge is offered, used, struck twice on and spent over the wire, in four tests — [2f180b4d](https://github.com/KirkDiggler/rpg-api/commit/2f180b4d).
- The seeded level-up fighter takes level 2 through the production RPC and arrives with the pool at 1 of 1, in `sandboxseed_test.go` beside the seed's other level-up tests — [ad67fd7f](https://github.com/KirkDiggler/rpg-api/commit/ad67fd7f).

## Decided

- Test 5 lives in `SandboxSeedSuite` (`internal/integration/character/sandboxseed_test.go`), because the seeded `level-up-fighter` is held there and no integration test plays `LevelUp` yet — [issue Design 4](https://github.com/KirkDiggler/rpg-project/issues/1272).
- The fight is scripted with one dice type (every d20 rolls 10, every d8 rolls 1), so alice acts first as in the Second Wind scene, her blows hit armor 13 and the skeleton stays standing after two — [fighter_action_surge_acceptance_test.go](https://github.com/KirkDiggler/rpg-api/blob/feat/the-fighters-action-surge-over-the-wire/internal/integration/session/fighter_action_surge_acceptance_test.go).
- No test builds a feature carrying `uses` or `max_uses`; a fighter saved before the change is not played — [operator ruling 2026-11-28](https://github.com/KirkDiggler/rpg-project/issues/1131#issuecomment-7232312437).

## Found

- After the pins alone, `go build ./...` passes and exactly two packages fail to compile their tests, `internal/handlers/dnd5e/v2/character` (handler_test.go:122) and `internal/orchestrators/character` (view_test.go:264), both `unknown field Uses` and `MaxUses`; every other package passes — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1272#issuecomment-7232788772).
- The compile error hid a runtime failure: `TestProjectView_AMonksEquipmentCarriesTheWisdomInclusiveAC` builds a monk from the fighter fixture and the monk's view refuses `action_surge` ("not in the monk status-view owner catalog"); case (a), one `delete` beside the existing Second Wind one, no assertion changed — [second comment](https://github.com/KirkDiggler/rpg-project/issues/1272#issuecomment-7232792161).
- The two fixtures' own Action Surge assertions (`handler_test.go:431-433`, `view_test.go:47-56`) hold unedited — [second comment](https://github.com/KirkDiggler/rpg-project/issues/1272#issuecomment-7232792161).
- The wire gives a free action `SLOT_NONE`; the offer is named Action Surge with ref `dnd5e:features:action_surge`, available, no `why` — [Test 1](https://github.com/KirkDiggler/rpg-api/blob/feat/the-fighters-action-surge-over-the-wire/internal/integration/session/fighter_action_surge_acceptance_test.go).
- The acts of Test 2 are one act, family `FAMILY_ACT`, an activation by `alice`, ability `dnd5e:features:action_surge` named "Action Surge", no target, one result of kind `capacity-granted` described "1 action"; her stored pool is 0 of 1 and her actions remaining 2 — [Test 2](https://github.com/KirkDiggler/rpg-api/blob/feat/the-fighters-action-surge-over-the-wire/internal/integration/session/fighter_action_surge_acceptance_test.go).
- A third attack is refused with `Aborted`, "attack: declaration is stale", on a dark attack row; a second activation is refused with `Aborted`, "activate: declaration is stale", and the offer says `SHORTFALL_REASON_NO_BUDGET`, `CURRENCY_CHARGES`, "no action surge uses remaining", with her pool and economy unchanged — [Tests 3 and 4](https://github.com/KirkDiggler/rpg-api/blob/feat/the-fighters-action-surge-over-the-wire/internal/integration/session/fighter_action_surge_acceptance_test.go).
- The level-up over the wire reports gained features `[Action Surge]` and resource changes `action_surge 0 to 1` and `hit_dice 1 to 2`; the served sheet (`GetCharacterData`) then holds `action_surge` at 1 of 1 and held none before — [Test 5](https://github.com/KirkDiggler/rpg-api/blob/feat/the-fighters-action-surge-over-the-wire/internal/integration/character/sandboxseed_test.go).
- The rules pin cannot go back to `v0.248.0` alone: session `v0.223.1` requires rules `v0.249.0`, and `go build` then wants go.sum entries the graph will not hold — [session v0.223.1 go.mod](https://github.com/KirkDiggler/rpg-toolkit/blob/rulebooks/dnd5e/session/v0.223.1/rulebooks/dnd5e/session/go.mod).
- The issue's substitute proof (her pool at 0 of 1 in the fixture) does not fail: all four tests still pass, because a launch is a long rest that refills the pool before the fight; the proof that bites removes her pool from the saved resources, and all four then fail at launch with `long rest "alice": feature action-surge reported malformed status: owner does not carry action_surge` — [proof (a)](https://github.com/KirkDiggler/rpg-project/issues/1272).
- Proof (b), a third strike expected in Test 3, fails with `verb VERB_ATTACK must be available: ... action: 1 needed, 0 left` — [proof (b)](https://github.com/KirkDiggler/rpg-project/issues/1272).
- Test times: Test 1 0.01s, Test 2 0.09s, Test 3 0.10s, Test 4 0.02s, Test 5 1.03s — [final run](https://github.com/KirkDiggler/rpg-project/issues/1272).
- Suite: `go build ./... && go vet ./... && go test ./... -count=1 -timeout 15m` passes at origin/dev; after the pins alone it fails in the two packages above; at the final head it passes; `make lint` reports only the four known issues in `zz_ue_spike_test.go` — [rpg-project#1272](https://github.com/KirkDiggler/rpg-project/issues/1272).

## Mistaken

- The issue's proof (a) fallback does not bite and was replaced by removing the pool; the fixture's `Current` is not load-bearing, because the launch's long rest sets it — [rpg-project#1272](https://github.com/KirkDiggler/rpg-project/issues/1272).
