# Run — 2026-11-08 — the Redis save is fenced on a checkpoint version

processed: 2026-11-12T165555Z

Story [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879), card [rpg-project#897](https://github.com/KirkDiggler/rpg-project/issues/897) (S12a, the api half). The toolkit half is [rpg-toolkit#2322](https://github.com/KirkDiggler/rpg-toolkit/pull/2322), tagged `rulebooks/dnd5e/session/v0.172.0`.

Built on `rpg-api` `dev` [`5711f56`](https://github.com/KirkDiggler/rpg-api/commit/5711f56), branch `feat/fenced-redis-save`.

## Shipped

- The Redis encounter repository carries `sdk.FencedEncounterRepository`, so the session takes the fenced save. The version lives beside the blob at `session-enc-ver:v1alpha1:<encounter>`, an integer, absent while no save has landed.
- `GetFencedEncounter` reads the blob and the version in one MGET. A missing blob is `ErrNotFound`. A blob with no version fails closed with `ErrBadRepository`.
- `SaveFenced` is one Lua script: check the version against `Expected`, check every touched sheet exists and keeps its player, then SET the world, the session record when given, each sheet, each seat, and advance the version. A moved version returns a stale marker the Go side wraps in `sdk.ErrStaleCheckpoint`. `goredis.Script` loads it once (EVALSHA, EVAL on NOSCRIPT).
- `SaveEncounter` stays for any plain caller and advances the version too, because a plain write is still a save that landed.
- `characterrepo.Key` and `characterrepo.PlayerIndexKey` are exported so the fenced save writes the one sheet key shape the character repository reads.
- The cold-reload measure gained two rows: the fenced read (MGET + decode) and the fenced save (encode + script) on a scratch encounter id.

## Decided

- The version shares the blob's lifetime: every script that writes the blob sets the version with the same TTL in the same call. No code deletes the blob today. A version found without its blob counts as no checkpoint, so a lingering version key cannot refuse a fresh Launch.
- Nothing is written until everything is checked. A Lua script does not roll back on a later error, so the fence, sheet existence and sheet owner are checked before the first SET.
- A sheet whose `player_id` would change fails closed with `ErrBadRepository`. No session verb moves a sheet between players; the player index belongs to the character repository's Create and Update.
- A fenced save's session record takes the encounter repository's TTL. The orchestrator builds both repositories with the same `Config.TTL`.
- The session orchestrator's `Config.Characters` must be the Redis character repository on `Config.Redis`, because the fenced save writes sheets into that Redis whatever the field holds. The server already wires it so; the field's doc now says it.
- The lobby handler suite ran the session over a mocked character repository, so the fenced save could not find the sheets it reads. It now runs on the Redis character repository on its own miniredis, as the server does, and seeds characters instead of arming mock calls.
- Two test hooks that matched the encounter save as a SET (the serialization suite's pause, the contract suite's write failure) now match the script too.
- The contract's `Seats` field is written under `session-seat:v1alpha1:<character>`, no TTL, as `SaveSeat` writes it. The card did not name seats; the toolkit contract does.

## Found

- **Blocker, toolkit v0.172.0, fixed in v0.172.1:** under the fence, Exit and a closing run never clear seats. Exit calls `clearSeats` after `commit`, and commit calls `clearSeatsOfAClosedRun` after `persist`. Under the fence `writeSeat` only adds to the pending set, the fenced save has already taken it, and releasing the guard drops it. A run's seats outlive its Exit, so every scene that exits and relaunches the same character fails with "seated in another session" (about 30 integration scenes here). The fix belongs in the toolkit.
- A run launched before this deploy holds a blob with no version and fails closed at its next verb. Relaunch it.
- The fenced save costs about 8 to 13 ms over encode + SET on the garrison keep's 9 MB blob, more than one round trip: the script copies the 9 MB argument. Move p50 moved within noise.

| row (p50, garrison keep, N=20) | before (dev) | after (branch) |
|---|---|---|
| Move answer, AnswerWhole | 949 ms | 942 ms |
| Move answer, S1 continues | 273 ms | 263 ms |
| Move drained, S1 continues | 957 ms | 914 ms |
| 5b encode + 5c SET | 62 + 8 ms | 57 + 8 ms |
| 5b + 5d fenced SaveFenced | - | 72 to 78 ms |
| 1 + 2 GetEncounter | 79 ms | 76 ms |
| 1 + 2 fenced GetFencedEncounter | - | 78 ms |

### Re-pinned on session v0.172.1 (seat clears ride the fenced save)

The toolkit fix (rpg-toolkit#2334) lands the cleared seats in the fenced save, and every Exit-then-relaunch scene passes. The pin also took `play/record` v0.3.0, which shrank the garrison keep's blob from 9.1 MB to 2.7 MB, so these Move numbers are not comparable with the table above. The fence cost is read inside one run: SaveFenced against encode + SET on the same blob.

| row (p50, garrison keep, N=20, session v0.172.1) | value |
|---|---|
| Move answer, AnswerWhole | 777 ms |
| Move answer, S1 continues | 198 ms |
| Move drained, S1 continues | 782 ms |
| 5b encode + 5c SET (whole / continues) | 23.05 + 1.01 ms / 22.07 + 0.94 ms |
| 5b + 5d fenced SaveFenced (whole / continues) | 24.43 ms / 23.29 ms |
| GetEncounter / fenced GetFencedEncounter (whole) | 26.27 ms / 27.41 ms |

At 2.7 MB the fence costs about one round trip, 0.3 ms on the write side and about 1 ms on the read side.

## Mistaken

- The first measure run read a stale `dump.rdb`: `redis-server` started from the workspace root loads the dump in its working directory. Start the side Redis with `--dir` set to an empty temp directory.

## Commands and proof

```
go test ./internal/orchestrators/session/ ./internal/repositories/character/ -count=1   # ok
go test ./internal/integration/session/ -run TestTheSessionSavesThroughTheFence -count=1  # ok; fails with the fence comparison disabled
golangci-lint run ./...                                                                     # 0 issues
redis-server --port 6498 --save '' --dir <empty tmp> --daemonize yes
RPG_MEASURE=1 RPG_MEASURE_N=20 RPG_MEASURE_REDIS=127.0.0.1:6498 \
  go test ./internal/integration/session/ -run TestMeasure_AnswerAndFirstBeat -count=1 -v -timeout 60m
```
