# Run — 2026-11-28 — second ability S8f rage is a word content can write

Slice [rpg-project#1293](https://github.com/KirkDiggler/rpg-project/issues/1293) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`c908d727`](https://github.com/KirkDiggler/rpg-api/commit/c908d727).

One line per entry. Every line ends with a link.

## Shipped

- The server's pins move to rules `v0.253.0`, resolver `v0.88.0` and session `v0.224.0` in one commit, with no production change — [e112e5a9](https://github.com/KirkDiggler/rpg-api/commit/e112e5a9).
- Two test-data sites, `testdata/rage/site.yaml` (a Lieutenant who rages) and `testdata/rage-item/site.yaml` (a cloak that lets its wearer rage) — [issue Design 2](https://github.com/KirkDiggler/rpg-project/issues/1293).
- Seven check tests in `internal/dungeons/rage_check_test.go`, one served-word test in `item_words_test.go`, one acceptance test in `lieutenant_rages_acceptance_test.go` — [issue Tests 1 to 6](https://github.com/KirkDiggler/rpg-project/issues/1293).

## Decided

- `action_surge_check_test.go:113` gains `rage` in its sorted place, `action surge, rage, second wind`, keeping its `Contains` — [allowed edit](https://github.com/KirkDiggler/rpg-project/issues/1293).
- The missing-`with` refusal is asserted on the word `damage_bonus`, which the rules' sentence carries in its example `{"damage_bonus": 2}` — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1293#issuecomment-7236558739).
- The creature is played with every d20 at 10 and every d8 at 7, so a blow lands on either side and he starts at 9 hit points; the fighter's blow is 7 and her 3, 10 slashing, and 5 as sent — [acceptance test](https://github.com/KirkDiggler/rpg-api/blob/feat/rage-is-a-word-content-can-write/internal/integration/session/lieutenant_rages_acceptance_test.go).

## Found

- Suite at the baseline: 44 packages ok, none failed; after the three pins alone: exactly one failure, `TestPutRefusesARowThatMisspellsActionSurgeAndOffersTheAbilities`; at the final head: 44 ok, none failed — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1293#issuecomment-7236558739).
- F3: only `action_surge_check_test.go:113` pins the list; no `NumField()` guard walks `monster.Ability` (they walk the machine-words table, `Connection` and response protos); no test uses Rage as an unknown ability — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1293#issuecomment-7236558739).
- The served label of the value is `Rage`; its one word `damage_bonus` is required, min 1, no max, labelled `adds {value} to the damage of each melee weapon hit made with strength` — [item_words_test.go](https://github.com/KirkDiggler/rpg-api/blob/feat/rage-is-a-word-content-can-write/internal/handlers/dnd5e/authoring/v1alpha1/item_words_test.go).
- Read-back row: `` `time` row 1 of `lieutenant-policy`: Lieutenant (lieutenant) activates “rage”, when it has that ability and can pay for it. `` (role `activates`); stat block: `` The stat block `gate-lieutenant` (Lieutenant) can use “rage”, 2 uses. What that is and how it is counted is written in its own words under `templates.gate-lieutenant.lets`, and the rules judge them. A behaviour's row uses one with `activate`. `` (role `lets`) — [rage_check_test.go](https://github.com/KirkDiggler/rpg-api/blob/feat/rage-is-a-word-content-can-write/internal/dungeons/rage_check_test.go).
- By-level refusal, one error with no knock-on, validate-only and save alike, at `templates.gate-lieutenant`: `template "gate-lieutenant": invalid encounter data: stat block "gate-lieutenant": lets[0] `with` for "rage": rage damage_bonus is "barbarian level"; it is a whole number of at least 1`; at the item, path `customItems.lieutenants-cloak`: `item "lieutenants-cloak": lets[0] `with` for "rage": rage damage_bonus is "barbarian level"; it is a whole number of at least 1` — [rage_check_test.go](https://github.com/KirkDiggler/rpg-api/blob/feat/rage-is-a-word-content-can-write/internal/dungeons/rage_check_test.go).
- With the `with` line removed, at both places: `lets[0] `with` for "rage": rage requires parameters, for example {"damage_bonus": 2}` — [rage_check_test.go](https://github.com/KirkDiggler/rpg-api/blob/feat/rage-is-a-word-content-can-write/internal/dungeons/rage_check_test.go).
- A misspelt row `activate: rge` is refused at `behaviorDefinitions.lieutenant-policy.table.time[0]`: `a table row says `activate: rge`, and the rules describe no ability by that word; it is one of action surge, rage, second wind` — [rage_check_test.go](https://github.com/KirkDiggler/rpg-api/blob/feat/rage-is-a-word-content-can-write/internal/dungeons/rage_check_test.go).
- The activation as sent: `Act{family FAMILY_ACT, activate{by "lieutenant", ability{ref "dnd5e:features:rage", name "Rage"}, results[{kind "condition-applied", target "lieutenant", ref "dnd5e:conditions:raging", name "Raging"}]}}`, one act, no target — [acceptance test](https://github.com/KirkDiggler/rpg-api/blob/feat/rage-is-a-word-content-can-write/internal/integration/session/lieutenant_rages_acceptance_test.go).
- His longsword strike carries a `condition` damage component from `dnd5e:conditions:raging` with modifier value 2 and total 2, in `AttackAct.damageComponents`; damage sent 9 (7 and 2) on both of his turns; the fighter's blow is 10 slashing and the damage sent is 5, his 9 hit points standing at 4 — [acceptance test](https://github.com/KirkDiggler/rpg-api/blob/feat/rage-is-a-word-content-can-write/internal/integration/session/lieutenant_rages_acceptance_test.go).
- At the old pins (go.mod and go.sum from `origin/dev`, restored after) all nine new tests fail: the stat block and the item are refused with `` `use` is "rage"; it is one of action surge, second wind ``, the word list has no `rage`, and the launch of the site fails to compile — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1293#issuecomment-7236558739).
- `make lint`: four issues, all in `internal/integration/session/zz_ue_spike_test.go`, as on `origin/dev`; the files of this slice add none — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1293#issuecomment-7236558739).
- Every new test runs under 0.3 seconds — [acceptance test](https://github.com/KirkDiggler/rpg-api/blob/feat/rage-is-a-word-content-can-write/internal/integration/session/lieutenant_rages_acceptance_test.go).

## Mistaken

- The first `item_words_test.go` addition used an unsized slice and the linter said so (`prealloc`); sized before the commit — [lint run](https://github.com/KirkDiggler/rpg-project/issues/1293).
