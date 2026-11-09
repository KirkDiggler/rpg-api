# Run — 2026-11-09 — alive, not lagging: what one live walker step costs

Per-step row of the measurement harness for story [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879), finding F23: why a live walker step costs about twice the encounter benchmark's walk step, by bucket, and what a verb pays once against what each step pays.

Measured at `rpg-api` `dev` [`109251fc`](https://github.com/KirkDiggler/rpg-api/commit/109251fc) plus the harness change on `measure/walker-step`. Built on session v0.172.1, encounter v0.185.0, play/record v0.3.0. Keep `bench-keep-garrison` (10k hexes, 99 garrison, 20 patrols, posts listening, neutral to the party), S1 continues mode, 3 warm-up Moves then N measured, Redis 7.2.2 on a private loopback port, CPU AMD Ryzen 9 3950X. Command, once per path length:

```
redis-server --port 6497 --save '' --dir <empty dir> --daemonize yes
RPG_MEASURE=1 RPG_MEASURE_REDIS=127.0.0.1:6497 RPG_MEASURE_N=20 RPG_MEASURE_MODES=continues RPG_MEASURE_RELOAD=0 RPG_MEASURE_PATH=8 \
  go test ./internal/integration/session/ -run TestMeasure_AnswerAndFirstBeat -count=1 -v -timeout 60m -cpuprofile cpu.out
```

## Shipped

- `RPG_MEASURE_PATH` sets the walk's length in cells (default 8), so runs at 1, 4 and 8 cells separate the per-verb cost from the per-step cost.
- A per-step table after the main one: the answer p50 grouped by steps answered and by whether the answer continued, and a least-squares line over the continued Moves, answer = fixed + steps × per step.

## The table

A continued S1 answer is the load and the walker's own steps up to the step that pays a round; nothing is saved. An answer that did not continue also carries the save and the commit, so it is not in the line.

| path | load avg | steps answered, continued: answer p50 | fit: fixed per verb | fit: per step |
|---|---|---|---|---|
| 8 cells, N=20 | 4.6 | 1: 120 ms · 3: 195 ms · 5: 272 ms | 84 ms | 37.4 ms |
| 8 cells, N=30 | 12 | 1: 129 ms · 3: 216 ms · 5: 299 ms | 90 ms | 42.3 ms |
| 4 cells, N=30 | 4 | 1: 109 ms · 3: 185 ms (4 steps, not continued: 256 ms) | 71 ms | 38 ms |
| 1 cell, N=30 | 4 | 0: 63 ms (1 step, not continued: 140 ms) | - | - |

## Found

- A live non-raising step costs 37 to 42 ms of wall time. A verb pays 63 ms before its first step, of which the open is 68 ms of CPU: 20 for the fenced fetch and JSON decode, 46 for `encounter.LoadEncounter`.
- Of 42 ms of CPU per answered step, the walker's own sight refresh is 32 ms and the session's mover, standing and pace seams are 9 ms. The walker's sweep of the floor from its new cell, cast fresh, is 17.5 ms of it.
- The mover seam builds `WorldView`, a full snapshot of the encounter including the behavior runtime and the record, once per step, 5.6 ms.

## Caveats

- Per-bucket milliseconds are CPU from the profile divided by about 71 answered steps (62 measured and about 9 warm-up); the profile also covers the drained continuations, which are filtered out by focusing on `startWalk`.
- The load average moved between 4 and 12 during the runs, which moves the per-step line by about 5 ms.
