# Run — 2026-11-09 — the record appended in Redis

processed: 2026-11-12T165555Z

Base: `rpg-api` `dev` at 2be23860. Story: [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879), card [rpg-project#897](https://github.com/KirkDiggler/rpg-project/issues/897) leg (c), PR 4 of 4. Upstream: record [rpg-toolkit#2352](https://github.com/KirkDiggler/rpg-toolkit/pull/2352) at play/record v0.4.0, encounter [rpg-toolkit#2353](https://github.com/KirkDiggler/rpg-toolkit/pull/2353) at v0.189.0, session [rpg-toolkit#2354](https://github.com/KirkDiggler/rpg-toolkit/pull/2354) at v0.176.0.

## Shipped

- The Redis encounter repository carries `sdk.AppendingEncounterRepository`. The checkpoint is the rest at `session-enc:v1alpha2:<id>`, the record segments in a list at `session-enc-log:v1alpha2:<id>`, the retention floor at `session-enc-floor:v1alpha2:<id>`, and the version key unchanged.
- One Lua script for every fenced save. It checks the version, refuses a first save that does not start at seq 1, and checks the sheets before any write. It then sets the rest, drops the list when the save starts at seq 1, pushes the segment, pops leading segments wholly below the floor, writes session, sheets and seats, and moves the version. Rest, list and version share one TTL.
- `SaveFenced` and the plain `SaveEncounter` split a whole world through the record's own `LoadLog` and `DataFrom` and write the same shape.
- The read is one MULTI/EXEC of rest, version, list and the old key's EXISTS, joined with `record.JoinData`. A checkpoint found only under `session-enc:v1alpha1:` is `ErrCheckpointBeforeAppendedRecord`, which sdkerr maps to FAILED_PRECONDITION.
- The measure harness counts the bytes each Move's save hands Redis and what the store holds after it. The cold-reload row reads rest plus record.

## Decided

- Each list element is `<last seq>\n<segment JSON>`, because the floor drop must read a segment's last seq without decoding it in Lua.
- An empty segment is not pushed, because a beatless verb has nothing to append and the list stays one element per save that wrote beats.
- The floor is stored in a sibling key under the shared TTL, not as a field on the rest, because the rest stays the encounter's own bytes with no envelope, and the floor is written on every save in the same script. The read joins on it.
- Every disagreement between rest, list and floor is `ErrCheckpointRecordMismatch`, beside `sdk.ErrBadRepository`. That covers a record or floor with no rest, a rest with no floor, a malformed or misnamed segment, and a missing seq at or above the floor, including a lost list under a rest whose next seq is above the floor. The read fails closed rather than load a short or empty story.
- The floor scan reads each dropped segment's prefix before any write, then drops them with one LTRIM. A malformed prefix refuses the save.
- A rest that carries entries is refused in Go before the script as `ErrRestCarriesRecord`.
- The store does not check `Since` against its last stored seq, because that would copy the last segment into Lua on every save. The fence guarantees it, and the join refuses a gap at the next read.
- The legacy key is refused, not migrated, as S5 did, because the dev env relaunches its sessions.
- The read passes its floor to `record.JoinData`, which tolerates what the store dropped below it and refuses any hole at or above it. A bounded record whose floor dropped its oldest segments has a test that it loads from the oldest seq kept.

## Found

- The rest weighs about 637 KB on the garrison keep, above the map's 550 KB guess. Segments come in two sizes: about 5.5 KB for a Move that paid no world round and 139 to 151 KB for one that did.

## Mistaken

- Review at 8c91401c, blocking: the floor was derived on the read from the oldest stored seq, so a bounded record whose list was lost loaded as an empty story with no error. The floor is now stored and joined on, and each mismatch is an error by name.
- The same review: the floor-pop boundary was unpinned, the Go decode ignored the last-seq prefix, and a rest with entries was not refused. All three are pinned now.
- The Lua prefix parse called `tonumber` on an empty match, which raises a script error instead of the named refusal. The new malformed-prefix test found it.

## Verification

Bytes per Move, `bench-keep-garrison`, path 4, N=30, AnswerWhole, private Redis. The after column was first measured at e127cb97. It was measured again on the review-fix code, because that adds the floor key to the save; every Move came within 54 bytes of the first run.

| | before (dev 2be23860) | after (this branch) |
|---|---|---|
| write, first Move | 1,238,601 | 641,221 |
| write, last Move | 4,032,134 | 778,246 |
| write p50 | 2,642,954 | 777,877 |
| rest written p50 | whole blob | 637,896 |
| segment written p50 | none | 139,427 |
| stored record after 30 Moves | in the blob | 3,481,512 in 34 segments |

Before, every Move rewrites the whole blob, which grows about 137 KB per Move. After, the write is flat over 30 Moves. The record grows in the list only.

- `make ci-check`: all checks passed.
- `go test -race -count=1 ./...`: green.
