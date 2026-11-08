# Run — 2026-11-08 — behavior on the asynchronous world path

Part of [#866](https://github.com/KirkDiggler/rpg-project/issues/866), integrating [runtime optimizations #879](https://github.com/KirkDiggler/rpg-project/issues/879).

## Shipped

- Prepared current continuation-aware API/protocol integration on real pushed canonical encounter/session pins; no API behavior logic or replacement runner is added — [authorization](https://github.com/KirkDiggler/rpg-project/issues/866#issuecomment-6717591263).

## Decided

- Run the canonical route and disturbance scenarios with the production continuation runner, not the shared harness's synchronous AnswerWhole shortcut; an early response is not completion or a save — [upstream contract](https://github.com/KirkDiggler/rpg-api/pull/1197).
- Read completion through an actual following GetWhere request on the same session guard, and drain an old host before recreating its in-process locker over Redis; a different locker is not the same coordination domain — [continuation contract](https://github.com/KirkDiggler/rpg-toolkit/pull/2314).
- The delayed-boundary test controls scheduling only before handing the unchanged continuation to the production runner; it supplies no test-only NPC picker, coordinator or movement — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).

## Found

- The six-cell Move answers CONTINUING with five steps, Through and no save; the real guarded read waits for the rest, then the exact two-/three-item routes and ten-choice clang/resumption journey retain their original configuration through Redis and fresh hosts — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- A deliberately delayed continuation keeps the next request blocked, leaves storage unchanged before execution, survives request cancellation and a producer-config edit, delivers the sixth player step after Through, selects both NPCs once and refuses a repeated callback — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- Replacing the production runner's WithoutCancel context with the canceled request in a virtual overlay makes that test fail at two expected choices versus zero; repository bytes remain unchanged — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).

## Mistaken

- The old tests treated an early answer as completed movement and recreated a host while accepted work still held the original guard; observed status/progress mismatches and a closed-Redis continuation exposed the stale test lifecycle. They now follow the actual asynchronous contract without turning it off — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- A first no-partial-save assertion reserialized native maps and compared their ordering; the storage values were unchanged. It now compares the complete decoded record, while Through and exact step/choice assertions independently prove the boundary — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
