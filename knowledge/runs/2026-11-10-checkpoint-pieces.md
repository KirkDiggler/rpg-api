# Run — 2026-11-10 — the rest stored as pieces in Redis

processed: 2026-11-12T165555Z

Base: `rpg-api` `dev` at 83082f15. Story: [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879), card [rpg-project#897](https://github.com/KirkDiggler/rpg-project/issues/897) leg (b), PR 4 of 4. Upstream: encounter [rpg-toolkit#2362](https://github.com/KirkDiggler/rpg-toolkit/pull/2362) developed against pushed commits by pseudo-version and pinned at its tag v0.192.0, and session [rpg-toolkit#2363](https://github.com/KirkDiggler/rpg-toolkit/pull/2363) pinned at its tag v0.178.0 (main 56ab0946).

## Shipped

- The Redis encounter repository carries `sdk.PiecesEncounterRepository`. The rest is four keys under v1alpha3: the head string `session-enc:v1alpha3:<id>` (every save, the existence sentinel), the roster hash `session-enc-roster:v1alpha3:<id>` (`_` and `m:<member>`), the knowledge hash `session-enc-know:v1alpha3:<id>` (`_` and `o:<observer>`), and the journal string `session-enc-journal:v1alpha3:<id>`. Every value is `<mark>\n<piece JSON>`. The record list, floor and version keys are leg (c)'s, unchanged.
- One Lua script for every save. All checks run first: the fence, a first or plain save carrying every piece and starting the record at seq 1, the floor scan, the sheets. Then it sets the head, replaces both hashes on a whole save, HSETs the fields sent and HDELs the removed ones, sets the journal when sent, appends the segment and drops below the floor, renews the TTL on every key together, writes session, sheets and seats, and increments the version last.
- `SaveFenced`, `SaveAppended` and the plain `SaveEncounter` split the rest into every piece and write the same shape through the same script.
- The read is one MULTI/EXEC of head, both hashes, journal, floor, version, list and the EXISTS of the earlier keys. It strips the marks, joins with `encounter.JoinPieces`, then `record.JoinData`, and returns the marks on `GetFencedEncounterOutput.Marks`.
- The measure harness splits each save's bytes by shape and marks round Moves from the head's clock. The cold-reload row reads the pieces.

## Decided

- One sentinel for an earlier store shape, `ErrCheckpointBeforePieces`, replacing leg (c)'s `ErrCheckpointBeforeAppendedRecord`. A v1alpha2 rest and a v1alpha1 blob are the same case: a checkpoint nothing writes any more, refused rather than migrated. sdkerr maps it to FAILED_PRECONDITION.
- Leg (c)'s `ErrCheckpointRecordMismatch` is broadened and renamed `ErrCheckpointMismatch`. Every disagreement between a checkpoint's keys is one class: a missing or bad piece, a stray field, a record short of its floor. It still wraps `sdk.ErrBadRepository`. sdkerr now maps it to FAILED_PRECONDITION ahead of the INTERNAL bucket, because the request is well formed and the session exists, and the stored state refuses every verb until relaunch. The join's own error (`ErrMissingPiece`, `ErrBadPiece`, the decode cause) stays in the chain and the message names the piece.
- A save is whole when it carries every piece its marks name. A whole save replaces both hashes, so a member the rest lost cannot linger when nothing names it removed. `SavePieces` without marks is refused.
- PEXPIRE renews every piece key on every save, unchanged hashes and journal included, so no piece outlives or predeceases the head.
- The version increments last, after every write.

## Found

- A rest with nil `Members` joins back as an empty slice (`encounter.JoinPieces` always makes the roster). It is not byte-exact for that hand-made fixture; every encounter `ToData` writes a non-nil slice. The contract test's replacement world now says `Members: []MemberData{}`. Raised with the lead for rpg-toolkit#2362.
- Garrison path 4, N=30, both modes: a Move without a round writes about 33 KB in 9 to 11 pieces. A round Move writes about 476 KB in about 101 pieces, of which knowledge is about 288 KB (the reconfirm stamp, rpg-toolkit#2360) and the segment about 140 KB.
- No journal write happened in 30 Moves at path 4.

## Mistaken

- The first pieces suite wrote a no-op assertion; removed before commit.

## Verification

- `go test -race -count=1 ./...` at the tag pins (encounter v0.192.0, session v0.178.0): green, no failures.
- `make ci-check` at the tag pins: all CI checks passed; release pins verified, no replace.
- `golangci-lint` on the changed packages: 0 issues.
- Mutants: dropping the whole-save DEL fails `TestAWholeSave_ReplacesTheHashes`; dropping the roster PEXPIRE fails `TestEveryKey_SharesTheCheckpointsTTL`.
- Measure: `RPG_MEASURE=1 RPG_MEASURE_REDIS=127.0.0.1:6512 RPG_MEASURE_N=30 RPG_MEASURE_PATH=4 RPG_MEASURE_RELOAD=0 go test ./internal/integration/session/ -run 'TestMeasure_AnswerAndFirstBeat$' -count=1 -v`, on a private Redis; before on dev 83082f15 on another.
