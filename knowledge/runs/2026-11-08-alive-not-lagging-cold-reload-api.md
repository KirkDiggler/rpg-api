# Run — 2026-11-08 — alive, not lagging: the cold reload of a session, by stage

Cold-reload row of the measurement harness for story [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879), in service of ruling R6 on [rpg-project#880](https://github.com/KirkDiggler/rpg-project/issues/880): the resident encounter becomes a cache behind a Redis lease, and a miss rebuilds from the store. This run says what that rebuild costs, by stage, against the Move it sits inside.

Measured at `rpg-api` `dev` [`85c8bcc`](https://github.com/KirkDiggler/rpg-api/commit/85c8bcc) plus the harness change on `feat/measure-cold-reload`. Built on session v0.171.0 and encounter v0.185.0. Keep `bench-keep-garrison` (10k hexes, 99 garrison, 20 patrols, neutral to the party), an 8-cell walk, N=20 after 3 warm-up Moves per mode. After each mode's walk, the same session is rebuilt 20 times from the store. Redis 7.2.2 on a private loopback port. CPU AMD Ryzen 9 3950X, load average 9 to 12. Command:

```
redis-server --port 6497 --save '' --daemonize yes
RPG_MEASURE=1 RPG_MEASURE_REDIS=127.0.0.1:6497 RPG_MEASURE_N=20 go test ./internal/integration/session/ -run TestMeasure_AnswerAndFirstBeat -count=1 -v -timeout 60m
```

## Shipped

- The reload row: after each mode's walk, `measureColdReload` rebuilds the walked session N times outside any verb, with the functions the session's load path calls, and prints a stage table beside the Move totals. `RPG_MEASURE_RELOAD=0` skips it — [internal/integration/session/measure_cold_reload_test.go](https://github.com/KirkDiggler/rpg-api/blob/feat/measure-cold-reload/internal/integration/session/measure_cold_reload_test.go).

## The table

Bytes are the stored blob after the 23rd Move of the mode. Stages run in sequence, one rebuild at a time.

| stage | bytes | AnswerWhole p50 | p90 | S1 continues p50 | p90 |
|---|---|---|---|---|---|
| session record fetch (GET) | 83 194 | 0.33 ms | 0.76 ms | 0.31 ms | 0.47 ms |
| session record decode | | 1.23 ms | 2.04 ms | 1.15 ms | 1.83 ms |
| 1 fetch: `session-enc` blob GET | 9 096 879 | 13.24 ms | 15.38 ms | 12.45 ms | 15.80 ms |
| 2 decode: JSON → `EncounterData` | | 62.76 ms | 69.02 ms | 60.78 ms | 67.80 ms |
| 1+2 as one: repository `GetEncounter` | | 75.75 ms | 83.16 ms | 74.40 ms | 83.16 ms |
| 3 site build: `encounter.LoadEncounter`, refusing capabilities | | 75.08 ms | 79.06 ms | 73.16 ms | 82.14 ms |
| 4 attach: read load − record − 1 − 2 − 3 | | 7.34 ms | 21.68 ms | 9.07 ms | 27.79 ms |
| read load whole: `Manager.Where` | | 159.30 ms | 176.62 ms | 160.08 ms | 187.28 ms |
| 5a `ToData` | | 8.27 ms | 10.05 ms | 7.93 ms | 10.80 ms |
| 5b JSON encode | 9 096 879 | 58.41 ms | 64.02 ms | 57.91 ms | 67.99 ms |
| 5c persist (SET, scratch key) | 9 096 879 | 7.40 ms | 9.59 ms | 7.52 ms | 9.24 ms |
| Move answer (the walk above) | | 889.65 ms | 1221.61 ms | 255.91 ms | 347.03 ms |
| Move drained (the walk above) | | 889.69 ms | 1221.64 ms | 914.85 ms | 1215.28 ms |

## Decided

- Fetch and decode are timed apart by doing the repository's two steps in the harness, a GET on the same key and the same `json.Unmarshal`; the repository's own `GetEncounter` is timed beside them as a cross-check, and it agrees.
- The site build is `encounter.LoadEncounter` with `encounter.RefusingCapabilities`, because the composition's load never consults a capability, so it isolates the field compile, doors and member reconstitution from every session seam.
- The attach stage is a residual of `Manager.Where`, the cheapest exported verb that runs the session's whole read-path load, because the session's `standingFor`, capability builders and `prepareExploration` are unexported. No hook was added to rpg-toolkit.
- The write side writes a scratch key, so the session the harness walked is never rewritten by the measurement.

## Found

- A cold read-path reload of the garrison keep costs 159 ms at p50, about 18% of the 890 ms whole Move. Decode and site build are about 62 and 75 ms each; the Redis GET of a 9.1 MB blob is 13 ms.
- The write side costs about 74 ms at p50: `ToData` 8 ms, JSON encode 58 ms, SET 7 ms. JSON is 121 ms of every load-and-save pair, more than the field compile.
- The blob grows with the record: 3.7 MB after 6 Moves in a smoke run, 9.1 MB after 23. Decode, encode and fetch scale with it, so a reload gets slower as a session ages.
- The attach residual on the read path is small, 7 to 9 ms at p50. The write-path opener adds work this row does not see; see the caveats.
- Load plus save is about 230 ms of a 890 ms Move. The other roughly 650 ms is the walk and the fold, which a resident encounter does not remove.

## Caveats

- The write-path opener, `openForChange`, adds the ledger load, write capabilities, `prepareExploration`'s player sheet staging and the act open on top of the read load. Those are reachable only through a verb, so they are not in this row.
- The attach residual includes the session lock and the driver resolution, and it inherits the noise of four subtracted stages, so its p90 is loose.
- The SET is on loopback, so a networked Redis would add transfer time for 9 MB.
