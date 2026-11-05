# 2026-11-05 · The guard holds the gate S2 · The api carries the bump

Story [rpg-project#836](https://github.com/KirkDiggler/rpg-project/issues/836), slice [rpg-project#842](https://github.com/KirkDiggler/rpg-project/issues/842), design R2 A and R7. Api half T9, on protos v0.1.256 and toolkit [rpg-toolkit#2278](https://github.com/KirkDiggler/rpg-toolkit/pull/2278) (encounter) and [#2280](https://github.com/KirkDiggler/rpg-toolkit/pull/2280) (session).

## Shipped

- `barredToProto` spells the session's `BarredAct` on the v1alpha2 act stream: mover, barrier, the refused cell for a step, the door for a door verb. A door bump carries no cell.
- Move needs no new code. A barred walk already crosses `STOPPED` with the session's reason as `stop_reason` and no step.
- OpenDoor, CloseDoor and Unlock answer a session `Refused` with `FAILED_PRECONDITION` and the holder's sentence. That is the S1b wire, unchanged.
- The castle gains gate-1's `barred` and `challenged` lines, the `on: barred` row on posted and alert, and the `challenged` state. The file is the encounter branch's mirror byte for byte.
- `TestAcceptance_TheGateGuardAnswersTheBump` walks the shipped castle: first bump, second bump, a hand on the held gate, and a wall bump.

## Decided

- The door responses have no refusal field, so the status code is the one honest answer. Returning the unchanged door as a success would read as one.
- The session's `Verb` on a door bump has no proto field and is dropped. Adding it is an additive protos follow-up.

## Found

- The act stream converts in the v1alpha2 handler, not v1alpha1's `convert.go`.
- The protos tag v0.1.256 points at a commit that carries the generated Go, so it pins directly.

## Mistaken

- Nothing this run.

— cross-team agent, on behalf of KirkDiggler
