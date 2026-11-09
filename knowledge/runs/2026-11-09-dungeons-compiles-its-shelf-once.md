# Run — 2026-11-09 — the dungeons package compiles its shelf once; the test budget runs on rpg-api

Issue [rpg-project#911](https://github.com/KirkDiggler/rpg-project/issues/911), story [rpg-project#907](https://github.com/KirkDiggler/rpg-project/issues/907) slice 4. Cut from `rpg-api` `dev` at [`109251f`](https://github.com/KirkDiggler/rpg-api/commit/109251f), merged up to [`e763358`](https://github.com/KirkDiggler/rpg-api/commit/e763358) (rpg-api#1210). PR [rpg-api#1212](https://github.com/KirkDiggler/rpg-api/pull/1212).

## Shipped

- `internal/dungeons` compiles its content shelf once per test binary. `TestMain` stages `content/`, the retired walk sites and the castle kitchen, and compiles them with the new `dungeonstest.CompileShelf`. The stores, single-room, creature-facts and reference-keep suites join that store with a fresh world store per test. The atlas row reads it. — this PR
- The bench keep fixture tests (`TestBenchFixtures_*`: 10k, 30k, garrison) run under `RPG_MEASURE=1`, beside the measure tests that use those keeps. — this PR
- `scripts/test-budget.sh` and its cases, copied from the toolkit's slice 3 (rpg-toolkit#2341) with the same interface, run in `.github/workflows/test.yml`: the suite runs with `-json`, the ten slowest top-level tests go to the job summary, and a test over 5 s under `-race` fails the job unless `testbudget.allow` names it with a reason. — this PR

## Decided

- The shared shelf compiles in `TestMain`, eagerly, so its cost lands on no test and the budget does not depend on which suite asks first. A focused `-run` pays one compile. — this PR
- The shelf directory outlives each test. A test that proves a save never writes it compares against the files as staged, never a before-snapshot of its own: an earlier save of the same bytes hides a write from a snapshot. `TestMain` fails the binary when any test changed the shelf. — this PR
- Tests about construction (a broken file, a missing default, a misnamed key, the seed) keep building their own registry over their own directory. — this PR
- The script is copied, not shared. Switching to a shared copy later is one path in the workflow. — this PR
- The budget step runs last in the job, so a test over budget never skips the coverage report and PR comment. — this PR
- The bench keep check runs in CI as its own step, without `-race` (the generator is deterministic and the check is byte equality) and outside the `-json` stream the budget judges. It took about 12 s locally. — this PR (review)
- No assertion was removed. — this PR

## Measured

Local, the same machine, `uptime` load 4–5 for the before run and 3 for the after run.

```
go test -race -json -count=1 ./internal/dungeons/...
```

| | before (`109251f`) | after (`1d6c267`) |
|---|---|---|
| `internal/dungeons` package | 199.1 s (wall 210 s) | 24.1 s (wall 26 s) |

Slowest top-level tests, before:

| test | seconds |
|---|---|
| TestStoresSuite | 82.09 |
| TestBenchFixtures_MatchGenerator | 39.45 |
| TestSingleRoomRegistrySuite | 23.99 |
| TestCreatureFactsWalkSuite | 19.26 |
| TestRegistrySuite | 15.18 |
| TestReferenceKeepSuite | 11.89 |
| TestBenchFixtures_ListOnAShelf | 6.10 |

After:

| test | seconds |
|---|---|
| TestRegistrySuite | 10.14 |
| TestReferenceKeepSuite | 7.18 |
| TestStoresSuite | 0.52 |
| TestCreatureFactsWalkSuite | 0.17 |
| TestCastleCopiesAgree | 0.14 |
| TestBenchFixtures_* | skipped without `RPG_MEASURE=1` |

With `RPG_MEASURE=1` (no `-race`) the bench tests pass: `MatchGenerator` 10.7 s (the 30k keep 8.5 s), `ListOnAShelf` 1.2 s.

Mutation checks:

- A save that also writes the shelf directory fails `TestStoresSuite/TestPutWritesTheWorldStoreAndNotTheDirectory` and the `TestMain` guard. Before the staged comparison, this mutation passed the stores suite: the finding behind the guard.
- A `Put` that no longer refuses a shipped key fails `TestStoresSuite/TestPutUnderAShippedKeyIsTheReferenceRefusal` and `TestReferenceKeepSuite`.

## Allowlist

Sized from CI's own timing, as the budget is judged there: every top-level test over 5 s in the test.json of CI run [42695344817](https://github.com/KirkDiggler/rpg-api/actions/runs/42695344817) (the `test-json` artifact the workflow now uploads), 29 names. Two are in `internal/dungeons` and carry reasons:

- `TestRegistrySuite` (17.4 s on CI): 38 cases in one suite; the seed row compiles the shipped content it seeded, the atlas row reads every shelf atlas.
- `TestReferenceKeepSuite` (12.3 s on CI): saves and reads back a copy of the 130 KB reference keep, two compiles of the keep the claim is about.

Six are 2.6 to 4.7 s locally and 5.0 to 6.5 s on CI: "over 5 s on CI runners (~1.5x local); rpg-api#1213". The other 21 say "over budget, to be sized: rpg-api#1213". The largest on CI: `TestCharacterCreationSuite` (integration/character) 149 s, `TestSessionStackSuite` (orchestrators/lobby) 93 s, `TestValidateOnlyWireSuite` (authoring handlers) 55 s.

## Budget proof

- Red: [run 42690637523](https://github.com/KirkDiggler/rpg-api/actions/runs/42690637523) at `6d3c486`, a temporary `TestBudgetProbe_SleepsSixSeconds` in `internal/dungeons`. The budget step failed naming it alone, "took 6s; the budget is 5s", with an error annotation; every other test was within the budget or allowed.
- Green: [run 42692257625](https://github.com/KirkDiggler/rpg-api/actions/runs/42692257625) at `92451e4`, the probe reverted and the near-budget tests allowed: "every top-level test in rpg-api is within 5s or allowed". The coverage steps and PR comment ran before it. Its `Run tests with coverage` step took 3 min.

## Found

- CI runs a test up to 1.6 times its local time, so a test at 4 s locally flaps on the 5 s budget. Tests at 3 to 5 s on CI can still flip from run to run.
- An allowlist name matches that test in every package: `TestTemplatesSuite` is allowed in `orchestrators/authoring` (10.4 s) and so also in `sessionworld` (4.0 s).
- Whole-suite local time with CI's flags was 234 s wall before rpg-api#1210 merged. `orchestrators/lobby` (61 s) and `integration/character` (137 s package) are now the largest packages.

— cross-team agent, on behalf of KirkDiggler
