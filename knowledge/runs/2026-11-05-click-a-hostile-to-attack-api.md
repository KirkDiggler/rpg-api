# 2026-11-05 · Click a hostile to attack D4 · The api carries the approach cell

processed: 2026-11-12T165555Z

Card [rpg-project#872](https://github.com/KirkDiggler/rpg-project/issues/872), decision D4 A. Api half, on protos v0.1.257 (commit 3c58083e) and the PUSHED toolkit heads of [rpg-toolkit#2286](https://github.com/KirkDiggler/rpg-toolkit/pull/2286) (session cdf03b34) and [#2285](https://github.com/KirkDiggler/rpg-toolkit/pull/2285) (encounter 56dffeaa). Both toolkit pins are pseudo-versions until those PRs merge and tag.

## Shipped

- `targetCandidateToProto` copies the session's `TargetCandidate.Approach` onto the wire `TargetCandidate.approach`. Nil stays unset.
- `positionPtrToProto` is the outbound twin of `positionPtrFromProto`. The origin is a real cell, so nil is never folded into it.
- Converter tests pin a set cell, the origin cell, and nil.
- `TestAcceptance_AnOutOfReachGuardNamesHisApproach` walks the shipped castle in free roam. Alice is seated four steps from gate-1's reach. Afford names his approach cell, a shortest walk there is accepted whole, and the Attack on him is then available and lands.

## Decided

- The acceptance walk is a cube-lerp hex line across open gatehouse floor. The api has no route helper of its own, and a client's walk on open floor draws the same line.

## Found

- The protos repo tags its root, not `gen/go`. `go get .../gen/go@v0.1.257` fails, and the tag's commit pins as a pseudo-version.
- Without the converter line both new tests fail, so they can fail.

## Mistaken

- Nothing this run.

— cross-team agent, on behalf of KirkDiggler
