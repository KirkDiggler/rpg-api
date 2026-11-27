# Run — 2026-11-28 — NPC behaviour 4d: a creature acts on its own hit points

Slice [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`e9606b8a`](https://github.com/KirkDiggler/rpg-api/commit/e9606b8a).

## Shipped

- Three pins in one commit: `mind/behavior` v0.28.0, `rulebooks/dnd5e/encounter` v0.267.0, `rulebooks/dnd5e/session` v0.221.0; `dnd5e` v0.244.0 and `resolution` v0.85.0 stay, the wire pin does not move — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- `internal/dungeons/hit_points_when_check_test.go` with the toolkit's `lieutenant-heals-when-hurt-walk.yaml` at session v0.221.0 as a test fixture under `internal/dungeons/testdata/hit-points-when/`; nothing under `content/` — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- `internal/integration/session/lieutenant_heals_when_hurt_acceptance_test.go`: unhurt (27 of 27) three rounds, no activate act reaches the fighter; hurt to 7 of 27, one activate act by him with one healing result (before 7, after 18, requested 11), none on the turn after — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- No production file changed, no existing test edited — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).

## Decided

- A refusal's second error is asserted as the one consequence (the recipe's pick finds no table), not left unsaid, so a third error fails the test — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- The unhurt scene rolls every d20 as 1 so nothing lands in three rounds; the hurt scene rolls d20 as 10 and d8 as 7 so each blow takes 10 and he stands at 17, then 7 — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).

## Found

- F1 holds: dev pins `mind/behavior` v0.26.0, `dnd5e` v0.244.0, encounter v0.265.0, resolution v0.85.0, session v0.219.0 — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1252#issuecomment-7229837832).
- F2 and F3 hold; the newest tags are `mind/behavior` v0.28.0, encounter v0.267.0, session v0.221.0 — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1252#issuecomment-7229837832).
- F4: `TestCastleCopiesAgree` passes with no edit; `TestLastWeeksCastleCompilesAsItDid` is not found by that name in `internal/notedwords` or anywhere in the module, and the suite after the pins passes whole — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1252#issuecomment-7229837832).
- F5a: the grep for the sentences listing a `when`'s conditions prints nothing at `origin/dev` — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1252#issuecomment-7229837832).
- F5b: six files use `NumField()` (authoring `machine_words_test`, `words_reflect_test`; session handler `hold_test`, `loot_test`, `speak_test`; integration `holdings_acceptance_test`); none fails after the pins — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1252#issuecomment-7229837832).
- F5c: no `_test.go` file in `internal` holds a 64-hex literal — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1252#issuecomment-7229837832).
- The four read-back sentences, at `behaviorDefinitions.lieutenant-policy.table.time[0..3]`: "if Lieutenant (lieutenant)'s hit points are at most 30% of its maximum, Lieutenant (lieutenant) activates “second wind”, when it has that ability and can pay for it."; "…hit points are at most 5, Lieutenant (lieutenant) moves away from the enemy."; "if NOT (…hit points are at most 5) and a walk this turn puts an enemy in Lieutenant (lieutenant)'s reach, … walks toward the enemy."; "if NOT (…hit points are at most 5) and an enemy is in … reach, … attacks the enemy." — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- Refusal `0%`: path `behaviorDefinitions.lieutenant-policy.table.line 1`, "`hit_points: { at_most: 0% }`: a share of the creature's own maximum is a whole percent from 1% to 100%" — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- Refusal `0`: same path, "`hit_points: { at_most: 0 }`: a number of hit points is 1 or more" — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- Refusal `low`: same path, "`hit_points: { at_most: low }`: write a whole number of hit points (`8`) or a whole percent of the creature's own maximum (`30%`)" — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- Each refusal comes with a second error at `room.room.monsterBindings.lieutenant.behaviors[0].use`: the recipe's pick finds "absent table \"time\"", because the table was not decoded; the issue expected exactly one error; the first is placed at the table with the line within the row's own text ("line 1" for the second row too), not at `table.time[N]` — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- The wire carries `Act{family ACT, activate{by "lieutenant", ability{ref "dnd5e:features:second_wind", name "Second Wind"}, results[{kind "healing-applied", target "lieutenant", amount 11, requested 11, before 7, after 18}]}}`; unhurt, rounds 1 to 3 carry walks, attacks and table answers and no activate act — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- Test 4 was possible: the fighter's own `Attack` and `EndTurn` verbs with scripted dice bring him to 17 then 7 of 27; the stored sheet is only read — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- At the old pins the two check tests fail ("`hit_points` is not a deed this build knows") and both wire tests fail at launch, because the site does not compile; the existing Second Wind scene still passes — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- Suite: `go build ./...` and `go test ./... -count=1` pass at `origin/dev` and after the pins; `make lint` reports the four issues in `zz_ue_spike_test.go` before and after — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
- The new tests run in 0.02 s (accepted), 0.01 s (refusals), 0.26 s (unhurt) and 0.35 s (hurt) — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).

## Mistaken

- The first unused helper `activationsBy` was left in the wire test file and `make lint` flagged it; removed before the push — [rpg-project#1252](https://github.com/KirkDiggler/rpg-project/issues/1252).
