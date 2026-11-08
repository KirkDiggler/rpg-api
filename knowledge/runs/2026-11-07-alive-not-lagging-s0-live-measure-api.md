# Run — 2026-11-07 — alive, not lagging S0 live measure: the answer and the first beat on the garrison keep

Live half of the measurement harness for story [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879) (S0, R5: the harness is the record), measuring R1 for slice [rpg-project#890](https://github.com/KirkDiggler/rpg-project/issues/890). Base: `rpg-api` `dev` at [`f890674`](https://github.com/KirkDiggler/rpg-api/commit/f890674). Built on session v0.169.0 and encounter v0.184.0.

## Shipped

- `TestMeasure_AnswerAndFirstBeat`: the v1alpha1 handler's Move on a real Redis over `bench-keep-garrison`, both answer modes, reporting answer, first beat, last beat, drain, acts per verb, broker lag and a Redis floor; skipped unless `RPG_MEASURE=1` — [internal/integration/session/measure_answer_first_beat_test.go](https://github.com/KirkDiggler/rpg-api/blob/feat/measure-answer-first-beat/internal/integration/session/measure_answer_first_beat_test.go).

## Decided

- The keep is `bench-keep-garrison`, not `bench-keep`, because the plain keep's monster forms a fight at launch, so its Move is a combat move and refuses a world-clock walk (empty declaration id) — [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879).
- The walk is 8 cells back and forth along one axial line from the file's seat, because a 30 ft walker pays a world round every six cells, so every measured Move crosses at least one pace — [rpg-project#890](https://github.com/KirkDiggler/rpg-project/issues/890).
- The run waits between Moves on its own count of continuations, not `Orchestrator.Drain`, because Drain is the shutdown drain on dev and refuses every later continuation — [rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197).
- Timing ends at the ActBroker channel, so transport and encoding are out of the number; the test's doc comment says so — [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879).

## Found

- At dev f890674, whole-answer Move p50 is 989 ms (p95 1280 ms), down from 1817 ms at d27ccfa (F15); the S1 answer p50 is 307 ms (p95 484 ms), down from 425 ms; the first beat p50 is 885 ms (p95 1150 ms), down from 1852 ms — [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879).
- Under S1 the first beat still lands with the last: a continued verb's acts reach the subscriber as one unit after its save — [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879).
- A garrison verb still delivers 165–320 acts to the walker, and the production broker buffer (64 commits, S10) holds them without lag over 30 Moves — [rpg-api#1201](https://github.com/KirkDiggler/rpg-api/pull/1201).
- Cost still grows with every Move: the S1 answer climbs from 139–263 ms to 357–522 ms and the whole Move from 599 to 1280 ms over 33 Moves — [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879).
- At d27ccfa a profile put 65% of a whole Move in the walk (sight refresh 44%) and 25% in the per-recipient fold over 100 members; folding for the player only cut p50 from 1429 to 1001 ms — [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879).

## Mistaken

- The first run on the merged #1197 failed: the test called `Orchestrator.Drain` between Moves, which became a one-way shutdown drain; the run now counts its own continuations — [rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197).
- One rerun on a machine at load 42 doubled every number; the published table is from a run at load 8–9.
