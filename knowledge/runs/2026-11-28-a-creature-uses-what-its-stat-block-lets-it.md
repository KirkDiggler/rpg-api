# Run — 2026-11-28 — NPC behaviour 3d, a creature uses what its stat block lets it

Slice [rpg-project#1241](https://github.com/KirkDiggler/rpg-project/issues/1241) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`bea2db4a`](https://github.com/KirkDiggler/rpg-api/commit/bea2db4ad8a71c0597a00644aa0071badbb627a9).

## Shipped

- The five pins: behavior v0.26.0, rulebook v0.244.0, encounter v0.265.0, resolution v0.85.0, session v0.219.0; only those five requirement lines move — [commit](https://github.com/KirkDiggler/rpg-api/commit/e3e97f567ce6a580e8b5166cf5969d4aff41355b).
- A save refuses a row whose `activate` word is no ability, at the row, in `contentRefusals` — [commit](https://github.com/KirkDiggler/rpg-api/commit/223404313c81d125e7a3ec850226da20ef425096).
- Tests 1 to 3 over the fixture `internal/dungeons/testdata/activate-row/site.yaml` (the toolkit's lieutenant-uses-second-wind-walk at session v0.219.0, a test fixture, nothing under `content/`) — [commit](https://github.com/KirkDiggler/rpg-api/commit/223404313c81d125e7a3ec850226da20ef425096).
- Test 5, a launched Lieutenant uses Second Wind and the fighter's client is sent it — [commit](https://github.com/KirkDiggler/rpg-api/commit/40dbfc297c1315b1bba357b59ee4711a73b175f9).

## Decided

- The read-back is made only when the session reports an activation refusal, by `activationReadBack`, which calls `readback.Of` under a `recover`; a failure or panic is logged and yields no places, so each refusal falls to one path-less error (the issue's Design 2) — [issue](https://github.com/KirkDiggler/rpg-project/issues/1241).
- Places come in the read-back's reading order (the file's), distinct, because a lexical sort would put `time[10]` before `time[2]` — [commit](https://github.com/KirkDiggler/rpg-api/commit/223404313c81d125e7a3ec850226da20ef425096).
- Test 5 rolls the table's weighed pick (a d200) as 1 and every other die at its maximum, so the first eligible row, the activation, is the one picked — [commit](https://github.com/KirkDiggler/rpg-api/commit/40dbfc297c1315b1bba357b59ee4711a73b175f9).

## Found

- F1 to F5 hold; origin/dev is now `bea2db4a`, so the issue's line numbers moved and nothing else — [comment](https://github.com/KirkDiggler/rpg-project/issues/1241#issuecomment-7228100133).
- F6: at the pins alone, the misspelt word is accepted on validate-only AND on save (`errors=[] entry=true`); the session's refusal is not reported until `contentRefusals` handles `Activations` — [comment](https://github.com/KirkDiggler/rpg-project/issues/1241#issuecomment-7228100133).
- F8: no existing test pins the retired sentence, a list of row words or a walked type; the suite passes at the pins with no failure; no existing test is edited — [comment](https://github.com/KirkDiggler/rpg-project/issues/1241#issuecomment-7228100133).
- F10: the harness is `internal/integration/session` (`newAcceptanceHarnessWithDice`, `sessionworld.Compile`, `launchAtStart`, `EndTurn`, `watchActs`) — [comment](https://github.com/KirkDiggler/rpg-project/issues/1241#issuecomment-7228100133).
- The wire carries his activation as one `Act` (family ACT) with `activate{by "lieutenant", ability{ref dnd5e:features:second_wind, name Second Wind}, no target, results[{healing-applied, target lieutenant, requested 11, amount 0, before 9, after 9}]}`, the shape a player's own is sent in; live equals GetStory — [commit](https://github.com/KirkDiggler/rpg-api/commit/40dbfc297c1315b1bba357b59ee4711a73b175f9).
- The refusal: `a table row says `activate: secnd wind`, and the rules describe no ability by that word; it is one of second wind`, at `behaviorDefinitions.lieutenant-policy.table.time[0]` (and `[3]` when a fourth row says it) — [commit](https://github.com/KirkDiggler/rpg-api/commit/223404313c81d125e7a3ec850226da20ef425096).
- Test 4 (the path-less fallback) is not written: it needs an internal-package test file and the issue lists none; no authored v5 file reaches a path-less refusal (`monsterBindings.<id>.table` is a string) — [gap](https://github.com/KirkDiggler/rpg-project/issues/1241#issuecomment-7228126721).
- Proof (a), the Activations handling off: Tests 2 and 3 fail with no error reported (`"[]" should have 1 item(s), but has 0`, and 2). Proof (b), every path empty: both fail on the path (expected `behaviorDefinitions.lieutenant-policy.table.time[0]`, actual empty). Proof (c) is not possible without Test 4 — [commit](https://github.com/KirkDiggler/rpg-api/commit/223404313c81d125e7a3ec850226da20ef425096).
- `make lint` reports 4 issues, all in `internal/integration/session/zz_ue_spike_test.go`, a file already on origin/dev and not touched here; none in this change — [comment](https://github.com/KirkDiggler/rpg-project/issues/1241#issuecomment-7228100133).
- Suite: `go test ./... -count=1` exit 0 before the pins, at the pins and at the head; `TestCastleCopiesAgree` and `TestLastWeeksCastleCompilesAsItDid` pass with no edit. `authoring-service.md` does not list what a save refuses, so it is not edited — [commit](https://github.com/KirkDiggler/rpg-api/commit/40dbfc297c1315b1bba357b59ee4711a73b175f9).

## Mistaken

- My first Test 5 rolled the pick as a d100, but it is a d200, so the Lieutenant walked and no activation was sent; I read the table answer's candidates and `of 200` and fixed the die — [commit](https://github.com/KirkDiggler/rpg-api/commit/40dbfc297c1315b1bba357b59ee4711a73b175f9).
