# Run — 2026-11-28 — second ability S5d-3 action surge is served

Slice [rpg-project#1267](https://github.com/KirkDiggler/rpg-project/issues/1267) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`40da1fc3`](https://github.com/KirkDiggler/rpg-api/commit/40da1fc3).

One line per entry. Every line ends with a link.

## Shipped

- The pins: `rulebooks/dnd5e` v0.248.0, `resolution` v0.86.0, `session` v0.223.0, protos `v0.0.0-20261128012435-d4dadb1f0734`; only those four `go.mod` lines move — [a01a4896](https://github.com/KirkDiggler/rpg-api/commit/a01a4896).
- A served value carries `NeedsCount: v.NeedsCount`, the one production line — [f9f2003b](https://github.com/KirkDiggler/rpg-api/commit/f9f2003b).
- Tests 2 to 5 and the fixture `internal/dungeons/testdata/action-surge/site.yaml` (the toolkit's `lieutenant-surges-walk.yaml` at session v0.223.0, not under `content/`) — [a85ae7d3](https://github.com/KirkDiggler/rpg-api/commit/a85ae7d3).
- Test 1, `TestAValueSaysWhetherItNeedsACount` in `item_words_test.go` — [f9f2003b](https://github.com/KirkDiggler/rpg-api/commit/f9f2003b).

## Decided

- `docs/architecture/components/authoring-service.md` is not edited: its item-words section does not list the fields of a served value — [the section](https://github.com/KirkDiggler/rpg-api/blob/f9f2003b/docs/architecture/components/authoring-service.md#the-item-words-getitemwords-rpg-project850).
- The misspelt-row test replaces `activate: action surge, weight: 9`, since `activate: action surge` is also written once in the fixture's header comment — [test](https://github.com/KirkDiggler/rpg-api/blob/f9f2003b/internal/dungeons/action_surge_check_test.go).
- The wire test reuses `scriptedDice` (d20 rolls 1, so nobody is hurt; the pick lands on the first eligible row) and `fighterEndsTurn` of the heals-when-hurt test — [test](https://github.com/KirkDiggler/rpg-api/blob/f9f2003b/internal/integration/session/lieutenant_surges_acceptance_test.go).

## Found

- F1 to F4 hold at `origin/dev`; F6 holds: the whole suite at the pins has one failure, the served-word guard, so the castle guards and the character tests pass unedited — [comment](https://github.com/KirkDiggler/rpg-project/issues/1267#issuecomment-7231913969).
- The guard's three observations: old protos with the new rulebook, `Words.Values.NeedsCount has no wire field "needs_count"`; the pins without the mapping, `Words.Values.NeedsCount` expected true, actual false; with the mapping it passes — [comment](https://github.com/KirkDiggler/rpg-project/issues/1267#issuecomment-7231913969).
- F7: no test pins the whole ability list; no `NumField()` guard walks `SpendProfile`, `kits.Value` (other than the words guard) or `actions.Definition`; `BankCapacity(` appears nowhere in rpg-api — [comment](https://github.com/KirkDiggler/rpg-project/issues/1267#issuecomment-7231913969).
- rpg-api-protos#456 is additive: `go build ./...` and `go vet ./...` pass at the pins — [a01a4896](https://github.com/KirkDiggler/rpg-api/commit/a01a4896).
- Read-back, row at `behaviorDefinitions.lieutenant-policy.table.time[0]`: "`time` row 1 of `lieutenant-policy`: if an enemy is in Lieutenant (lieutenant)'s reach, Lieutenant (lieutenant) activates “action surge”, when it has that ability and can pay for it (weight 9)." — [test](https://github.com/KirkDiggler/rpg-api/blob/f9f2003b/internal/dungeons/action_surge_check_test.go).
- Read-back, stat block at `templates.gate-lieutenant.lets`: "The stat block `gate-lieutenant` (Lieutenant) can use “action surge”, 1 use. What that is and how it is counted is written in its own words under `templates.gate-lieutenant.lets`, and the rules judge them. A behaviour's row uses one with `activate`." — [test](https://github.com/KirkDiggler/rpg-api/blob/f9f2003b/internal/dungeons/action_surge_check_test.go).
- Uncounted, at `templates.gate-lieutenant` on validate-only and save, one error and no knock-on: "template "gate-lieutenant": invalid encounter data: stat block "gate-lieutenant": lets[0]: `use` is "action surge", which gives its user something for its count alone; write `uses` or a `cost`"; nothing stored — [test](https://github.com/KirkDiggler/rpg-api/blob/f9f2003b/internal/dungeons/action_surge_check_test.go).
- Misspelt row, at `behaviorDefinitions.lieutenant-policy.table.time[0]`: "a table row says `activate: actoin surge`, and the rules describe no ability by that word; it is one of action surge, second wind"; nothing stored — [test](https://github.com/KirkDiggler/rpg-api/blob/f9f2003b/internal/dungeons/action_surge_check_test.go).
- The wire: the fighter's client is sent one `activate` act by `lieutenant`, ability ref `dnd5e:features:action_surge`, name `Action Surge`, no target, one result of kind `capacity-granted` described `1 action`; two strikes by him that turn, no `activate` and one strike on his next — [test](https://github.com/KirkDiggler/rpg-api/blob/f9f2003b/internal/integration/session/lieutenant_surges_acceptance_test.go).
- At the old pins (go.mod and the handler as at `origin/dev`) tests 2 to 5 fail with the rules' "`use` is "action surge"; it is one of second wind"; dropping the mapping fails Test 1 ("action surge must be paid from a count") and the guard (`Words.Values.NeedsCount`) — [a01a4896](https://github.com/KirkDiggler/rpg-api/commit/a01a4896).
- The new tests take 0.00s to 0.19s each; the suite is green before (62s) and after (rc 0, 44 packages ok); `make lint` reports the same four issues, all in `zz_ue_spike_test.go`, none in this slice's files — [comment](https://github.com/KirkDiggler/rpg-project/issues/1267#issuecomment-7231913969).
- Production files changed: one, `internal/handlers/dnd5e/authoring/v1alpha1/get_item_words.go`, to copy the value's `NeedsCount` onto the wire — [f9f2003b](https://github.com/KirkDiggler/rpg-api/commit/f9f2003b).

## Mistaken

- The first draft of the misspelt-row test replaced text written twice and failed its own precondition; the replacement was widened to the row's `weight: 9` — [f9f2003b](https://github.com/KirkDiggler/rpg-api/commit/f9f2003b).
