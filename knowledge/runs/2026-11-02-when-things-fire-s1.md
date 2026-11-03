# 2026-11-02 · When things fire S1 · The api serves the kind

Story [rpg-project#825](https://github.com/KirkDiggler/rpg-project/issues/825), slice [rpg-project#828](https://github.com/KirkDiggler/rpg-project/issues/828), design R1 B. Api PR [rpg-api#1181](https://github.com/KirkDiggler/rpg-api/pull/1181), on protos [rpg-api-protos#415](https://github.com/KirkDiggler/rpg-api-protos/pull/415) and toolkit [rpg-toolkit#2245](https://github.com/KirkDiggler/rpg-toolkit/pull/2245) and [#2246](https://github.com/KirkDiggler/rpg-toolkit/pull/2246).

## Shipped

- `walkToProto` sets `WalkAct.kind` from the session walk: own, compelled and displaced, with empty read as own. It keeps `forced` as the deprecated mirror ([rpg-api#1181](https://github.com/KirkDiggler/rpg-api/pull/1181)).
- The Thunderwave acceptance asserts `MOVE_KIND_DISPLACED` and `forced`. The new Dissonant Whispers acceptance asserts `MOVE_KIND_COMPELLED` and no `forced`, at the publish boundary and on the served story.

## Decided

- An unknown kind word refuses its act with `errUnknownValue`, like every other closed set in `convert_act.go`.
- Writing the deprecated `Forced` carries a scoped `nolint:staticcheck` with the reason.

## Found

- The api cannot pin a protos pseudo-version before the protos PR merges. Generated Go exists only on the CI-owned `generated` branch. The pins here are `generated` 4f44fc4 (v0.1.253) and the pushed toolkit heads, which re-pin to tags at merge-out.
- The Dissonant Whispers acceptance seats nobody beside the skeleton. It proves the kind, not the hold.

## Mistaken

- The first head shipped without this record and without pins, so CI could not compile it.

— cross-team agent, on behalf of KirkDiggler
